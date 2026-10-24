// agentops claude agent runtime — the SDK driver: one `query()` session held
// open per pod, fed one work unit at a time through a streaming-input
// channel, implementing the SAME AgentRuntime /work contract driver-cli.js
// does:
//   1. long-poll  GET  $CONTROL_URL/work?convo=$CONVO_ID&pod=$POD_NAME&wait=25
//   2. feed the unit into the open session (opening or re-opening one when
//      needed), streaming a formatted transcript to STDOUT
//   3. report     POST $CONTROL_URL/work/done
//   4. exit 0 after RUNTIME_IDLE_TTL_M minutes without work
//
// Selected by RUNTIME_CLAUDE_DRIVER=sdk (the default) — runtime.js is the
// thin entrypoint that reads that var and chooses between this driver and
// the `cli` one (driver-cli.js, one `claude` CLI spawn per unit). This file
// is never the process entry point itself; runtime.js calls start() below.
//
// What changes relative to driver-cli.js, and why (openspec/changes/
// claude-runtime-sdk/design.md, decisions D1-D7):
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
//        session id, and the continuity ladder (reused unchanged from
//        driver-cli.js) governs the re-open path exactly as it governs the
//        cli driver's resume-retry path.
//   D6 — the session is torn down on idle-TTL exit, on a fingerprint change
//        (before the replacement opens), and on an unattributable session
//        failure (the generator ending mid-turn, or throwing).
//   D7 — the transcript renders identically to driver-cli.js's, because
//        formatEvent's field reads (model/tools/mcp_servers on init,
//        content blocks on assistant, subtype/num_turns/duration_ms/result
//        on result) line up with the SDK's typed messages — so this driver
//        reuses that exact function rather than reimplementing it.
'use strict';

const fs = require('node:fs');
const {
  syncRepo, checkContextPresence, contextIdOf, extractBrief, formatEvent,
  SESSIONS_DIR, WORKSPACE, pollNextUnit, reportDone,
} = require('./driver-cli');
const { agentDeclaredTools, composeAllowedTools, safeJoin, sanitizeLog, logUngrantedTools } = require('./tools');
const { DEFAULT_LIMIT, newSpinWatch, noteToolUse, spinMessage, discardedNotice } = require('./spin');
const { newCallRecorder } = require('./report');

const CONTROL_URL = process.env.CONTROL_URL || '';
const CONVO_ID = process.env.CONVO_ID || '';
const POD_NAME = process.env.POD_NAME || '';
const TTL_MS = (Number.parseInt(process.env.RUNTIME_IDLE_TTL_M || '10', 10)) * 60 * 1000;
const MCP_CONFIG = process.env.MCP_CONFIG || '/etc/agentops/mcp.json';
// Same floor driver-cli.js passes as `--max-turns` when a unit carries none
// — kept identical so the two drivers behave the same when the profile sets
// no turn budget.
const DEFAULT_MAX_TURNS = 60;
// Same breaker, same default, as driver-cli.js — see spin.js.
const SPIN_LIMIT = (() => {
  const v = Number.parseInt(process.env.RUNTIME_UNPARSED_REPEAT_LIMIT || '', 10);
  return Number.isFinite(v) && v >= 0 ? v : DEFAULT_LIMIT;
})();

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

// ---------------------------------------------------------------------------
// The SDK-import seam. `@anthropic-ai/claude-agent-sdk` is a real npm
// dependency installed globally in the built image (never in this
// directory's — nonexistent — node_modules), so requiring it unconditionally
// at module load would break `node --test` here. `getQuery()` resolves it
// lazily, on first real use; `__setQueryForTest` lets a test substitute a
// scripted fake query() without the real package ever being required.
// ---------------------------------------------------------------------------
let _query = null;
function getQuery() {
  if (!_query) _query = require('@anthropic-ai/claude-agent-sdk').query;
  return _query;
}
function __setQueryForTest(fn) { _query = fn; }

// ---------------------------------------------------------------------------
// Streaming input (design D2's "the poll loop is the generator"). One
// long-lived async generator feeds query()'s `prompt`; push() delivers the
// next SDKUserMessage, resolving a pending read if one is already waiting,
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

// makeSdkUserMessage builds the minimal valid SDKUserMessage for one prompt.
// `session_id` is the field the real type carries on every message but this
// driver never reads back on input — the SDK owns correlating it to the
// open query(), so an empty string is passed rather than inventing one.
function makeSdkUserMessage(promptText) {
  return { type: 'user', message: { role: 'user', content: promptText }, parent_tool_use_id: null, session_id: '' };
}

// ---------------------------------------------------------------------------
// Transcript rendering (D7). The SDK's typed messages carry the same fields
// driver-cli.js's formatEvent already reads off parsed stream-json
// (model/tools/mcp_servers on init, content blocks on assistant,
// subtype/num_turns/duration_ms/result on result) — so this IS that
// function, re-exported under a name that says what feeds it here, rather
// than a second implementation that could drift from the first.
const formatSdkEvent = formatEvent;

// ---------------------------------------------------------------------------
// The composed MCP config, read fresh every unit (never cached): it is
// CONTENT, not a REF, so an operator's edit must reach the fingerprint (D4)
// on the very next unit, exactly as `--mcp-config` would reread it on the
// cli driver's every fresh spawn.
// ---------------------------------------------------------------------------
function readMcpServers(mcpConfigPath, log = () => {}) {
  try {
    const parsed = JSON.parse(fs.readFileSync(mcpConfigPath, 'utf8'));
    return (parsed && typeof parsed.mcpServers === 'object' && parsed.mcpServers) || {};
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
// allowlistMatch reuses the SAME base-name/server-name extraction
// tools.js's logUngrantedTools already demonstrates for cross-checking a
// grant against the vendor's own tool list — NOT a re-implementation of the
// CLI's own sub-command glob matching for a scoped entry like
// `Bash(git *)`. A scoped entry is matched (and therefore granted) at the
// BASE TOOL NAME only here, same as it is for that cross-check. This is a
// deliberate, narrower guarantee than the real CLI's `--allowedTools`
// parser enforces for that one scoped form — flagged for review, see the
// hand-back report.
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
  return async function canUseTool(toolName, input) {
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
    maxTurns: normalizedMaxTurns(unit),
    // Deliberately NEVER `agent: unit.agent` — see the file header. Passing
    // it would re-apply the agent definition's own tool list as a vendor
    // availability intersection, defeating the allowlist tools.js already
    // composed. `unit.agent` is a fingerprint input only (D4).
  };
  const contextId = contextIdOf(unit);
  if (contextId) options.resume = contextId;
  if (unit.systemPrompt) options.systemPrompt = unit.systemPrompt;
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
// took, distinctly, so a pod log shows the active path without inference
// (D7's own last paragraph).
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

// readInit takes the vendor's own account of what it actually has —
// cross-checked against the grant once the turn completes, exactly as
// driver-cli.js does it, the one point in this process that ever sees it.
function readInit(ev) {
  return { initTools: ev.tools || [], initMcpServers: ev.mcp_servers || [] };
}

// abortSpinningTurn is the async equivalent of driver-cli.js's SIGTERM:
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

async function consumeTurn(sdkQuery, watch, calls) {
  let sessionId = null;
  let spinVerdict = null;
  let initTools = null;
  let initMcpServers = null;
  for (;;) {
    const { value: ev, done } = await sdkQuery.next(); // NOSONAR
    if (done) return { endedUnexpectedly: true, sessionId, initTools, initMcpServers };
    if (ev?.session_id && !sessionId) sessionId = ev.session_id;
    if (ev?.type === 'system' && ev.subtype === 'init') ({ initTools, initMcpServers } = readInit(ev));
    calls.note(ev);
    const txt = formatSdkEvent(ev);
    if (txt) process.stdout.write(txt);
    if (ev?.type === 'assistant' && !spinVerdict) {
      spinVerdict = findSpinVerdict(ev, watch);
      if (spinVerdict) abortSpinningTurn(sdkQuery, spinVerdict);
    }
    if (ev?.type === 'result') return { result: ev, sessionId, spinVerdict, initTools, initMcpServers };
  }
}

// resumeSucceededSdk mirrors driver-cli.js's resumeSucceeded, translated to
// the SDK's typed fields: a session that was found emits a session id even
// on a later failure, so "no session id and no result" is what a missing
// session looks like here too — an `is_error: true` result with nothing
// else to show for it.
function resumeSucceededSdk(first) {
  if (first.status === 'succeeded') return true;
  return !(!first.sessionId && !first.result);
}

// runTurnOnLiveSession pushes one prompt into the already-open/reused/
// re-opened liveSession and reads it to a driver-cli.js-shaped outcome:
// `{status, sessionId?, result, brief?, turns?, toolCalls?}`, never an
// `exitCode` — there is no process exit to report (API-7).
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
// into the driver-cli.js-shaped outcome.
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
    // Recovered on its own, exactly as driver-cli.js observes — abandoning
    // the tool and answering from what the session already held.
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

// storageOutageReportSdk is the SDK driver's own copy of driver-cli.js's
// storageOutageReport shape — duplicated rather than imported because
// driver-cli.js exports no such helper and stays untouched by this change.
// The TEXT is identical on purpose (task 6.3: the existing user-facing text,
// on either driver), and `exitCode` is never set — there is no process exit
// to report here.
function storageOutageReportSdk(first, reason, resultText) {
  return {
    status: 'failed',
    ...(first.sessionId ? { sessionId: first.sessionId } : {}),
    continuity: 'unavailable',
    continuityReason: reason,
    result: resultText,
  };
}

// executeUnit is runUnit's own half of driver-cli.js's runClaude+spawnClaude:
// composes the allowlist and MCP config, decides open/reuse/re-open (D4),
// runs the turn, and — on a resume attempt that did not land — walks the
// SAME continuity ladder driver-cli.js's cli driver uses for its resume
// retry (D5/6.2), with the SAME user-facing failure text (6.3) on confirmed
// absence.
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
      return { failure: { status: 'failed', result: `prompt read: ${e.message}` } };
    }
  }
  if (!prompt) return { failure: { status: 'failed', result: 'empty prompt' } };
  return { prompt };
}

async function executeUnit(unit) {
  const read = readPrompt(unit);
  if (read.failure) return read.failure;
  const { prompt } = read;

  // The work unit carries the WIRING's tools and the mode; the agent's own
  // definition carries the rest. Re-read fresh every unit — NOT cached
  // across a reused session — since the allowlist is excluded from the
  // fingerprint precisely so a wiring edit heals without a re-open (D4.2).
  const declared = agentDeclaredTools(WORKSPACE, unit.agent, (m) => console.log(sanitizeLog(m)));
  const allowed = composeAllowedTools(declared, unit.allowedTools, unit.toolsMode);
  const mcpServers = readMcpServers(MCP_CONFIG, (m) => console.log(sanitizeLog(m)));
  const fingerprint = computeFingerprint(unit, mcpServers);
  const contextId = contextIdOf(unit);
  const isResumeAttempt = Boolean(contextId);

  console.log(`\n[runtime] run ${sanitizeLog(unit.runId)}${contextId ? ' continue=' + sanitizeLog(contextId) : ''} thread=${sanitizeLog(unit.threadId ?? 'general')}`);
  console.log(`[runtime] tools agent=${sanitizeLog(unit.agent || '-')} declared=${declared.length} wiring=${(unit.allowedTools || '').split(',').filter(Boolean).length} mode=${sanitizeLog(unit.toolsMode || 'merge')} -> ${sanitizeLog(allowed.length ? allowed.join(',') : '(none)')}`);
  if (unit.systemPrompt) console.log(`[runtime] appending system prompt (${sanitizeLog(String(unit.systemPrompt.length))} chars)`);

  openOrReuseSession(unit, fingerprint, allowed, mcpServers);

  const first = await runTurnOnLiveSession(prompt, allowed);
  if (first.unrecoverable) {
    const { unrecoverable, ...rest } = first;
    return rest;
  }

  if (!isResumeAttempt) return { ...first, continuity: 'new' };
  if (resumeSucceededSdk(first)) return { ...first, continuity: 'continued' };

  // GONE, NOT ANSWERING, or UNREADABLE? Same three outcomes driver-cli.js's
  // own comment on this distinguishes, ported unchanged (D5/6.2).
  const presence = await checkContextPresence(contextId);

  if (presence === 'unavailable') {
    const reason =
      `the stored context for this conversation could not be read under ${SESSIONS_DIR} — the store ` +
      'itself errored rather than answering "not found" (a stale mount, a permission error, or a share ' +
      'manager that has not come back)';
    console.log(`[runtime] ${reason} — reporting an outage, not a loss`);
    return storageOutageReportSdk(
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
    if (resumeSucceededSdk(retried)) return { ...retried, continuity: 'continued' };
    // The store answered present, yet the vendor still could not resume it —
    // report exactly what happened, no continuity claim of a recovery that
    // did not occur.
    return retried;
  }

  // presence === 'missing': confirmed gone, not merely slow to answer.
  const reason =
    'the stored context for this conversation could not be reached — no session files under ' +
    `${SESSIONS_DIR} (is /data/context backed by a volume? without one it dies with the pod)`;
  console.log(`[runtime] ${reason} — failing rather than answering without it`);
  return storageOutageReportSdk(
    first,
    reason,
    '⚠️ **This conversation cannot be continued.**\n\n' +
      'Its stored context is no longer available, so answering now would mean answering with no ' +
      'memory of what came before — including anything already done. Start a new conversation to ' +
      `continue.\n\nReason: ${reason}`,
  );
}

// A sync failure here must FAIL THIS RUN with that reason, same rule as
// driver-cli.js's own runUnit.
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
  console.log(`[runtime] claude runtime (sdk) — convo=${CONVO_ID} pod=${POD_NAME} ttl=${TTL_MS / 60000}m workspace=${WORKSPACE}`);
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
    const unit = await pollNextUnit(); // NOSONAR
    if (!unit) continue;
    const out = await runUnit(unit); // NOSONAR
    lastWork = Date.now();
    await reportDone(unit, out); // NOSONAR
  }
  process.exit(0);
}

// start is what runtime.js (the entrypoint) calls once it has resolved
// RUNTIME_CLAUDE_DRIVER to `sdk` (the default).
function start() {
  if (!CONTROL_URL || !CONVO_ID) {
    console.error('[runtime] CONTROL_URL and CONVO_ID are required');
    process.exit(1);
  }
  return runLoop(); // NOSONAR
}

module.exports = {
  // the SDK-import seam
  getQuery, __setQueryForTest,
  // streaming input / transcript
  makeInputChannel, makeSdkUserMessage, formatSdkEvent,
  // MCP config
  readMcpServers,
  // fingerprint (D4)
  FINGERPRINT_FIELDS, computeFingerprint, changedFingerprintFields, fingerprintsEqual, normalizedMaxTurns,
  // permissions (D3)
  allowlistMatch, createCanUseTool,
  // session lifecycle
  openSession, closeLiveSession, openOrReuseSession, consumeTurn, runTurnOnLiveSession, resumeSucceededSdk,
  // the loop
  executeUnit, runUnit, runLoop, start,
  // test-only introspection of the module-level live session
  __getLiveSessionForTest: () => liveSession,
  __resetLiveSessionForTest: () => { liveSession = null; },
  // shared config, for a test asserting this driver reads the same env the
  // way driver-cli.js does
  CONTROL_URL, CONVO_ID, POD_NAME, WORKSPACE, MCP_CONFIG, SPIN_LIMIT,
};
