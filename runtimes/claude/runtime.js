// agentops claude agent runtime — implements the AgentRuntime /work contract:
//   1. long-poll  GET  $CONTROL_URL/work?convo=$CONVO_ID&pod=$POD_NAME&wait=25
//   2. feed the unit into ONE live `@anthropic-ai/claude-agent-sdk` session
//      held open for the pod's life, streaming a formatted transcript to
//      STDOUT (pod logs / VictoriaLogs)
//   3. report     POST $CONTROL_URL/work/done
//   4. exit 0 after RUNTIME_IDLE_TTL_M minutes without work
//
// This is the SDK-only runtime (`agentops-runtime-claude`, default-eligible).
// `runtimes/claude-cli/` carries today's one-CLI-spawn-per-unit logic,
// unchanged, as its own component (off by default) — see
// openspec/changes/claude-runtime-sdk/design.md for why the split is two
// components rather than one image with a switch.
//
// What changes relative to the CLI shape, and why (design.md decisions D1-D7):
//   D2 — the poll loop is the generator: one query() session lives across
//        units, each fed through an async-generator "channel" (makeInputChannel)
//        instead of a fresh subprocess per unit.
//   D3 — canUseTool decides every call. No `allowedTools` (it would shadow
//        the callback), `permissionMode: 'default'` (NOT `dontAsk`, which
//        skips the callback entirely).
//   D4 — the session is reused ONLY when a unit's session-affecting
//        configuration (systemPrompt/mcpServers/maxTurns/cwd/agent) is
//        identical to the one the open session was built from. The tool
//        allowlist is deliberately excluded — it lives in the callback and
//        is re-read fresh every unit, open/reuse/re-open alike.
//   D5 — the live session is a cache. Every unit reports the vendor's own
//        session id, and the continuity ladder (verbatim from the pre-SDK
//        shape, still shared with runtimes/claude-cli's own copy) governs
//        the re-open path exactly as it governed the old resume-retry path.
//   D6 — the session is torn down on idle-TTL exit, on a fingerprint change
//        (before the replacement opens), and on an unattributable session
//        failure (the generator ending mid-turn, or throwing).
//   D7 — the transcript renders identically to before: formatEvent's field
//        reads (model/tools/mcp_servers on init, content blocks on
//        assistant, subtype/num_turns/duration_ms/result on result) line up
//        with the SDK's typed messages without any change.
//
// The repository is checked out at /data/workspace — the SAME path the
// pre-operator claude-runner used, so Claude session files (keyed by cwd
// under $HOME/.claude/projects/-data-workspace/) resume seamlessly across
// systems and across the CLI/SDK split.

'use strict';

const { spawn } = require('child_process');
const fs = require('fs');
const path = require('path');
const { agentDeclaredTools, composeAllowedTools, safeJoin, sanitizeLog, logUngrantedTools } = require('./tools');
const { DEFAULT_LIMIT, newSpinWatch, noteToolUse, spinMessage, discardedNotice } = require('./spin');
const { newCallRecorder } = require('./report');

const CONTROL_URL = process.env.CONTROL_URL || '';
const CONVO_ID = process.env.CONVO_ID || '';
const POD_NAME = process.env.POD_NAME || '';
const REPO_URL = process.env.REPO_URL || '';
const REPO_REF = process.env.REPO_REF || 'master';
const GIT_AUTH_TYPE = process.env.GIT_AUTH_TYPE || '';
const GIT_SSH_KEY = process.env.GIT_SSH_KEY || '';
const GIT_TOKEN = process.env.GIT_TOKEN || '';
const TTL_MS = (Number.parseInt(process.env.RUNTIME_IDLE_TTL_M || '10', 10)) * 60 * 1000;
const WORKSPACE = process.env.WORKSPACE || '/data/workspace';
const MCP_CONFIG = process.env.MCP_CONFIG || '/etc/agentops/mcp.json';
// Same floor the pre-SDK shape passed as `--max-turns` when a unit carried
// none — kept identical so a unit with no turn budget behaves the same.
const DEFAULT_MAX_TURNS = 60;
// How many identical unparsable tool calls in a row end a run. 0 disables the
// breaker entirely, which is a decision an install can make and this file will
// not make for it.
const SPIN_LIMIT = (() => {
  const v = Number.parseInt(process.env.RUNTIME_UNPARSED_REPEAT_LIMIT || '', 10);
  return Number.isFinite(v) && v >= 0 ? v : DEFAULT_LIMIT;
})();

// Guarded on require.main so this file can be `require()`d by its own test
// suite — coverage's only way to reach the functions below — without also
// exiting the test process or starting the network loop at the bottom.
if (require.main === module && (!CONTROL_URL || !CONVO_ID)) {
  console.error('[runtime] CONTROL_URL and CONVO_ID are required');
  process.exit(1);
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

// ---------------------------------------------------------------------------
// Git checkout. Unchanged from the pre-SDK shape — the SDK drives claude-code
// in-process, but this process still owns syncing the profile's repo before
// any unit runs, exactly as before.
// ---------------------------------------------------------------------------

function gitEnv() {
  const env = { ...process.env };
  if (GIT_AUTH_TYPE === 'ssh' && GIT_SSH_KEY) {
    env.GIT_SSH_COMMAND = `ssh -i ${GIT_SSH_KEY} -o UserKnownHostsFile=/tmp/known_hosts -o StrictHostKeyChecking=accept-new`;
  }
  return env;
}

function repoURL() {
  if (GIT_AUTH_TYPE === 'https' && GIT_TOKEN && REPO_URL.startsWith('https://')) {
    return REPO_URL.replace('https://', `https://x-access-token:${GIT_TOKEN}@`);
  }
  return REPO_URL;
}

function run(cmd, args, opts = {}) {
  return new Promise((resolve) => {
    const p = spawn(cmd, args, { ...opts, stdio: ['ignore', 'pipe', 'pipe'] });
    let out = '', err = '';
    p.stdout.on('data', (c) => { out += c; });
    p.stderr.on('data', (c) => { err += c; });
    p.on('error', (e) => resolve({ code: -1, out, err: String(e.message) }));
    p.on('close', (code) => resolve({ code, out, err }));
  });
}

// WORKSPACE may itself be a volume mount point — never remove the directory,
// only its contents.
function clearDir(dir) {
  for (const e of fs.readdirSync(dir)) {
    fs.rmSync(path.join(dir, e), { recursive: true, force: true });
  }
}

async function syncRepo() {
  if (!REPO_URL) return;
  if (!fs.existsSync(path.join(WORKSPACE, '.git'))) {
    fs.mkdirSync(WORKSPACE, { recursive: true });
    clearDir(WORKSPACE);
    const r = await run('git', ['clone', '--depth', '1', '-b', REPO_REF, repoURL(), '.'], { cwd: WORKSPACE, env: gitEnv() });
    if (r.code !== 0) throw new Error(`git clone: ${r.err.slice(-400)}`);
  } else {
    const f = await run('git', ['fetch', '--depth', '1', 'origin', REPO_REF], { cwd: WORKSPACE, env: gitEnv() });
    if (f.code !== 0) throw new Error(`git fetch: ${f.err.slice(-400)}`);
    const rs = await run('git', ['reset', '--hard', `origin/${REPO_REF}`], { cwd: WORKSPACE, env: gitEnv() });
    if (rs.code !== 0) throw new Error(`git reset: ${rs.err.slice(-400)}`);
  }
}

// MAX_BRIEF mirrors ConversationStatus.Brief's MaxLength — bounding here means
// a value this runtime reports never gets silently truncated by the manager
// mid-sentence.
const MAX_BRIEF = 512;

// extractBrief pulls a standalone `<brief>...</brief>` block (format.md) out of
// the agent's final text, mirroring the block grammar's own recognition rule —
// the tag alone on its own line — closely enough for this one tag, without
// pulling in the full parser channels/telegram and the console each carry for
// the tags a reader actually sees. `brief` is a CONTRACT FIELD, never a
// displayed section, so it is removed from what becomes `result`.
function extractBrief(text) {
  const m = /^<brief>[ \t]*\r?\n([\s\S]*?)\r?\n^<\/brief>[ \t]*(?:\r?\n)*/m.exec(text);
  if (!m) return { text, brief: '' };
  const brief = m[1].trim().replace(/\s+/g, ' ').slice(0, MAX_BRIEF);
  return { text: (text.slice(0, m.index) + text.slice(m.index + m[0].length)).trim(), brief };
}

// Turn one typed SDK message into a compact human-readable log line (D7). The
// SDK's SDKSystemMessage(init)/SDKAssistantMessage/SDKResultMessage carry the
// same fields the old parsed stream-json events did (model/tools/mcp_servers,
// content blocks, subtype/num_turns/duration_ms/result) — so the exact same
// rendering applies unchanged, and a pod log reads the same on either
// component.
function formatEvent(ev, rawLine) {
  try {
    if (ev.type === 'system' && ev.subtype === 'init') {
      return `[init] model=${ev.model} tools=${(ev.tools || []).length} mcp=${(ev.mcp_servers || []).map((s) => `${s.name}:${s.status}`).join(',')}\n`;
    }
    if (ev.type === 'assistant') {
      return (ev.message?.content || []).map((b) => {
        if (b.type === 'text') return `[claude] ${b.text}\n`;
        if (b.type === 'tool_use') {
          const input = JSON.stringify(b.input || {});
          return `[tool] ${b.name} ${input.length > 160 ? input.slice(0, 160) + '…' : input}\n`;
        }
        return '';
      }).join('');
    }
    if (ev.type === 'result') {
      return `\n=== RESULT (${ev.subtype}, ${ev.num_turns} turns, ${Math.round((ev.duration_ms || 0) / 1000)}s) ===\n${ev.result || ''}\n`;
    }
    return '';
  } catch {
    return rawLine + '\n';
  }
}

// ---------------------------------------------------------------------------
// The SDK-import seam. `@anthropic-ai/claude-agent-sdk` is a real npm
// dependency installed in the built image (never in this directory's —
// nonexistent — node_modules), so requiring it unconditionally at module
// load would break `node --test` here. getQuery() resolves it lazily, on
// first real use; __setQueryForTest lets a test substitute a scripted fake
// query() without the real package ever being required.
//
// The package is ESM-only (`"type": "module"`, no `.cjs`/`.js` CommonJS
// build) — verified against @anthropic-ai/claude-agent-sdk@0.3.310's
// package.json. `require()` of a synchronous (no top-level-await) ESM
// module is supported in Node 22 (the image's base), confirmed directly:
// `require('@anthropic-ai/claude-agent-sdk')` resolves and returns the
// module's named exports on Node 22.22. Kept as a plain `require()` rather
// than a dynamic `import()` so this file stays CommonJS throughout — no
// package.json exists in this directory, so CommonJS is Node's default.
// ---------------------------------------------------------------------------
let _query = null;
function getQuery() {
  if (!_query) _query = require('@anthropic-ai/claude-agent-sdk').query;
  return _query;
}
function __setQueryForTest(fn) { _query = fn; }

// ---------------------------------------------------------------------------
// Streaming input (D2: "the poll loop is the generator"). One long-lived
// async generator feeds query()'s `prompt`; push() delivers the next
// SDKUserMessage, resolving a pending read if one is already waiting,
// queuing otherwise. A conversation is strictly serial, so there is never
// more than one unread push outstanding.
// ---------------------------------------------------------------------------
function makeInputChannel() {
  let resolveNext = null;
  const queue = [];
  async function* generator() {
    for (;;) {
      const msg = queue.length ? queue.shift() : await new Promise((res) => { resolveNext = res; }); // NOSONAR
      yield msg;
    }
  }
  function push(msg) {
    if (resolveNext) {
      const r = resolveNext;
      resolveNext = null;
      r(msg);
    } else {
      queue.push(msg);
    }
  }
  return { iterator: generator(), push };
}

// makeSdkUserMessage builds the SDKUserMessage for one prompt — `message`
// (an Anthropic Messages API user message) and `parent_tool_use_id: null`
// are the only fields the type requires; every other SDKUserMessage field is
// optional and this driver sets none of them.
function makeSdkUserMessage(promptText) {
  return { type: 'user', message: { role: 'user', content: promptText }, parent_tool_use_id: null };
}

// ---------------------------------------------------------------------------
// The composed MCP config, read fresh every unit (never cached): it is
// CONTENT, not a REF, so an operator's edit must reach the fingerprint (D4)
// on the very next unit, exactly as the old CLI's `--mcp-config` would reread
// the file on every fresh spawn.
//
// `internal/mcpcompile` writes a secret-backed header/env value as a literal
// "${ENV_NAME}" placeholder in the file and adds ENV_NAME to the pod spec —
// resolution happens in the kubelet, and the CLI's OWN `--mcp-config` loader
// performs the "${ENV_NAME}" -> process.env lookup when it reads the file off
// disk. The SDK's `mcpServers` option takes an already-parsed object instead
// of a file path, bypassing that file loader entirely, so this driver
// performs the SAME substitution itself before handing the object to
// query() — otherwise a header carrying a literal "${ENV_NAME}" string would
// reach the MCP server as-is instead of the secret it names.
// ---------------------------------------------------------------------------
const ENV_PLACEHOLDER = /\$\{([A-Z0-9_]+)\}/g;

function expandEnvPlaceholders(value, log) {
  if (typeof value === 'string') {
    return value.replace(ENV_PLACEHOLDER, (whole, name) => {
      if (Object.hasOwn(process.env, name)) return process.env[name];
      log(`[runtime] mcp config: "\${${name}}" names no env var on this pod — left unresolved`);
      return whole;
    });
  }
  if (Array.isArray(value)) return value.map((v) => expandEnvPlaceholders(v, log));
  if (value && typeof value === 'object') {
    const out = {};
    for (const [k, v] of Object.entries(value)) out[k] = expandEnvPlaceholders(v, log);
    return out;
  }
  return value;
}

function readMcpServers(mcpConfigPath, log = () => {}) {
  try {
    const parsed = JSON.parse(fs.readFileSync(mcpConfigPath, 'utf8'));
    const servers = (parsed && typeof parsed.mcpServers === 'object' && parsed.mcpServers) || {};
    return expandEnvPlaceholders(servers, log);
  } catch (e) {
    log(`[runtime] mcp config ${mcpConfigPath}: ${e.message} — treating it as no MCP servers`);
    return {};
  }
}

// ---------------------------------------------------------------------------
// Configuration fingerprint (D4). Deliberately excludes the tool allowlist —
// that lives in the canUseTool callback (D3) and is re-read fresh every
// unit, whatever the session decision below turns out to be.
// ---------------------------------------------------------------------------
const FINGERPRINT_FIELDS = ['systemPrompt', 'mcpServers', 'maxTurns', 'cwd', 'agent'];

function normalizedMaxTurns(unit) {
  return unit.maxTurns || DEFAULT_MAX_TURNS;
}

function computeFingerprint(unit, mcpServers) {
  return {
    systemPrompt: unit.systemPrompt || '',
    mcpServers: mcpServers || {},
    maxTurns: normalizedMaxTurns(unit),
    cwd: WORKSPACE,
    agent: unit.agent || '',
  };
}

function changedFingerprintFields(prev, next) {
  return FINGERPRINT_FIELDS.filter((f) => JSON.stringify(prev[f]) !== JSON.stringify(next[f]));
}

function fingerprintsEqual(prev, next) {
  return changedFingerprintFields(prev, next).length === 0;
}

// ---------------------------------------------------------------------------
// Permissions (D3). The composed allowlist (tools.js, unchanged) becomes the
// callback's rule set instead of an `--allowedTools` flag.
//
// allowlistMatch reuses the SAME base-name/server-name extraction tools.js's
// logUngrantedTools already uses for cross-checking a grant against the
// vendor's own tool list — NOT a re-implementation of the CLI's own
// sub-command glob matching for a scoped entry like `Bash(git *)`. A scoped
// entry is matched (and therefore granted) at the BASE TOOL NAME only here,
// same granularity as that cross-check. This is a deliberate, narrower
// guarantee than the real CLI's `--allowedTools` parser enforces for that one
// scoped form — flagged for review, see the hand-back report.
// ---------------------------------------------------------------------------
function allowlistMatch(allowed, toolName) {
  for (const entry of allowed || []) {
    const base = String(entry).split('(')[0].trim();
    if (!base) continue;
    if (base.startsWith('mcp__')) {
      // A server-wide grant (`mcp__<server>`) matches any tool on that
      // server; otherwise the match must be exact.
      if (toolName === base || toolName.startsWith(`${base}__`)) return entry;
      continue;
    }
    if (base === toolName) return entry;
  }
  return null;
}

// createCanUseTool returns the per-call decision canUseTool implements.
// `allowedHolder` is a MUTABLE box (`{ current: [...] }`), never the array
// itself — the allowlist is excluded from the fingerprint specifically so it
// can change between units with no session re-open (D4), and a callback
// bound once at session-open time must still see the CURRENT unit's
// composition, not the one open() saw. The caller updates `.current` on
// every unit, reused or not.
function createCanUseTool(allowedHolder, log = () => {}) {
  return async function canUseTool(toolName) {
    const matched = allowlistMatch(allowedHolder.current, toolName);
    if (matched) return { behavior: 'allow' };
    const allowed = allowedHolder.current || [];
    log(`[runtime] denied "${sanitizeLog(toolName)}" — no entry in the composed allowlist (${sanitizeLog(allowed.length ? allowed.join(',') : '(none)')}) matched`);
    return { behavior: 'deny', message: `"${toolName}" is not in this conversation's allowed tools` };
  };
}

// ---------------------------------------------------------------------------
// One live session, held across units (D2/D6).
// ---------------------------------------------------------------------------
let liveSession = null; // { sdkQuery, channel, fingerprint, allowedHolder }

function openSession(unit, fingerprint, allowed, mcpServers) {
  const channel = makeInputChannel();
  const allowedHolder = { current: allowed };
  const options = {
    // D3: no `allowedTools` (a bare entry there would auto-approve BEFORE
    // this callback runs), `permissionMode: 'default'` (NOT `dontAsk`, which
    // skips the callback entirely — the no-prompt guarantee comes from the
    // callback always answering, never from the mode).
    canUseTool: createCanUseTool(allowedHolder, (m) => console.log(sanitizeLog(m))),
    permissionMode: 'default',
    cwd: WORKSPACE,
    mcpServers,
    // Matches the CLI shape's `--strict-mcp-config`: only the servers named
    // in MCP_CONFIG are honoured, ignoring project/user/plugin MCP config.
    strictMcpConfig: true,
    maxTurns: normalizedMaxTurns(unit),
    // The RUN_ID/TG_THREAD_ID an MCP server's own process reads from its
    // env — set once, at session open, from the FIRST unit this session
    // serves. A unit served later by a REUSED session runs under the
    // session-opening unit's RUN_ID/TG_THREAD_ID, not its own: the SDK's
    // `Options.env` applies for the query()'s whole life, and RUN_ID/
    // TG_THREAD_ID are deliberately not in the fingerprint (D4 only lists
    // session-affecting config, and per-unit correlation ids were never
    // meant to force a re-open). No MCP server in this repository reads
    // either var today; an external one that does should treat them as
    // "the id of whichever unit most recently opened this session", not
    // "the id of the unit in flight" — flagged for review.
    env: { ...process.env, RUN_ID: unit.runId, TG_THREAD_ID: unit.threadId != null ? String(unit.threadId) : '' },
    // Deliberately NEVER `agent: unit.agent` — passing it would re-apply the
    // agent definition's own tool list as a vendor availability
    // intersection, defeating the allowlist tools.js already composed.
    // `unit.agent` is a fingerprint input only (D4).
  };
  const contextId = contextIdOf(unit);
  if (contextId) options.resume = contextId;
  if (unit.systemPrompt) {
    // `{ type: 'preset', append }` ADDS to Claude Code's own default system
    // prompt, matching the old CLI shape's `--append-system-prompt` exactly
    // — the runtime's own system prompt survives, and the allowlist (not
    // this prompt) stays the only permission authority. A bare string here
    // would REPLACE the whole system prompt instead, which is not what
    // `--append-system-prompt` ever did.
    options.systemPrompt = { type: 'preset', preset: 'claude_code', append: unit.systemPrompt };
  }
  const sdkQuery = getQuery()({ prompt: channel.iterator, options });
  return { sdkQuery, channel, fingerprint, allowedHolder };
}

function closeLiveSession() {
  if (!liveSession) return;
  try {
    liveSession.sdkQuery.close();
  } catch {
    // Best-effort teardown — a replacement session must never be blocked on
    // a superseded one failing to close cleanly.
  }
  liveSession = null;
}

// openOrReuseSession decides open/reuse/re-open (D4) and logs which one it
// took, as three DISTINCT lines, so a pod log shows the active path without
// inference (D7).
function openOrReuseSession(unit, fingerprint, allowed, mcpServers) {
  const contextId = contextIdOf(unit);
  const runTag = `run ${sanitizeLog(unit.runId)}`;
  if (!liveSession) {
    console.log(`[runtime] session open${contextId ? ' continue=' + sanitizeLog(contextId) : ''} (${runTag})`);
    liveSession = openSession(unit, fingerprint, allowed, mcpServers);
    return;
  }
  if (!fingerprintsEqual(liveSession.fingerprint, fingerprint)) {
    const changed = changedFingerprintFields(liveSession.fingerprint, fingerprint);
    console.log(`[runtime] session re-open${contextId ? ' continue=' + sanitizeLog(contextId) : ''} — configuration changed (${changed.join(', ')}) (${runTag})`);
    closeLiveSession();
    liveSession = openSession(unit, fingerprint, allowed, mcpServers);
    return;
  }
  console.log(`[runtime] session reuse (${runTag})`);
  // The allowlist is excluded from the fingerprint on purpose (D4) — it
  // still has to reach the live callback every unit, reused or not.
  liveSession.allowedHolder.current = allowed;
}

// ---------------------------------------------------------------------------
// Reading one turn (D2). Consumes the open query()'s own async-generator
// output until this push's `result`, which is the unit's outcome — the
// "strictly serial per conversation" invariant is exactly what makes this
// unambiguous, with no id correlation needed.
// ---------------------------------------------------------------------------

// findSpinVerdict scans one assistant message for a tool call the model could
// not form (spin.js). Returns the first verdict, or null.
function findSpinVerdict(ev, watch) {
  for (const b of ev.message?.content || []) {
    if (b.type !== 'tool_use') continue;
    const verdict = noteToolUse(watch, b.name, b.input);
    if (verdict) return verdict;
  }
  return null;
}

// readInit takes the vendor's own account of what it actually has — cross-
// checked against the grant once the turn completes, the one point in this
// process that ever sees it.
function readInit(ev) {
  return { initTools: ev.tools || [], initMcpServers: ev.mcp_servers || [] };
}

// abortSpinningTurn is the async equivalent of a SIGTERM to a spawned CLI:
// abort generation rather than let a loop that cannot end burn the turn
// budget. The caller keeps draining to this turn's `result` rather than
// returning — the generator must reach a settled boundary before the next
// unit can safely push into it.
function abortSpinningTurn(sdkQuery, spinVerdict) {
  console.log(
    `[runtime] ${spinVerdict.name} called ${spinVerdict.repeats}x with the same unparsable arguments — ` +
      `nothing ran, ending the run: ${spinVerdict.raw.slice(0, 160)}`,
  );
  sdkQuery.interrupt().catch(() => {});
}

// foldTurnEvent folds one event into the turn's state `t`.
function foldTurnEvent(ev, t, { sdkQuery, watch, calls }) {
  if (ev?.session_id && !t.sessionId) t.sessionId = ev.session_id;
  if (ev?.type === 'system' && ev.subtype === 'init') Object.assign(t, readInit(ev));
  calls.note(ev);
  const txt = formatEvent(ev, '');
  if (txt) process.stdout.write(txt);
  if (ev?.type === 'assistant' && !t.spinVerdict) {
    t.spinVerdict = findSpinVerdict(ev, watch);
    if (t.spinVerdict) abortSpinningTurn(sdkQuery, t.spinVerdict);
  }
}

async function consumeTurn(sdkQuery, watch, calls) {
  const t = { sessionId: null, spinVerdict: null, initTools: null, initMcpServers: null };
  for (;;) {
    const { value: ev, done } = await sdkQuery.next(); // NOSONAR
    if (done) return { endedUnexpectedly: true, sessionId: t.sessionId, initTools: t.initTools, initMcpServers: t.initMcpServers };
    foldTurnEvent(ev, t, { sdkQuery, watch, calls });
    if (ev?.type === 'result') return { result: ev, ...t };
  }
}

// resumeSucceeded decides whether one attempted resume actually recovered
// the context — the vendor's own signal (a session id, any result text). A
// session that was found emits a session id even on a later failure, so "no
// session id and no result" is what a missing session looks like here too.
function resumeSucceeded(first) {
  if (first.status === 'succeeded') return true;
  return !(!first.sessionId && !first.result);
}

// runTurnOnLiveSession pushes one prompt into the already-open/reused/
// re-opened liveSession and reads it to an outcome shaped
// `{status, sessionId?, result, brief?, turns?, toolCalls?}` — never an
// `exitCode`, since there is no process exit to report here.
async function runTurnOnLiveSession(prompt, allowed) {
  const watch = newSpinWatch(SPIN_LIMIT);
  const calls = newCallRecorder();
  liveSession.channel.push(makeSdkUserMessage(prompt));

  let outcome;
  try {
    outcome = await consumeTurn(liveSession.sdkQuery, watch, calls);
  } catch (e) {
    // Unattributable session failure (D6) — tear the session down rather
    // than leave it open across a failure this process did not understand.
    console.log(`[runtime] session error mid-turn: ${sanitizeLog(e.message)} — tearing the session down`);
    closeLiveSession();
    return { unrecoverable: true, status: 'failed', result: `session error: ${e.message}` };
  }

  if (outcome.initTools) {
    logUngrantedTools(allowed, outcome.initTools, outcome.initMcpServers, (m) => console.log(sanitizeLog(m)));
  }

  if (outcome.endedUnexpectedly) {
    // The generator ended with no result for this push — the live process
    // is gone. D6: the unit is reported failed, the handle recorded stands,
    // and the NEXT unit resumes from disk via the ordinary open path.
    console.log('[runtime] session ended unexpectedly mid-turn — tearing it down');
    closeLiveSession();
    return {
      unrecoverable: true,
      status: 'failed',
      ...(outcome.sessionId ? { sessionId: outcome.sessionId } : {}),
      result: 'the live session ended unexpectedly before this run produced a result — see the pod log',
    };
  }

  return completedTurnReport(outcome, watch, calls);
}

// completedTurnReport turns a settled turn (one that reached its `result`)
// into the reported outcome.
function completedTurnReport(outcome, watch, calls) {
  if (outcome.spinVerdict) {
    return {
      status: 'failed',
      ...(outcome.sessionId ? { sessionId: outcome.sessionId } : {}),
      result: spinMessage(outcome.spinVerdict),
      ...calls.report(),
    };
  }

  const r = outcome.result;
  const isError = Boolean(r?.is_error);
  const extracted = extractBrief(r?.result || '');
  let resultText = extracted.text.slice(0, 2000);
  if (watch.total > 0) {
    // Recovered on its own, the common case — abandoning the tool and
    // answering from what the session already held. The log line is for
    // whoever operates this; the notice on the answer is for whoever asked,
    // because the answer does not say it.
    console.log(`[runtime] ${watch.total} tool call(s) never ran — the arguments were not valid JSON`);
    const notice = discardedNotice(watch);
    if (notice && resultText) resultText = `${resultText}\n\n${notice}`;
  }
  return {
    status: isError ? 'failed' : 'succeeded',
    ...(outcome.sessionId ? { sessionId: outcome.sessionId } : {}),
    result: resultText,
    ...(extracted.brief ? { brief: extracted.brief } : {}),
    ...calls.report(),
  };
}

// storageOutageReport is the shared shape of a resume failure the manager
// must read as "no continuity", never as a quiet substitute for one — see
// `continuity: 'unavailable'` in docs/contracts/runtime.md. No `exitCode`:
// there is no process exit to report here.
function storageOutageReport(first, reason, resultText) {
  return {
    status: 'failed',
    ...(first.sessionId ? { sessionId: first.sessionId } : {}),
    continuity: 'unavailable',
    continuityReason: reason,
    result: resultText,
  };
}

/**
 * checkContextPresence re-checks after short delays, distinguishing three
 * outcomes that look identical from one failed resume attempt: the context
 * REAPPEARING (`'present'`, the store was merely slow), confirmed absence
 * after the full ladder (`'missing'`), and the store itself failing to
 * answer at all (`'unavailable'`) — an I/O error is NOT a "not found", and
 * reading it as one is exactly the conflation this exists to prevent.
 *
 * Bounded and short on purpose: a person is waiting on a chat reply, so the
 * budget is seconds. Long enough for a share-manager restart or an attribute
 * cache to settle; nowhere near long enough to read as a hung reply. An
 * unreadable store fails this check immediately instead of walking the rest
 * of the ladder — waiting out a fixed I/O error changes nothing about it.
 */
async function checkContextPresence(contextId) {
  if (!contextId) return 'missing';
  for (const waitMs of [500, 1500, 3000]) {
    await new Promise((r) => setTimeout(r, waitMs));
    try {
      if (await sessionFileExists(SESSIONS_DIR, contextId)) return 'present';
    } catch {
      return 'unavailable';
    }
  }
  return 'missing';
}

/**
 * sessionFileExists looks for a context's transcript anywhere under dir.
 *
 * A directory that simply does not exist (ENOENT) reads as "not found" —
 * legitimate when a context was never stored here. Any other error (EACCES,
 * EIO, ESTALE, a stale NFS handle) is NOT "not found" and is left to
 * propagate, so the caller can tell a store that answered "no" from one that
 * could not answer at all.
 */
async function sessionFileExists(dir, contextId) {
  let entries;
  try {
    entries = await fs.promises.readdir(dir, { withFileTypes: true });
  } catch (e) {
    if (e.code === 'ENOENT') return false;
    throw e;
  }
  for (const e of entries) {
    if (e.isDirectory()) {
      if (await sessionFileExists(`${dir}/${e.name}`, contextId)) return true;
    } else if (e.name.includes(contextId)) {
      return true;
    }
  }
  return false;
}

// SESSIONS_DIR is where this runtime keeps conversation context. It is
// claude-code's layout, and it is the ONLY component that knows it — the
// manager stores an opaque handle and assumes nothing about where it points.
const SESSIONS_DIR = `${process.env.HOME || '/data/context'}/.claude/projects`;

// contextIdOf reads the handle the manager sent. Prefers the current name and
// falls back to the retired one for one release, so this image works against a
// manager on either side of the rename.
function contextIdOf(unit) {
  return unit.runtimeContextId || unit.resumeSessionId || '';
}

/** strip removes the internal stderr capture from what is reported — kept
 * for parity with the pre-SDK shape, even though no path here sets stderr. */
function strip({ stderr, ...rest }) {
  return rest
}

// readPrompt resolves the unit's prompt: inline text, else the templated
// file under the workspace. Returns `{ prompt }` or `{ failure }`.
function readPrompt(unit) {
  let prompt = unit.promptText || '';
  if (!prompt && unit.promptFile) {
    try {
      const file = safeJoin(WORKSPACE, unit.promptFile);
      if (!file) throw new Error('promptFile escapes the workspace');
      prompt = fs.readFileSync(file, 'utf8');
      for (const [k, v] of Object.entries(unit.promptVars || {})) {
        prompt = prompt.replaceAll(`{{${k}}}`, v);
      }
    } catch (e) {
      // No process ever ran — exitCode is OMITTED rather than a stand-in
      // like -1, which would read as a real (if unusual) process exit.
      return { failure: { status: 'failed', result: `prompt read: ${e.message}` } };
    }
  }
  if (!prompt) return { failure: { status: 'failed', result: 'empty prompt' } };
  return { prompt };
}

// executeUnit composes the allowlist and MCP config, decides open/reuse/
// re-open (D4), runs the turn, and — on a resume attempt that did not land —
// walks the SAME continuity ladder the pre-SDK shape used for its resume
// retry (D5/6.2), with the SAME user-facing failure text on confirmed
// absence (6.3).
async function executeUnit(unit) {
  const read = readPrompt(unit);
  if (read.failure) return read.failure;
  const { prompt } = read;

  // The work unit carries the WIRING's tools and the mode; the agent's own
  // definition carries the rest. Re-read fresh every unit — NOT cached
  // across a reused session — since the allowlist is excluded from the
  // fingerprint precisely so a wiring edit heals without a re-open (D4).
  const declared = agentDeclaredTools(WORKSPACE, unit.agent, (m) => console.log(sanitizeLog(m)));
  const allowed = composeAllowedTools(declared, unit.allowedTools, unit.toolsMode);
  const mcpServers = readMcpServers(MCP_CONFIG, (m) => console.log(sanitizeLog(m)));
  const fingerprint = computeFingerprint(unit, mcpServers);
  const contextId = contextIdOf(unit);
  const isResumeAttempt = Boolean(contextId);

  console.log(`\n[runtime] run ${sanitizeLog(unit.runId)}${contextId ? ' continue=' + sanitizeLog(contextId) : ''} thread=${sanitizeLog(unit.threadId ?? 'general')}`);
  console.log(`[runtime] tools agent=${sanitizeLog(unit.agent || '-')} declared=${declared.length} wiring=${(unit.allowedTools || '').split(',').filter(Boolean).length} mode=${sanitizeLog(unit.toolsMode || 'merge')} -> ${sanitizeLog(allowed.length ? allowed.join(',') : '(none)')}`);
  if (unit.systemPrompt) console.log(`[runtime] appending system prompt (${unit.systemPrompt.length} chars)`);

  openOrReuseSession(unit, fingerprint, allowed, mcpServers);

  const first = await runTurnOnLiveSession(prompt, allowed);
  if (first.unrecoverable) {
    const { unrecoverable, ...rest } = first;
    return rest;
  }

  if (!isResumeAttempt) return strip({ ...first, continuity: 'new' });
  if (resumeSucceeded(first)) return strip({ ...first, continuity: 'continued' });

  // GONE, NOT ANSWERING, or UNREADABLE? The three look identical from one
  // attempt and must not be treated alike: a shared filesystem can fail to
  // answer for seconds — a restarting share-manager, a stale handle after
  // this pod moved, a listing that has not yet seen a file another node
  // wrote — and ending a conversation over a lag of that kind would turn a
  // storage nicety into a correctness bug. An answer of "not there" is a
  // loss; the store itself erroring is an OUTAGE and must never be read as
  // either "not there" or "continued".
  const presence = await checkContextPresence(contextId);

  if (presence === 'unavailable') {
    const reason =
      `the stored context for this conversation could not be read under ${SESSIONS_DIR} — the store ` +
      'itself errored rather than answering "not found" (a stale mount, a permission error, or a share ' +
      'manager that has not come back)';
    console.log(`[runtime] ${reason} — reporting an outage, not a loss`);
    return storageOutageReport(
      first,
      reason,
      '⚠️ **This conversation cannot be continued right now.**\n\n' +
        'Its stored context could not be read — that looks like a storage outage, not a lost conversation. ' +
        `Reason: ${reason}`,
    );
  }

  if (presence === 'present') {
    console.log('[runtime] context reappeared on re-check — the store was slow, not empty');
    closeLiveSession();
    liveSession = openSession(unit, fingerprint, allowed, mcpServers);
    const retried = await runTurnOnLiveSession(prompt, allowed);
    if (retried.unrecoverable) {
      const { unrecoverable, ...rest } = retried;
      return rest;
    }
    if (resumeSucceeded(retried)) return strip({ ...retried, continuity: 'continued' });
    // The store answered present, yet the vendor still could not resume it —
    // report exactly what happened, with no continuity claim of a recovery
    // that did not occur. An absent `continuity` is a claim of nothing, never
    // a lost context (docs/contracts/runtime.md).
    return strip(retried);
  }

  // presence === 'missing': confirmed gone, not merely slow to answer.
  // DO NOT answer without it: a conversation without its context is not that
  // conversation, it is a new one wearing the same name and thread. Answering
  // anyway presents the second as the first to the person replying, and an
  // agent asked to undo something it has no memory of will guess. Fail, say
  // why, and spend no second invocation on an answer that should not exist.
  const reason =
    'the stored context for this conversation could not be reached — no session files under ' +
    `${SESSIONS_DIR} (is /data/context backed by a volume? without one it dies with the pod)`;
  console.log(`[runtime] ${reason} — failing rather than answering without it`);
  // NEVER an empty result. Failing with nothing to read is precisely why this
  // path used to answer without context instead, and reintroducing it as the
  // fix would be the same mistake wearing the opposite decision.
  return storageOutageReport(
    first,
    reason,
    '⚠️ **This conversation cannot be continued.**\n\n' +
      'Its stored context is no longer available, so answering now would mean answering with no ' +
      'memory of what came before — including anything already done. Start a new conversation to ' +
      `continue.\n\nReason: ${reason}`,
  );
}

// module.exports precedes the main loop so a test can `require()` this file
// for its pure/subprocess-driving pieces without the IIFE below ever running
// — that only happens when this file is the process entry point.
module.exports = {
  gitEnv, repoURL, run, clearDir, syncRepo, formatEvent,
  checkContextPresence, sessionFileExists, contextIdOf, strip, extractBrief,
  SESSIONS_DIR, WORKSPACE,
  // the SDK-import seam
  getQuery, __setQueryForTest,
  // streaming input
  makeInputChannel, makeSdkUserMessage,
  // MCP config
  readMcpServers, expandEnvPlaceholders,
  // fingerprint (D4)
  FINGERPRINT_FIELDS, computeFingerprint, changedFingerprintFields, fingerprintsEqual, normalizedMaxTurns,
  // permissions (D3)
  allowlistMatch, createCanUseTool,
  // session lifecycle
  openSession, closeLiveSession, openOrReuseSession, consumeTurn, runTurnOnLiveSession, resumeSucceeded,
  // the unit executor
  readPrompt, executeUnit,
  // test-only introspection of the module-level live session
  __getLiveSessionForTest: () => liveSession,
  __resetLiveSessionForTest: () => { liveSession = null; },
};

// Extracted to a NAMED top-level function rather than left as an inline
// IIFE body: SonarCloud's javascript:S3776 (cognitive complexity) charges a
// nested function literal's control flow to the ENCLOSING function, so
// wrapping it in a closure (or `t.Run`-style callback) does not reset the
// score. A plain call is not a nesting-increasing construct, so the guarded
// `void runLoop()` below costs nothing -- MEASURED: adding `void` to the
// previous inline IIFE (for javascript:S9383, above) touched this
// function's first line and made SonarCloud count its PRE-EXISTING
// complexity (30, against a limit of 15) as new code, failing
// new_maintainability_rating on a change that added no control flow here.
// A `fetch` that cannot reach $CONTROL_URL, or a long-poll that came back
// empty, is ordinary — the caller just polls again. `undefined` means
// "nothing to do this iteration", never an error.
async function pollNextUnit() {
  let res;
  try {
    res = await fetch(`${CONTROL_URL}/work?convo=${encodeURIComponent(CONVO_ID)}&pod=${encodeURIComponent(POD_NAME)}&wait=25`);
  } catch { await sleep(5000); return undefined; }
  if (res.status === 204) return undefined;
  if (!res.ok) { await sleep(5000); return undefined; }
  try { return await res.json(); } catch { return undefined; }
}

// isDefinitiveRejection names the one class of /work/done response a retry
// cannot fix: the manager read the report and refused its content, so
// resending the same bytes would get the same verdict. 408 (request
// timeout) and 429 (rate limit) are excluded — those say "try again", not
// "this is wrong" — and every 5xx/network failure is transient by
// definition.
function isDefinitiveRejection(status) {
  return status >= 400 && status < 500 && status !== 408 && status !== 429;
}

// Retries for up to ten minutes (60 * 10s) rather than failing the run: the
// manager's own outage must not discard a result claude-code already
// produced. `continuity` rides along from executeUnit — the manager cannot
// infer it, since it sends a handle and gets a handle back.
//
// A DEFINITIVE 4xx is the one outcome that ends the loop early: the manager
// has already read and refused the report, so spending the rest of the
// ten-minute budget resending identical bytes only delays the next poll. The
// refusal is logged with its status and body — the one place a bad report is
// diagnosable from the pod log — and the run is never retried, never dropped
// silently.
async function reportDone(unit, out) {
  // Report the handle under BOTH names for one release: the current one,
  // and the retired spelling so this image also works against an older
  // manager.
  const { sessionId, ...rest } = out;
  const done = {
    convo: CONVO_ID,
    runId: unit.runId,
    ...rest,
    ...(sessionId ? { runtimeContextId: sessionId, sessionId } : {}),
  };
  let lastFailure = 'no attempt reached the manager';
  for (let i = 0; i < 60; i++) {
    try {
      const r = await fetch(`${CONTROL_URL}/work/done`, {
        method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(done),
      });
      if (r.ok) return;
      if (isDefinitiveRejection(r.status)) {
        const body = await r.text().catch(() => '(body unreadable)');
        console.error(`[runtime] /work/done refused run ${sanitizeLog(unit.runId)}: ${r.status} ${sanitizeLog(body).slice(0, 500)} — not retried, resuming polling`);
        return;
      }
      lastFailure = `status ${r.status}`;
    } catch (e) {
      lastFailure = e.message;
    }
    await sleep(10000);
  }
  console.error(`[runtime] /work/done never delivered run ${sanitizeLog(unit.runId)} after 10 minutes of retries (${sanitizeLog(lastFailure)}) — giving up, resuming polling`);
}

// Exported alongside the module.exports object above rather than folded
// into it: both are declared below that object (function hoisting makes
// either position work), and a second, small assignment here keeps them
// visually beside the loop code they test.
module.exports.pollNextUnit = pollNextUnit;
module.exports.reportDone = reportDone;
module.exports.isDefinitiveRejection = isDefinitiveRejection;

// A sync failure here (a bad ref, an unreachable origin, a rejected
// fetch/reset) must FAIL THIS RUN with that reason — proceeding to
// executeUnit anyway would run the agent against a missing or stale
// checkout and report whatever confusion that produces, attributing
// nothing to the real cause.
async function runUnit(unit) {
  try {
    await syncRepo();
    return await executeUnit(unit);
  } catch (e) {
    console.error(`[runtime] sync: ${e.message}`);
    return { status: 'failed', result: `workspace sync failed: ${e.message}` };
  }
}

async function runLoop() {
  console.log(`[runtime] claude runtime — convo=${CONVO_ID} pod=${POD_NAME} ttl=${TTL_MS / 60000}m workspace=${WORKSPACE}`);
  try { await syncRepo(); } catch (e) { console.error(`[runtime] initial sync: ${e.message}`); }

  let lastWork = Date.now();
  let idle = false;
  while (!idle) {
    if (Date.now() - lastWork > TTL_MS) {
      console.log('[runtime] idle TTL reached — exiting');
      closeLiveSession();
      idle = true;
      continue;
    }
    const unit = await pollNextUnit();
    if (!unit) continue;
    lastWork = Date.now();
    // Strictly serial per conversation: one inflight unit, so the await in
    // the loop is the invariant, not an oversight.
    const out = await runUnit(unit); // NOSONAR
    lastWork = Date.now();
    await reportDone(unit, out); // NOSONAR
  }
  process.exit(0);
}

if (require.main === module) {
  // javascript:S7785 wants top-level `await` here instead. Real top-level
  // await is an ESM feature and is a SyntaxError in a CommonJS module (this
  // file has no package.json, so CommonJS is Node's default) -- converting
  // this file and everything it requires to ESM is a far larger change than
  // this line justifies. NOSONAR rather than a real fix.
  void runLoop(); // NOSONAR
}
