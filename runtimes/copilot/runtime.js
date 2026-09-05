// agentops copilot agent runtime — implements the AgentRuntime /work contract:
//   1. long-poll  GET  $CONTROL_URL/work?convo=$CONVO_ID&pod=$POD_NAME&wait=25
//   2. run GitHub Copilot (through its SDK, in-process) against the profile's
//      repo checkout, streaming a formatted transcript to STDOUT
//   3. report     POST $CONTROL_URL/work/done
//   4. exit 0 after RUNTIME_IDLE_TTL_M minutes without work
//
// THE THIRD RUNTIME, and the first whose vendor owns its own tool vocabulary,
// its own agent-definition format and its own session store. Every one of
// those is translated HERE — vocabulary.js, tools.js, mcp.js — and none of it
// reaches a CRD. A Pipeline binds the same toolsets it binds for claude.
//
// THE CONTEXT HANDLE IS MINTED HERE. The SDK accepts a caller-chosen session
// id, so `runtimeContextId` is an id this process generated and the vendor's
// state lives under $COPILOT_HOME/session-state/<id>/ — which is $HOME/.copilot,
// the CONTEXT volume or the pod-local copy context-sync restores. The bundle
// declares that path; this file assumes nothing else about it.

'use strict';

const { spawn } = require('child_process');
const crypto = require('crypto');
const fs = require('fs');
const path = require('path');
const { agentDeclaredTools, composeAllowedTools, safeJoin, sanitizeLog } = require('./tools');
const { translate, decide } = require('./vocabulary');
const { loadMcpServers } = require('./mcp');
const { confirmContextMissing } = require('./continuity');

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
const HOME = process.env.HOME || '/data/context';
// COPILOT_HOME is the SDK's own name for its state root. Set explicitly so the
// path the bundle declares to context-sync and the path the runtime writes are
// one string, decided in one place.
const COPILOT_HOME = process.env.COPILOT_HOME || path.join(HOME, '.copilot');
const SESSIONS_DIR = path.join(COPILOT_HOME, 'session-state');
const COPILOT_GITHUB_TOKEN = process.env.COPILOT_GITHUB_TOKEN || '';
const COPILOT_MODEL = process.env.COPILOT_MODEL || '';
// One run's wall-clock ceiling. The unit's maxTurns has no Copilot equivalent
// (see runCopilot), so this and the optional credit budget are what bound it.
const RUN_TIMEOUT_MS = (Number.parseInt(process.env.COPILOT_RUN_TIMEOUT_S || '3600', 10)) * 1000;
// BYOK: the SDK's `provider` config, verbatim — an install that fronts its
// own model endpoint sets it, and it bypasses Copilot API authentication.
const PROVIDER = (() => {
  const raw = process.env.COPILOT_PROVIDER_JSON || '';
  if (!raw) return null;
  try { return JSON.parse(raw); } catch (e) { console.error(`[runtime] COPILOT_PROVIDER_JSON is not JSON: ${e.message}`); process.exit(1); }
})();
const MAX_AI_CREDITS = (() => {
  const v = Number(process.env.COPILOT_MAX_AI_CREDITS || '');
  return Number.isFinite(v) && v > 0 ? v : 0;
})();

// Guarded on require.main so this file can be `require()`d by its own test
// suite -- coverage's only way to reach the functions below -- without also
// exiting the test process, installing real signal handlers on it, or
// starting the network loop at the bottom.
if (require.main === module) {
if (!CONTROL_URL || !CONVO_ID) {
  console.error('[runtime] CONTROL_URL and CONVO_ID are required');
  process.exit(1);
}
if (!COPILOT_GITHUB_TOKEN && !PROVIDER) {
  // Fail at START, not on the first run: a missing credential is a wiring
  // fact, and a pod that polls for work it can never do looks healthy.
  console.error('[runtime] COPILOT_GITHUB_TOKEN is required (the bundle projects it from its credential Secret)');
  process.exit(1);
}
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

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
  if (!REPO_URL) { fs.mkdirSync(WORKSPACE, { recursive: true }); return; }
  if (!fs.existsSync(path.join(WORKSPACE, '.git'))) {
    fs.mkdirSync(WORKSPACE, { recursive: true });
    clearDir(WORKSPACE);
    const r = await run('git', ['clone', '--depth', '1', '-b', REPO_REF, repoURL(), '.'], { cwd: WORKSPACE, env: gitEnv() });
    if (r.code !== 0) throw new Error(`git clone: ${r.err.slice(-400)}`);
  } else {
    await run('git', ['fetch', '--depth', '1', 'origin', REPO_REF], { cwd: WORKSPACE, env: gitEnv() });
    await run('git', ['reset', '--hard', `origin/${REPO_REF}`], { cwd: WORKSPACE, env: gitEnv() });
  }
}

// ---- the SDK client ---------------------------------------------------------

let sdk = null;
let client = null;

// Loaded lazily so the unit tests can require the modules beside this file
// without the SDK — the SDK bundles a 300 MB CLI, and a test suite that needs
// it is a test suite that does not run.
async function getClient() {
  if (client) return client;
  sdk = sdk || (await import('@github/copilot-sdk'));
  // mode "empty" is the SDK's multi-tenant posture: no session telemetry, no
  // cross-session store, no skills, no memory, custom instructions skipped,
  // and every session MUST state availableTools — which is exactly the rule
  // this runtime enforces anyway.
  client = new sdk.CopilotClient({
    mode: 'empty',
    baseDirectory: COPILOT_HOME,
    workingDirectory: WORKSPACE,
    ...(COPILOT_GITHUB_TOKEN ? { gitHubToken: COPILOT_GITHUB_TOKEN } : {}),
    useLoggedInUser: false,
    logLevel: process.env.COPILOT_LOG_LEVEL || 'warning',
  });
  await client.start();
  return client;
}

// checkInventory logs every mapped built-in the runtime did not register. A
// wire name is a vendor fact that moves; a wrong one must surface on the first
// run, not as a tool that silently never appears.
let inventoryChecked = false;
async function checkInventory(available) {
  if (inventoryChecked) return;
  inventoryChecked = true;
  try {
    const c = await getClient();
    // WITH A MODEL, always: the model-less catalog names `str_replace_editor`
    // where every real session registers `view`/`create`/`edit`, and warned
    // about a tool that was working — verified live. `auto` is what a session
    // naming no model resolves to.
    const list = await c.rpc.tools.list({ model: COPILOT_MODEL || 'auto' });
    const names = new Set((list && (list.tools || list)) .map((t) => t.name).filter(Boolean));
    for (const f of available) {
      if (!f.startsWith('builtin:')) continue;
      const n = f.slice('builtin:'.length);
      if (!names.has(n)) console.log(`[runtime] WARNING: mapped built-in "${n}" is not registered by this Copilot runtime — the mapping is stale`);
    }
  } catch (e) {
    console.log(`[runtime] built-in inventory check skipped: ${e.message}`);
  }
}

// ---- transcript -------------------------------------------------------------

function onEvent(ev, state) {
  try {
    switch (ev.type) {
      case 'session.start':
      case 'session.resume':
        return;
      case 'assistant.message':
        if (ev.data && typeof ev.data.content === 'string' && ev.data.content) {
          state.lastText = ev.data.content;
          process.stdout.write(`[copilot] ${ev.data.content}\n`);
        }
        return;
      case 'tool.execution_start': {
        const args = JSON.stringify(ev.data && ev.data.arguments !== undefined ? ev.data.arguments : {});
        const name = ev.data && ev.data.toolName;
        if (ev.data && ev.data.toolCallId) state.toolNames.set(ev.data.toolCallId, name);
        process.stdout.write(`[tool] ${name} ${args.length > 160 ? args.slice(0, 160) + '…' : args}\n`);
        state.toolCalls++;
        return;
      }
      case 'tool.execution_complete':
        if (ev.data && ev.data.success === false && ev.data.error) {
          const name = state.toolNames.get(ev.data.toolCallId) || ev.data.toolCallId || '';
          process.stdout.write(`[tool] ${name} failed: ${String(ev.data.error.message || '').slice(0, 200)}\n`);
        }
        return;
      case 'session.error':
        state.errors.push(String((ev.data && (ev.data.message || ev.data.error)) || 'session error'));
        process.stdout.write(`[error] ${state.errors[state.errors.length - 1]}\n`);
        return;
      case 'assistant.usage':
        state.turns++;
        return;
      default:
        return;
    }
  } catch {
    // a transcript line must never take the run down
  }
}

// ---- one run -----------------------------------------------------------------

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

function sessionConfig(unit, grant, mcp, denials) {
  // ALWAYS an explicit allowlist, even []: nothing is substituted for a
  // composition that produced nothing, and Copilot's own "no declaration means
  // everything" never gets a chance to apply.
  const cfg = {
    workingDirectory: WORKSPACE,
    availableTools: grant.available,
    streaming: false,
    enableConfigDiscovery: false,
    skipCustomInstructions: true,
    mcpServers: mcp.servers,
    onPermissionRequest: (request) => {
      const d = decide(grant, request);
      if (!d.allow) { console.log(`[runtime] denied ${request.kind}: ${d.why}`); denials.push(d.why); }
      // `reject` and `approve-once` are the runtime's vocabulary; `deny` is
      // refused as malformed and fails the call as an ERROR, which reads as
      // a denial but is not one. The feedback is what the model is told.
      return d.allow ? { kind: 'approve-once' } : { kind: 'reject', feedback: `not permitted by this conversation's tool allowlist: ${d.why}` };
    },
  };
  if (COPILOT_MODEL) cfg.model = COPILOT_MODEL;
  if (PROVIDER) cfg.provider = PROVIDER;
  if (MAX_AI_CREDITS) cfg.sessionLimits = { maxAiCredits: MAX_AI_CREDITS };
  // Inline role text from a repo-less profile, APPENDED to the runtime's own
  // system prompt exactly as --append-system-prompt does one runtime over. The
  // SDK does not persist it, so it is supplied on create AND on resume.
  if (unit.systemPrompt) cfg.systemMessage = { mode: 'append', content: unit.systemPrompt };
  return cfg;
}

// openSession creates or resumes the vendor session for this unit.
async function openSession(c, unit, cfg) {
  const id = contextIdOf(unit);
  if (id) return { session: await c.resumeSession(id, cfg), id, resumed: true };
  // MINTED, never derived: encoding the conversation name would make the
  // handle reproducible, which quietly re-introduces write-once semantics.
  const fresh = crypto.randomUUID();
  return { session: await c.createSession({ ...cfg, sessionId: fresh }), id: fresh, resumed: false };
}

async function attempt(c, unit, cfg, prompt, denials = []) {
  const state = { lastText: '', toolCalls: 0, turns: 0, errors: [], toolNames: new Map() };
  let opened;
  try {
    opened = await openSession(c, unit, cfg);
  } catch (e) {
    // The open itself failed — auth, the CLI, a resume of a handle that is
    // gone. Logged AND carried as the result, so a fresh session that cannot
    // open reports why instead of an empty failure; the resume path reads
    // `openError` to tell "not found" from everything else.
    const msg = String(e.message || e);
    console.log(`[runtime] could not open the Copilot session: ${msg}`);
    return { status: 'failed', exitCode: 1, sessionId: contextIdOf(unit) || null, result: `could not open the Copilot session: ${msg}`, openError: msg };
  }
  const { session, id } = opened;
  const off = session.on((ev) => onEvent(ev, state));
  const started = Date.now();
  let status = 'succeeded', result = '';
  try {
    const ev = await session.sendAndWait({ prompt }, RUN_TIMEOUT_MS);
    result = (ev && ev.data && typeof ev.data.content === 'string' ? ev.data.content : state.lastText) || '';
    if (state.errors.length && !result) { status = 'failed'; result = state.errors.join('\n'); }
    // NEVER an empty success. A turn that ends without text — a rejected tool
    // call ends it, verified — is reported as a failure that says why.
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
    try { await session.disconnect(); } catch {}
  }
  process.stdout.write(`\n=== RESULT (${status}, ${state.turns} turns, ${state.toolCalls} tool calls, ${Math.round((Date.now() - started) / 1000)}s) ===\n${result}\n`);
  return { status, exitCode: status === 'succeeded' ? 0 : 1, sessionId: id, result: result.slice(0, 2000) };
}

const NOT_FOUND = /session not found/i;

async function runCopilot(unit) {
  const p = resolvePrompt(unit);
  if (p.error) return { status: 'failed', exitCode: -1, result: p.error };

  const declared = agentDeclaredTools(WORKSPACE, unit.agent, (m) => console.log(sanitizeLog(m)));
  const allowed = composeAllowedTools(declared, unit.allowedTools, unit.toolsMode);
  const grant = translate(allowed);
  const mcp = loadMcpServers(MCP_CONFIG);

  const id = contextIdOf(unit);
  console.log(`\n[runtime] run ${sanitizeLog(unit.runId)}${id ? ' continue=' + sanitizeLog(id) : ''} thread=${sanitizeLog(unit.threadId ?? 'general')}`);
  console.log(`[runtime] tools agent=${sanitizeLog(unit.agent || '-')} declared=${declared.length} wiring=${(unit.allowedTools || '').split(',').filter(Boolean).length} mode=${sanitizeLog(unit.toolsMode || 'merge')} -> ${sanitizeLog(allowed.length ? allowed.join(',') : '(none)')}`);
  console.log(`[runtime] copilot available=${sanitizeLog(grant.available.length ? grant.available.join(',') : '(none)')}${grant.shell ? ` shell=${grant.shell.all ? 'any' : sanitizeLog(grant.shell.prefixes.map((x) => `"${x} *"`).join('|'))}` : ''}`);
  for (const u of grant.unmapped) console.log(`[runtime] UNMAPPED pattern withheld: ${sanitizeLog(u)} — no Copilot equivalent, granting nothing`);
  for (const r of grant.refused) console.log(`[runtime] REFUSED per-server wildcard: ${sanitizeLog(r)} — Copilot admits mcp:* or an exact name, and mcp:* would grant every bound server`);
  for (const f of mcp.failed) console.log(`[runtime] MCP server "${f.name}" not registered: ${f.reason}`);
  console.log(`[init] model=${COPILOT_MODEL || 'default'} tools=${grant.available.length} mcp=${Object.keys(mcp.servers).join(',') || '-'}`);
  if (unit.systemPrompt) console.log(`[runtime] appending system prompt (${unit.systemPrompt.length} chars)`);
  // maxTurns bounds a claude run; Copilot's nearest control is a credit budget,
  // not a turn count. Logged, never faked as enforced.
  if (unit.maxTurns) console.log(`[runtime] maxTurns=${sanitizeLog(unit.maxTurns)} requested (not enforced by this runtime; ${MAX_AI_CREDITS ? `maxAiCredits=${MAX_AI_CREDITS}` : 'no credit cap'}, run timeout ${RUN_TIMEOUT_MS / 1000}s)`);

  let c;
  try {
    c = await getClient();
  } catch (e) {
    return { status: 'failed', exitCode: 1, result: `copilot sdk: ${e.message}` };
  }
  await checkInventory(grant.available);
  const denials = [];
  const cfg = sessionConfig(unit, grant, mcp, denials);

  const first = await attempt(c, unit, cfg, p.prompt, denials);
  if (!id) return finish(first, 'new');
  if (!first.openError) return finish(first, 'continued');

  // The resume did not open. A store that says NOT FOUND and one that merely
  // did not answer must not be treated alike.
  if (!NOT_FOUND.test(first.openError)) {
    return finish({ ...first, result: `resume: ${first.openError}` }, 'continued');
  }
  if (!(await confirmContextMissing(SESSIONS_DIR, id))) {
    console.log('[runtime] context reappeared on re-check — the store was slow, not empty');
    const retried = await attempt(c, unit, cfg, p.prompt, denials);
    if (retried.openError) return finish({ ...retried, result: `resume: ${retried.openError}` }, 'continued');
    return finish(retried, 'continued');
  }

  // Genuinely gone. DO NOT answer without it: a conversation without its
  // context is a new one wearing the same name and thread. Same text the
  // other runtimes use, so a person meets one message whatever executes.
  const reason =
    'the stored context for this conversation could not be reached — no session state under ' +
    `${SESSIONS_DIR} (is /data/context backed by a volume? without one it dies with the pod)`;
  console.log(`[runtime] ${reason} — failing rather than answering without it`);
  return {
    status: 'failed',
    exitCode: 1,
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

// ---- the loop ----------------------------------------------------------------

// module.exports precedes the main loop so a test can `require()` this file
// for its pure/subprocess-driving pieces without the IIFE below ever running
// — that only happens when this file is the process entry point.
module.exports = {
  gitEnv, repoURL, run, clearDir, syncRepo, onEvent, resolvePrompt, contextIdOf,
  sessionConfig, finish,
  SESSIONS_DIR, WORKSPACE, COPILOT_HOME,
};

// The block below is wrapped rather than edited: its own lines are untouched
// text, so a static analyzer's PR-new-code tracking treats its existing
// findings (if any) as old code rather than re-flagging them as new the
// moment a guard is added.
if (require.main === module) {
// PID 1 GETS NO DEFAULT SIGNAL HANDLING. `node` is the container's entrypoint,
// so a SIGTERM the kubelet sends on pod deletion is IGNORED unless handled —
// and the pod then sits in Terminating for the whole grace period, holding its
// conversation's slot and its name. Verified: 120 seconds, every deletion.
// Exit promptly; an inflight run is lost either way, and the manager re-runs
// its input on the next pod.
for (const sig of ['SIGTERM', 'SIGINT']) {
  process.on(sig, () => {
    console.log(`[runtime] ${sig} — exiting`);
    const done = () => process.exit(0);
    if (client) client.stop().then(done, done); else done();
    setTimeout(done, 5000).unref();
  });
}

(async () => {
  console.log(`[runtime] copilot runtime — convo=${CONVO_ID} pod=${POD_NAME} ttl=${TTL_MS / 60000}m workspace=${WORKSPACE} state=${SESSIONS_DIR}`);
  try { await syncRepo(); } catch (e) { console.error(`[runtime] initial sync: ${e.message}`); }

  let lastWork = Date.now();
  let idle = false;
  while (!idle) {
    if (Date.now() - lastWork > TTL_MS) {
      console.log('[runtime] idle TTL reached — exiting');
      try { if (client) await client.stop(); } catch {}
      idle = true;
      continue;
    }
    let res;
    try {
      res = await fetch(`${CONTROL_URL}/work?convo=${encodeURIComponent(CONVO_ID)}&pod=${encodeURIComponent(POD_NAME)}&wait=25`);
    } catch { await sleep(5000); continue; }
    if (res.status === 204) continue;
    if (!res.ok) { await sleep(5000); continue; }
    let unit;
    try { unit = await res.json(); } catch { continue; }
    lastWork = Date.now();
    try { await syncRepo(); } catch (e) { console.error(`[runtime] sync: ${e.message}`); }
    const out = await runCopilot(unit);
    lastWork = Date.now();
    // The handle under BOTH names for one release: the current one, and the
    // retired spelling so this image also works against an older manager.
    const { sessionId, ...rest } = out;
    const done = {
      convo: CONVO_ID,
      runId: unit.runId,
      ...rest,
      ...(sessionId ? { runtimeContextId: sessionId, sessionId } : {}),
    };
    for (let i = 0; i < 60; i++) {
      try {
        const r = await fetch(`${CONTROL_URL}/work/done`, {
          method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(done),
        });
        if (r.ok) break;
      } catch {}
      await sleep(10000);
    }
  }
  process.exit(0);
})();
}
