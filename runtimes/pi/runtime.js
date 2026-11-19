// agentops pi agent runtime — implements the AgentRuntime /work contract:
//   1. long-poll  GET  $CONTROL_URL/work?convo=$CONVO_ID&pod=$POD_NAME&wait=25
//   2. run the pi coding agent (through its SDK, in-process) against the
//      profile's repo checkout, streaming a formatted transcript to STDOUT
//   3. report     POST $CONTROL_URL/work/done
//   4. exit 0 after RUNTIME_IDLE_TTL_M minutes without work
//
// THE FOURTH RUNTIME, and the second (after copilot) whose vendor owns its
// own tool vocabulary and its own session store. Both are translated HERE —
// vocabulary.js, tools.js — and neither reaches a CRD. A Pipeline binds the
// same toolsets it binds for every other runtime.
//
// THE CONTEXT HANDLE IS MINTED HERE, never pi's own. Confirmed live
// (design.md's spike S3): `SessionManager.create(cwd, sessionDir, {id})`
// mints a session whose own id IS the id this process generated, and
// `SessionManager.findById`/`.open` resolve it back — no id-to-session
// mapping file needed, unlike D5's original assumption. Each context gets
// its OWN sessionDir under SESSIONS_ROOT, which is $HOME/.pi/agent/sessions
// by default — the CONTEXT volume, or the pod-local copy context-sync
// restores. The bundle declares that path; this file assumes nothing else
// about it.
//
// INTEGRATION POINTS FOR LATER SECTIONS OF THIS CHANGE, left deliberately
// visible rather than stubbed out:
//   - section 3 (mcp.js): wired. buildResourceLoader's extensionFactories
//     array carries mcp.js's registerMcpTools beside the tool_call-permission
//     factory this file already registers — grant.mcpPatterns (from
//     vocabulary.translate) is what it resolves against.
//   - section 4 (report.js): wired. `attempt()`'s `state.calls` is a
//     `newCallRecorder()`, fed by `onEvent` beside the transcript
//     bookkeeping it already does, and its `report()` is spread into
//     `attempt()`'s return value alongside `result`/`brief`.

'use strict';

const { spawn } = require('child_process');
const crypto = require('crypto');
const fs = require('fs');
const path = require('path');
const { readAgentDefinition, composeAllowedTools, safeJoin, sanitizeLog } = require('./tools');
const { translate, decide, checkInventory } = require('./vocabulary');
const { confirmContextMissing } = require('./continuity');
const { registerMcpTools } = require('./mcp');
const { newCallRecorder } = require('./report');
const { loaderOptions: piCodeLoaderOptions, piCodeEnabled } = require('./picode');

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
// MCP_CONFIG is read by mcp.js's loadMcpServers, through buildResourceLoader's
// registerMcpTools call below — declared here, beside every other env fact
// this runtime reads, rather than left for mcp.js alone to default silently.
const MCP_CONFIG = process.env.MCP_CONFIG || '/etc/agentops/mcp.json';
const HOME = process.env.HOME || '/data/context';
// PI_AGENT_DIR is the SDK's own `agentDir`: auth.json, models.json and the
// models-store cache live there (spike S4). Set explicitly so the path the
// bundle declares to context-sync and the path the runtime writes are one
// string, decided in one place — same reasoning as copilot's COPILOT_HOME.
const PI_AGENT_DIR = process.env.PI_AGENT_DIR || path.join(HOME, '.pi', 'agent');
// SESSIONS_ROOT holds one subdirectory per minted runtimeContextId (S3/S4),
// bypassing pi's own cwd-based path encoding entirely.
const SESSIONS_ROOT = path.join(PI_AGENT_DIR, 'sessions');
// Provider/model/credential are env (D7) — THIS runtime's own naming, since
// neither the chart bundle (section 6) nor the specification (section 7) had
// run yet when this was written. Flagged in the hand-back report as a
// cross-lane interface a later dispatch must agree with, not invent a second
// time.
const PI_PROVIDER = process.env.PI_PROVIDER || 'default';
const PI_PROVIDER_API = process.env.PI_PROVIDER_API || '';
const PI_BASE_URL = process.env.PI_BASE_URL || '';
const PI_MODEL = process.env.PI_MODEL || '';
const PI_API_KEY = process.env.PI_API_KEY || '';
// One run's wall-clock ceiling. pi's `session.prompt()` takes no timeout of
// its own (confirmed against PromptOptions) — unlike copilot's
// sendAndWait(prompt, timeoutMs), so this is enforced with a race rather than
// a parameter.
const RUN_TIMEOUT_MS = (Number.parseInt(process.env.PI_RUN_TIMEOUT_S || '3600', 10)) * 1000;

// Guarded on require.main so this file can be `require()`d by its own test
// suite -- coverage's only way to reach the functions below -- without also
// exiting the test process, installing real signal handlers on it, or
// starting the network loop at the bottom.
if (require.main === module) {
if (!CONTROL_URL || !CONVO_ID) {
  console.error('[runtime] CONTROL_URL and CONVO_ID are required');
  process.exit(1);
}
if (!PI_API_KEY) {
  // Fail at START, not on the first run: a missing credential is a wiring
  // fact, and a pod that polls for work it can never do looks healthy.
  console.error('[runtime] PI_API_KEY is required (the bundle projects it from its credential Secret)');
  process.exit(1);
}
if (!PI_MODEL) {
  console.error('[runtime] PI_MODEL is required (no model, nothing to resolve a session against)');
  process.exit(1);
}
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

// withTimeout races `promise` against a bound, clearing the timer in EVERY
// case — won, lost, or thrown. `Promise.race` alone leaves the loser's timer
// armed: a `setTimeout(ms)` that is never cleared keeps the event loop (and
// so the process) non-idle for the FULL `ms`, even after the race is
// decided. Harmless under a bare `node` invocation ending in
// `process.exit()`, but it stalls anything that waits for the event loop to
// drain on its own — `node --test` among them, measured live against a real
// session (an hour-long dangling timer from the default RUN_TIMEOUT_MS,
// silently eaten by `Promise.race`).
async function withTimeout(promise, ms, message) {
  let timer;
  const bound = new Promise((_resolve, reject) => { timer = setTimeout(() => reject(new Error(message)), ms); });
  try {
    return await Promise.race([promise, bound]);
  } finally {
    clearTimeout(timer);
  }
}

// ---- repo checkout ----------------------------------------------------------

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
  if (!REPO_URL) { fs.mkdirSync(WORKSPACE, { recursive: true }); return; } // MGR-5
  if (!fs.existsSync(path.join(WORKSPACE, '.git'))) {
    fs.mkdirSync(WORKSPACE, { recursive: true });
    clearDir(WORKSPACE);
    const r = await run('git', ['clone', '--depth', '1', '-b', REPO_REF, repoURL(), '.'], { cwd: WORKSPACE, env: gitEnv() });
    if (r.code !== 0) throw new Error(`git clone: ${r.err.slice(-400)}`);
  } else {
    const fetch = await run('git', ['fetch', '--depth', '1', 'origin', REPO_REF], { cwd: WORKSPACE, env: gitEnv() });
    if (fetch.code !== 0) throw new Error(`git fetch: ${fetch.err.slice(-400)}`);
    const reset = await run('git', ['reset', '--hard', `origin/${REPO_REF}`], { cwd: WORKSPACE, env: gitEnv() });
    if (reset.code !== 0) throw new Error(`git reset: ${reset.err.slice(-400)}`);
  }
}

// ---- the SDK client ----------------------------------------------------------

let sdk = null;
let cachedModelCtx = null;

// Loaded lazily so the unit tests can require the modules beside this file
// without the SDK ever importing — pi is ESM-only (no `require()` entry at
// all, confirmed live), and a test suite that pays its import cost on every
// `require('./runtime')` is a test suite that runs slower for no reason most
// of its own tests need.
async function getSdk() {
  if (!sdk) sdk = await import('@earendil-works/pi-coding-agent');
  return sdk;
}

// getModelContext resolves the provider/model/credential env into a live
// `ModelRuntime` and the `Model` object every session is created against.
// File-based (`ModelRuntime.create({modelsPath, authPath})`), never
// `registerProvider()` — the latter's model schema requires `name`/`input`/
// `cost` even for a minimal entry (confirmed live, crashes otherwise), while
// the file schema tolerates a bare `{id}`, which is all this runtime ever has
// to offer for a BYOK/self-hosted endpoint.
async function getModelContext() {
  if (cachedModelCtx) return cachedModelCtx;
  const s = await getSdk();
  fs.mkdirSync(PI_AGENT_DIR, { recursive: true });
  const modelsPath = path.join(PI_AGENT_DIR, 'models.json');
  const authPath = path.join(PI_AGENT_DIR, 'auth.json');
  const providerConfig = {
    apiKey: PI_API_KEY,
    ...(PI_PROVIDER_API ? { api: PI_PROVIDER_API } : {}),
    ...(PI_BASE_URL ? { baseUrl: PI_BASE_URL } : {}),
    models: [{ id: PI_MODEL }],
  };
  fs.writeFileSync(modelsPath, JSON.stringify({ providers: { [PI_PROVIDER]: providerConfig } }));
  const modelRuntime = await s.ModelRuntime.create({ modelsPath, authPath, allowModelNetwork: false });
  const model = modelRuntime.getModel(PI_PROVIDER, PI_MODEL);
  if (!model) throw new Error(`model "${PI_MODEL}" not found under provider "${PI_PROVIDER}" — check PI_PROVIDER/PI_MODEL`);
  cachedModelCtx = { modelRuntime, model };
  return cachedModelCtx;
}

// logInventoryGaps cross-checks the built-in names this run granted against
// what the live session actually registered (design's "Risks" row: a vendor
// name that moves must surface on every run, not just the first).
function logInventoryGaps(session, builtinNames) {
  try {
    const gaps = checkInventory(builtinNames, session.getActiveToolNames());
    for (const gap of gaps) console.log(`[runtime] WARNING: ${gap}`);
  } catch (e) {
    console.log(`[runtime] built-in inventory check skipped: ${e.message}`);
  }
}

// ---- transcript ---------------------------------------------------------------

// MAX_BRIEF mirrors ConversationStatus.Brief's MaxLength — bounding here
// means a value this runtime reports never gets silently truncated by the
// manager mid-sentence.
const MAX_BRIEF = 512;

// extractBrief pulls a standalone `<brief>...</brief>` block (format.md) out
// of the agent's final text, mirroring the block grammar's own recognition
// rule — the tag alone on its own line — closely enough for this one tag.
// `brief` is a CONTRACT FIELD, never a displayed section, so it is removed
// from what becomes `result` (same rule, same shape, as every other
// runtime). Inlined rather than required from a report.js this change does
// not create in this section — claude's runtime.js keeps the same function
// inline rather than in its own report.js, the precedent this follows.
function extractBrief(text) {
  const m = /^<brief>[ \t]*\r?\n([\s\S]*?)\r?\n^<\/brief>[ \t]*(?:\r?\n)*/m.exec(text);
  if (!m) return { text, brief: '' };
  const brief = m[1].trim().replace(/\s+/g, ' ').slice(0, MAX_BRIEF);
  return { text: (text.slice(0, m.index) + text.slice(m.index + m[0].length)).trim(), brief };
}

// onEvent folds pi's AgentSession events into one run's transcript/state.
// Kept to a `switch` over confirmed event `type`s (spike S5) rather than a
// generic dump, so an event shape this runtime does not expect is ignored
// instead of corrupting the transcript.
function onEvent(ev, state) {
  try {
    // section 4 (report.js): a recorder shaped like runtimes/copilot's own
    // `newCallRecorder` plugs into this same event stream, beside (never
    // instead of) the transcript bookkeeping below — guarded on `state.calls`
    // so the existing onEvent unit tests, which construct a bare transcript
    // state with no recorder, are untouched.
    if (state.calls) state.calls.note(ev);
    switch (ev.type) {
      case 'turn_end': {
        state.turns++;
        const msg = ev.message || {};
        if (msg.stopReason === 'error') {
          state.errors.push(String(msg.errorMessage || 'turn error'));
          process.stdout.write(`[error] ${state.errors[state.errors.length - 1]}\n`);
          return;
        }
        const text = (msg.content || [])
          .filter((c) => c && c.type === 'text' && typeof c.text === 'string')
          .map((c) => c.text)
          .join('');
        if (text) {
          state.lastText = text;
          process.stdout.write(`[pi] ${text}\n`);
        }
        return;
      }
      case 'tool_execution_start': {
        state.toolCalls++;
        const args = JSON.stringify(ev.args !== undefined ? ev.args : {});
        process.stdout.write(`[tool] ${sanitizeLog(ev.toolName)} ${args.length > 160 ? args.slice(0, 160) + '…' : args}\n`);
        return;
      }
      case 'tool_execution_end':
        if (ev.isError) {
          process.stdout.write(`[tool] ${sanitizeLog(ev.toolName)} failed: ${String(ev.result || '').slice(0, 200)}\n`);
        }
        return;
      default:
        return;
    }
  } catch {
    // a transcript line must never take the run down
  }
}

// ---- one run -------------------------------------------------------------------

function resolvePrompt(unit) {
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
      return { error: `prompt read: ${e.message}` };
    }
  }
  if (!prompt) return { error: 'empty prompt' };
  return { prompt };
}

// contextIdOf reads the handle the manager sent. Prefers the current name and
// falls back to the retired one for one release.
function contextIdOf(unit) {
  return unit.runtimeContextId || unit.resumeSessionId || '';
}

// buildResourceLoader is task 2.6/2.7/2.9's one assembly point: the system
// prompt (the agent definition's body, then the unit's own `systemPrompt`,
// APPENDED per D4/S2 — resupplied on every call since neither override
// persists across a resume), pi's own AGENTS.md/CLAUDE.md auto-load turned
// OFF (task 2.9, S6), and the permission decision for every `tool_call`
// (task 2.7) answered from the composed allowlist, deny by default, with no
// path to an interactive prompt.
// `mcpHandle`, when the caller passes its own `{ toolNames, clients }`
// (mcp.js's own shape), is filled with every `mcp__<server>__<tool>` name
// the MCP extension actually registered and every `McpClient` it
// connected — `await loader.reload()` below runs that extension factory to
// completion first, so by the time this function returns, both lists are
// the caller's own: `toolNames` is what `openSession` must fold into its
// `tools:` option (`pi.registerTool()` alone never activates a tool on the
// session; see mcp.js's registerMcpTools), and `clients` is what the
// caller must close once this attempt's session ends (`attempt()` below —
// never left open past one run; see mcp.js's own note on why
// `session_shutdown` alone is not enough here).
async function buildResourceLoader(sdk, unit, grant, definitionBody, denials, mcpHandle = { toolNames: [], clients: [] }) {
  const extra = [definitionBody, unit.systemPrompt].filter(Boolean).join('\n\n');
  const extensionFactory = (pi) => {
    pi.on('tool_call', (ev) => {
      // `mcpHandle.toolNames` is read fresh on every call, by reference —
      // `registerMcpTools` (mcp.js) fills it during `loader.reload()`
      // below, before this hook can ever fire, so a correctly-registered
      // `mcp__<server>__<tool>` name is granted exactly like a built-in,
      // never falling through to "not in the composed allowlist" just
      // because `decide()` only ever knew pi's six built-in names.
      const d = decide([...grant.builtinNames, ...mcpHandle.toolNames], ev.toolName);
      if (d.allow) return undefined;
      denials.push(d.why);
      console.log(`[runtime] denied tool_call ${sanitizeLog(ev.toolName)}: ${sanitizeLog(d.why)}`);
      return { block: true, reason: `not permitted by this conversation's tool allowlist: ${d.why}` };
    });
  };
  // section 3: the one extension this image ships beyond the permission
  // hook above — connects only the servers MCP_CONFIG lists and registers
  // each tool grant.mcpPatterns permits, named mcp__<server>__<tool>.
  const mcpExtensionFactory = (pi) => registerMcpTools(pi, MCP_CONFIG, grant.mcpPatterns, undefined, undefined, mcpHandle);
  const loader = new sdk.DefaultResourceLoader({
    cwd: WORKSPACE,
    agentDir: PI_AGENT_DIR,
    // task 2.9 / S6: a checkout's own AGENTS.md/CLAUDE.md is a second,
    // uncomposed prompt the manager never agreed to — turned off entirely.
    agentsFilesOverride: () => ({ agentsFiles: [] }),
    ...(extra ? { appendSystemPromptOverride: (base) => [...base, extra] } : {}),
    extensionFactories: [extensionFactory, mcpExtensionFactory],
    // The OPTIONAL pi-code package (picode.js): reads the checkout's .claude/ as
    // Claude Code does. Empty when the chart's `pi.piCode` is off.
    ...piCodeLoaderOptions(sdk, { agentDir: PI_AGENT_DIR, workspace: WORKSPACE }),
    // task 3.4: load ONLY the two factories above — pi's own built-in mcp
    // extension would otherwise double-register these same mcp__ tool
    // names from its own (unused by this runtime) connection logic.
    disabledBuiltinExtensions: ['mcp'],
  });
  await loader.reload(); // a caller-supplied loader is never auto-reloaded
  return loader;
}

// openSession creates or resumes the vendor session for this unit. Minting
// happens HERE, never derived from the conversation name (same reasoning as
// every other runtime's write-once handle): `SessionManager.create` is given
// the id outright (S3), so the minted id IS the session's own id.
//
// `modelCtx` arrives as a plain parameter rather than this file reaching for
// its own module-level cache — the same reasoning copilot's `attempt()`
// takes the SDK client as a parameter: a function depending on its caller's
// explicit input, not on hidden module state, is what makes it exercisable
// directly in a test.
//
// `extraToolNames` folds into the explicit `tools:` activation list beside
// `grant.builtinNames` — `createAgentSession`'s own `tools` option, when
// given at all, becomes the session's `initialActiveToolNames` OUTRIGHT
// (confirmed live against the SDK's `sdk.js`), so an MCP tool registered on
// the resource loader's own extension but left out of this list would be
// known to pi's tool registry yet never offered to the model.
async function openSession(sdk, modelCtx, unit, grant, loader, extraToolNames = []) {
  const base = {
    cwd: WORKSPACE,
    agentDir: PI_AGENT_DIR,
    modelRuntime: modelCtx.modelRuntime,
    model: modelCtx.model,
    tools: [...grant.builtinNames, ...extraToolNames],
    resourceLoader: loader,
  };
  const existingId = contextIdOf(unit);
  if (!existingId) {
    const id = crypto.randomUUID();
    const sessionManager = sdk.SessionManager.create(WORKSPACE, path.join(SESSIONS_ROOT, id), { id });
    const { session } = await sdk.createAgentSession({ ...base, sessionManager });
    return { session, id, resumed: false };
  }
  const sessionDir = path.join(SESSIONS_ROOT, existingId);
  const found = sdk.SessionManager.findById(WORKSPACE, existingId, sessionDir);
  if (!found) throw new Error(`no pi session file found for context ${existingId}`);
  const sessionManager = sdk.SessionManager.open(found, sessionDir, WORKSPACE);
  const { session } = await sdk.createAgentSession({ ...base, sessionManager });
  return { session, id: existingId, resumed: true };
}

// closeMcpClients ends every MCP connection this one attempt opened.
// Required rather than left to mcp.js's own `session_shutdown` listener:
// this runtime calls the bare `AgentSession.dispose()` between units
// (never the SDK's own session-replacement path), which never fires that
// event — a stdio server's child process, or an open HTTP MCP connection,
// would otherwise outlive every single run.
async function closeMcpClients(handle) {
  if (!handle || !handle.clients || !handle.clients.length) return;
  await Promise.all(handle.clients.map((c) => c.close().catch(() => {})));
}

async function attempt(sdk, modelCtx, unit, grant, definitionBody, prompt, denials = []) {
  const state = { lastText: '', toolCalls: 0, turns: 0, errors: [], calls: newCallRecorder() };
  let opened;
  const mcpHandle = { toolNames: [], clients: [] };
  try {
    const loader = await buildResourceLoader(sdk, unit, grant, definitionBody, denials, mcpHandle);
    opened = await openSession(sdk, modelCtx, unit, grant, loader, mcpHandle.toolNames);
  } catch (e) {
    // The open itself failed — auth, a resume of a handle that is gone.
    // Logged AND carried as the result, so a fresh session that cannot open
    // reports why instead of an empty failure; the resume path reads
    // `openError` to tell "not found" from everything else.
    const msg = String(e.message || e);
    console.log(`[runtime] could not open the pi session: ${msg}`);
    // buildResourceLoader may have connected MCP servers even though the
    // SESSION never opened — close them here too, or a failed open leaks
    // every one of them.
    await closeMcpClients(mcpHandle);
    // No `exitCode`: the session never opened, so no run ever executed —
    // there is no process outcome to report.
    return { status: 'failed', sessionId: contextIdOf(unit) || null, result: `could not open the pi session: ${msg}`, openError: msg };
  }
  const { session, id } = opened;
  const off = session.subscribe((ev) => onEvent(ev, state));
  logInventoryGaps(session, grant.builtinNames);
  const started = Date.now();
  let status = 'succeeded', result = '', brief = '';
  try {
    await withTimeout(session.prompt(prompt), RUN_TIMEOUT_MS, `run exceeded ${RUN_TIMEOUT_MS / 1000}s`);
    const raw = state.lastText || '';
    // Extracted BEFORE the 2000-char cap below: a brief written late in a
    // long answer must not be sliced away with the rest.
    const extracted = extractBrief(raw);
    result = extracted.text;
    brief = extracted.brief;
    if (state.errors.length && !result) { status = 'failed'; result = state.errors.join('\n'); }
    // NEVER an empty success. A turn that ends without text — a denied tool
    // call ends it, same shape as every other runtime — is reported as a
    // failure that says why.
    if (!result) {
      status = 'failed';
      result = denials.length
        ? `The agent produced no answer: its tool call was denied — ${denials[denials.length - 1]}`
        : 'The agent produced no answer.';
    }
  } catch (e) {
    status = 'failed';
    result = state.lastText || `run: ${String(e.message || e)}`;
  } finally {
    off();
    try { session.dispose(); } catch {}
    // dispose() invalidates the extension context but — confirmed live —
    // never emits 'session_shutdown'; only a session-REPLACEMENT flow does
    // that, and this runtime never takes it. So mcp.js's own fallback
    // listener never fires here, and this is the one place that actually
    // closes a connection this attempt opened — every path above returns
    // before reaching it, so each is responsible for its own call (see the
    // catch above).
    await closeMcpClients(mcpHandle);
  }
  process.stdout.write(`\n=== RESULT (${status}, ${state.turns} turns, ${state.toolCalls} tool calls, ${Math.round((Date.now() - started) / 1000)}s) ===\n${result}\n`);
  return {
    status, exitCode: status === 'succeeded' ? 0 : 1, sessionId: id, result: result.slice(0, 2000),
    ...(brief ? { brief } : {}), ...state.calls.report(),
  };
}

async function runPi(unit) {
  const p = resolvePrompt(unit);
  // No `exitCode`: the prompt could not even be assembled, so nothing ran.
  if (p.error) return { status: 'failed', result: p.error };

  const definition = readAgentDefinition(WORKSPACE, unit.agent, (m) => console.log(sanitizeLog(m)));
  const allowed = composeAllowedTools(definition.tools, unit.allowedTools, unit.toolsMode);
  const grant = translate(allowed);

  const id = contextIdOf(unit);
  console.log(`\n[runtime] run ${sanitizeLog(unit.runId)}${id ? ' continue=' + sanitizeLog(id) : ''} thread=${sanitizeLog(unit.threadId ?? 'general')}`);
  console.log(`[runtime] tools agent=${sanitizeLog(unit.agent || '-')} declared=${definition.tools.length} wiring=${(unit.allowedTools || '').split(',').filter(Boolean).length} mode=${sanitizeLog(unit.toolsMode || 'merge')} -> ${sanitizeLog(allowed.length ? allowed.join(',') : '(none)')}`);
  console.log(`[runtime] pi built-ins=${sanitizeLog(grant.builtinNames.length ? grant.builtinNames.join(',') : '(none)')}`);
  console.log(`[runtime] pi-code ${piCodeEnabled() ? "on: the checkout's .claude/ is read as Claude Code reads it" : "off"}`);
  for (const s of grant.scoped) console.log(`[runtime] scoped pattern withheld: ${sanitizeLog(s)} — pi has no per-command shell narrowing, granting nothing`);
  for (const u of grant.unmapped) console.log(`[runtime] UNMAPPED pattern withheld: ${sanitizeLog(u)} — no pi equivalent, granting nothing`);
  if (grant.mcpPatterns.length) console.log(`[runtime] ${grant.mcpPatterns.length} mcp__ pattern(s) granted to the MCP extension: ${sanitizeLog(grant.mcpPatterns.join(','))}`);
  console.log(`[init] model=${PI_MODEL} provider=${PI_PROVIDER} tools=${grant.builtinNames.length}`);
  if (unit.systemPrompt) console.log(`[runtime] appending unit systemPrompt (${unit.systemPrompt.length} chars)`);
  if (unit.maxTurns) console.log(`[runtime] maxTurns=${sanitizeLog(unit.maxTurns)} requested (not enforced by this runtime; run timeout ${RUN_TIMEOUT_MS / 1000}s)`);

  let s, ctx;
  try {
    s = await getSdk();
    ctx = await getModelContext();
  } catch (e) {
    // No `exitCode`: the client never came up, so no run ever started.
    return { status: 'failed', result: `pi sdk: ${e.message}` };
  }

  const denials = [];
  const first = await attempt(s, ctx, unit, grant, definition.body, p.prompt, denials);
  if (!id) return finish(first, 'new');
  return resolveContinuity(id, first, () => attempt(s, ctx, unit, grant, definition.body, p.prompt, denials));
}

// resolveContinuity decides what a RESUME's outcome reports, given the first
// attempt's result and a way to retry. Pulled out of runPi so the decision is
// testable without the SDK: any open failure re-checks the store (never
// trusting what the SDK's error text happens to say), an unreadable store
// counts as PRESENT (confirmContextMissing's own rule, so it takes the retry
// branch, never the lost one), and a failed retry is reported for what it is
// — never a continuity claim ('continued') it did not earn (task 2.8a).
async function resolveContinuity(id, first, retry, confirmOpts) {
  if (!first.openError) return finish(first, 'continued');

  if (!(await confirmContextMissing(SESSIONS_ROOT, id, confirmOpts))) {
    console.log('[runtime] context present, or the store did not answer, on re-check — retrying');
    const retried = await retry();
    if (retried.openError) {
      const { openError, ...rest } = retried;
      return { ...rest, result: `resume: ${openError}` };
    }
    return finish(retried, 'continued');
  }

  // Genuinely gone. DO NOT answer without it: a conversation without its
  // context is a new one wearing the same name and thread. Same text the
  // other runtimes use, so a person meets one message whatever executes.
  const reason =
    'the stored context for this conversation could not be reached — no session state under ' +
    `${SESSIONS_ROOT} (is /data/context backed by a volume? without one it dies with the pod)`;
  console.log(`[runtime] ${reason} — failing rather than answering without it`);
  return {
    status: 'failed',
    // No `exitCode`: the resume never opened a session, so no run executed.
    sessionId: id,
    continuity: 'unavailable',
    continuityReason: reason,
    result:
      '⚠️ **This conversation cannot be continued.**\n\n' +
      'Its stored context is no longer available, so answering now would mean answering with no ' +
      'memory of what came before — including anything already done. Start a new conversation to ' +
      `continue.\n\nReason: ${reason}`,
  };
}

function finish(out, continuity) {
  const { openError, ...rest } = out;
  return { ...rest, continuity };
}

// ---- the loop ------------------------------------------------------------------

// module.exports precedes the main loop so a test can `require()` this file
// for its pure/subprocess-driving pieces without the IIFE below ever running
// — that only happens when this file is the process entry point.
module.exports = {
  gitEnv, repoURL, run, clearDir, syncRepo, onEvent, resolvePrompt, contextIdOf,
  buildResourceLoader, openSession, attempt, runPi, finish, resolveContinuity,
  extractBrief, logInventoryGaps, getSdk, getModelContext, closeMcpClients,
  SESSIONS_ROOT, WORKSPACE, PI_AGENT_DIR,
};

// Extracted to a NAMED top-level function rather than left as an inline
// IIFE body: SonarCloud's javascript:S3776 (cognitive complexity) charges a
// nested function literal's control flow to the ENCLOSING function, so
// wrapping it in a closure (or `t.Run`-style callback) does not reset the
// score. A plain call is not a nesting-increasing construct, matching the
// same NOSONAR reasoning runtimes/copilot/runtime.js already carries.
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

// 408 (timeout) and 429 (rate limit) are themselves transient, not a verdict
// on the report's content, so they keep the bounded retry below rather than
// being treated as the definitive rejection every OTHER 4xx is (API-10).
const TRANSIENT_4XX = new Set([408, 429]);

// Retries for up to ten minutes (60 * 10s) rather than failing the run: the
// manager's own outage must not discard a result pi already produced. A
// DEFINITIVE 4xx is different — resending the same bytes cannot change the
// manager's verdict, so retrying would only block the poll loop. Every
// undelivered report leaves a log line carrying the status and body (or the
// transport error), so nothing is dropped silently (API-11/API-12).
async function reportDone(unit, out) {
  const { sessionId, ...rest } = out;
  const done = {
    convo: CONVO_ID,
    runId: unit.runId,
    ...rest,
    ...(sessionId ? { runtimeContextId: sessionId } : {}),
  };
  const body = JSON.stringify(done);
  let lastFailure = 'no attempt reached a response';
  for (let i = 0; i < 60; i++) {
    let r;
    try {
      r = await fetch(`${CONTROL_URL}/work/done`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body });
    } catch (e) {
      lastFailure = `transport error: ${e.message}`;
      await sleep(10000);
      continue;
    }
    if (r.ok) return;
    if (r.status >= 400 && r.status < 500 && !TRANSIENT_4XX.has(r.status)) {
      // The body is the control plane's text, not ours: strip control
      // characters so it cannot forge a log line.
      const text = (await r.text().catch(() => '')).replace(/[\x00-\x1f\x7f]+/g, ' ');
      console.error(`[runtime] work/done refused permanently (${r.status}): ${text} — not retrying, resuming polling`);
      return;
    }
    lastFailure = `status ${r.status}`;
    await sleep(10000);
  }
  console.error(`[runtime] work/done still undelivered after the retry budget — ${lastFailure}, resuming polling`);
}

// Exported alongside the module.exports object above rather than folded
// into it: both are declared below that object (function hoisting makes
// either position work), and a second, small assignment here keeps them
// visually beside the loop code they test.
module.exports.pollNextUnit = pollNextUnit;
module.exports.reportDone = reportDone;

// A checkout the run cannot trust must fail the run, with a reason, never
// proceed with whatever the workspace happened to hold from before.
async function runUnit(unit) {
  try {
    await syncRepo();
  } catch (e) {
    console.error(`[runtime] sync: ${e.message}`);
    return { status: 'failed', result: `repository checkout: ${e.message}` };
  }
  return runPi(unit);
}

async function runLoop() {
  console.log(`[runtime] pi runtime — convo=${CONVO_ID} pod=${POD_NAME} ttl=${TTL_MS / 60000}m workspace=${WORKSPACE} state=${SESSIONS_ROOT}`);
  try { await syncRepo(); } catch (e) { console.error(`[runtime] initial sync: ${e.message}`); }

  let lastWork = Date.now();
  let idle = false;
  while (!idle) {
    if (Date.now() - lastWork > TTL_MS) {
      console.log('[runtime] idle TTL reached — exiting');
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
  // PID 1 GETS NO DEFAULT SIGNAL HANDLING. `node` is the container's
  // entrypoint, so a SIGTERM the kubelet sends on pod deletion is IGNORED
  // unless handled — and the pod then sits in Terminating for the whole
  // grace period, holding its conversation's slot and its name. Verified on
  // the other three Node runtimes: 120 seconds, every deletion. Exit
  // promptly; an inflight run is lost either way, and the manager re-runs
  // its input on the next pod.
  for (const sig of ['SIGTERM', 'SIGINT']) {
    process.on(sig, () => {
      console.log(`[runtime] ${sig} — exiting`);
      process.exit(0);
    });
  }

  // javascript:S7785 wants top-level `await` here instead. Real top-level
  // await is an ESM feature and is a SyntaxError in a CommonJS module
  // (declared `"type": "commonjs"` in package.json) -- converting this file
  // and everything it requires to ESM is a far larger change than this line
  // justifies. NOSONAR rather than a real fix, same as every other runtime.
  void runLoop(); // NOSONAR
}
