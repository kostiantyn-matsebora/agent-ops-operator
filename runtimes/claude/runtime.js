// agentops claude agent runtime — implements the AgentRuntime /work contract:
//   1. long-poll  GET  $CONTROL_URL/work?convo=$CONVO_ID&pod=$POD_NAME&wait=25
//   2. run claude against the profile's repo checkout, streaming a formatted
//      transcript to STDOUT (pod logs / VictoriaLogs)
//   3. report     POST $CONTROL_URL/work/done
//   4. exit 0 after RUNTIME_IDLE_TTL_M minutes without work
//
// The repository is checked out at /data/workspace — the SAME path the
// pre-operator claude-runner used, so Claude session files (keyed by cwd under
// $HOME/.claude/projects/-data-workspace/) resume seamlessly across systems.

'use strict';

const { spawn } = require('child_process');
const fs = require('fs');
const path = require('path');
const { agentDeclaredTools, composeAllowedTools, safeJoin } = require('./tools');
const { DEFAULT_LIMIT, newSpinWatch, noteToolUse, spinMessage, discardedNotice } = require('./spin');

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
// How many identical unparsable tool calls in a row end a run. 0 disables the
// breaker entirely, which is a decision an install can make and this file will
// not make for it.
const SPIN_LIMIT = (() => {
  const v = Number.parseInt(process.env.RUNTIME_UNPARSED_REPEAT_LIMIT || '', 10);
  return Number.isFinite(v) && v >= 0 ? v : DEFAULT_LIMIT;
})();

if (!CONTROL_URL || !CONVO_ID) {
  console.error('[runtime] CONTROL_URL and CONVO_ID are required');
  process.exit(1);
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

// resolveBin looks `name` up on PATH once and returns the first absolute hit,
// falling back to the bare name so a missing binary still fails with the
// ordinary ENOENT a reader expects. go:S4036's ask, one process over: say
// explicitly where the binary this spawns from comes from, rather than
// leaving PATH to answer it implicitly at the call site.
function resolveBin(name) {
  for (const dir of (process.env.PATH || '').split(path.delimiter)) {
    if (!dir) continue;
    const candidate = path.join(dir, name);
    try {
      fs.accessSync(candidate, fs.constants.X_OK);
      return candidate;
    } catch {
      // not here — keep looking
    }
  }
  return name;
}

const CLAUDE_BIN = resolveBin('claude');

// sanitizeLog strips control characters (CR/LF and other C0) from a value
// before it reaches a log line -- jssecurity:S5145's ask, since a crafted
// runId, agent name or thread id in a work unit could otherwise forge a
// second log line that reads as the runtime's own.
function sanitizeLog(v) {
  return String(v).replace(/[\r\n\x00-\x1f]/g, ' ');
}

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
    await run('git', ['fetch', '--depth', '1', 'origin', REPO_REF], { cwd: WORKSPACE, env: gitEnv() });
    await run('git', ['reset', '--hard', `origin/${REPO_REF}`], { cwd: WORKSPACE, env: gitEnv() });
  }
}

// Turn one stream-json event into a compact human-readable log line.
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

function runClaude(unit) {
  return new Promise((resolve) => {
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
        return resolve({ status: 'failed', exitCode: -1, result: `prompt read: ${e.message}` });
      }
    }
    if (!prompt) return resolve({ status: 'failed', exitCode: -1, result: 'empty prompt' });

    // The work unit carries the WIRING's tools and the mode; the agent's own
    // definition carries the rest, and only this process can read it.
    const declared = agentDeclaredTools(WORKSPACE, unit.agent, (m) => console.log(sanitizeLog(m)));
    const allowed = composeAllowedTools(declared, unit.allowedTools, unit.toolsMode);

    // --allowedTools is passed ALWAYS, even empty: nothing is substituted for
    // an allowlist nobody declared. --permission-mode dontAsk makes an
    // unlisted tool a denial the model is told about, instead of a permission
    // prompt — in a pod there is nobody to answer one, so it would hang until
    // the idle TTL and report nothing.
    //
    // Note the composition happens HERE and not via --agent: passing --agent
    // would re-apply the definition's list as an availability intersection,
    // which silently defeats overwrite and drops any merged tool the agent did
    // not declare. The lane templates name the agent in the prompt instead.
    // Inline role text from a repo-less profile. APPENDED, so the runtime's own
    // system prompt survives — and it says nothing about tools: the allowlist
    // above is the only permission authority.
    const args = [
      ...(contextIdOf(unit) ? ['--resume', contextIdOf(unit)] : []),
      ...(unit.systemPrompt ? ['--append-system-prompt', unit.systemPrompt] : []),
      '-p', prompt,
      '--allowedTools', allowed.join(','),
      '--permission-mode', 'dontAsk',
      '--max-turns', String(unit.maxTurns || 60),
      '--output-format', 'stream-json',
      '--verbose',
      '--strict-mcp-config',
      '--mcp-config', MCP_CONFIG,
    ];
    console.log(`\n[runtime] run ${sanitizeLog(unit.runId)}${contextIdOf(unit) ? ' continue=' + sanitizeLog(contextIdOf(unit)) : ''} thread=${sanitizeLog(unit.threadId ?? 'general')}`);
    console.log(`[runtime] tools agent=${sanitizeLog(unit.agent || '-')} declared=${declared.length} wiring=${(unit.allowedTools || '').split(',').filter(Boolean).length} mode=${sanitizeLog(unit.toolsMode || 'merge')} -> ${allowed.length ? allowed.join(',') : '(none)'}`);
    if (unit.systemPrompt) console.log(`[runtime] appending system prompt (${unit.systemPrompt.length} chars)`);
    resolve(spawnClaude(args, unit, Boolean(contextIdOf(unit))));
  });
}

// spawnClaude runs one `claude` invocation.
//
// A RESUME whose session no longer exists is retried ONCE without --resume.
// Sessions live in $HOME/.claude/projects/-data-workspace/, so they vanish
// whenever /data/context does not outlive the pod — an install without a context PVC,
// an eviction, a node move. Losing the thread's context is a real cost; failing
// the reply outright is a worse one, and it fails with an EMPTY result because
// claude never reaches its result event, so the person who typed sees "failed"
// and no reason at all.
//
// Only a resume is retried, and only once: an ordinary agent failure must not be
// silently run twice, which would double the cost and could repeat a mutation.
async function spawnClaude(args, unit, isResume) {
  const attempt = (argv) =>
    new Promise((resolve) => {
      const p = spawn(CLAUDE_BIN, argv, {
        cwd: WORKSPACE,
        env: { ...process.env, RUN_ID: unit.runId, TG_THREAD_ID: unit.threadId != null ? String(unit.threadId) : '' },
        stdio: ['ignore', 'pipe', 'pipe'],
      });
      let buf = '', sessionId = null, result = '', stderr = '';
      // Tool calls the model could not FORM. Nothing executes them, so a run
      // that only makes those looks busy and answers from whatever it already
      // had — see spin.js.
      const watch = newSpinWatch(SPIN_LIMIT);
      let spin = null, kill = null;
      p.stdout.on('data', (c) => {
        buf += c;
        let nl;
        while ((nl = buf.indexOf('\n')) >= 0) {
          const line = buf.slice(0, nl);
          buf = buf.slice(nl + 1);
          if (!line.trim()) continue;
          let ev;
          try { ev = JSON.parse(line); } catch { console.log(line); continue; }
          if (ev.session_id && !sessionId) sessionId = ev.session_id;
          if (ev.type === 'result') result = (ev.result || '').slice(0, 2000);
          const txt = formatEvent(ev, line);
          if (txt) process.stdout.write(txt);
          if (ev.type === 'assistant' && !spin) {
            for (const b of ev.message?.content || []) {
              if (b.type !== 'tool_use') continue;
              const verdict = noteToolUse(watch, b.name, b.input);
              if (!verdict) continue;
              spin = verdict;
              console.log(
                `[runtime] ${verdict.name} called ${verdict.repeats}x with the same unparsable arguments — ` +
                  `nothing ran, ending the run: ${verdict.raw.slice(0, 160)}`,
              );
              // TERM first, KILL only if it does not go: an outright kill is
              // a last resort, and a CLI that ignores TERM must still not be
              // able to hold the pod.
              p.kill('SIGTERM');
              kill = setTimeout(() => p.kill('SIGKILL'), 5000);
              break;
            }
          }
        }
      });
      p.stderr.on('data', (c) => {
        stderr += c;
        process.stderr.write(c);
      });
      p.on('error', (e) => resolve({ status: 'failed', exitCode: -1, sessionId, result: `spawn: ${e.message}`, stderr }));
      p.on('close', (code) => {
        if (kill) clearTimeout(kill);
        if (spin) {
          // FAILED, and said plainly. The alternative is what happened before
          // this existed: a run reported success while every tool call in it
          // had been discarded unread.
          return resolve({ status: 'failed', exitCode: code ?? -1, sessionId, result: spinMessage(spin), stderr });
        }
        if (watch.total > 0) {
          // Recovered on its own, which is the common case — by ABANDONING the
          // tool and answering from what the session already held, twice out of
          // twice observed. The log line is for whoever operates this; the
          // notice on the answer is for whoever asked, because the answer does
          // not say it.
          console.log(`[runtime] ${watch.total} tool call(s) never ran — the arguments were not valid JSON`);
          const notice = discardedNotice(watch);
          if (notice && result) result = `${result}\n\n${notice}`;
        }
        resolve({ status: code === 0 ? 'succeeded' : 'failed', exitCode: code, sessionId, result, stderr });
      });
    });

  const first = await attempt(args);
  if (!isResume) return strip({ ...first, continuity: 'new' });
  if (first.status === 'succeeded') return strip({ ...first, continuity: 'continued' });

  // A session that was found emits session_id early, even on a later failure.
  // No session id AND no result is what a missing session looks like; the
  // stderr text is the direct confirmation when the CLI gives one.
  const lost = !first.sessionId && !first.result;
  const saysSoOnStderr = /no conversation found|session .* not found|could not find session/i.test(first.stderr || '');
  if (!lost && !saysSoOnStderr) return strip({ ...first, continuity: 'continued' });

  // GONE, or merely NOT ANSWERING? The two look identical from one attempt and
  // must not be treated alike: a shared filesystem can fail to answer for
  // seconds — a restarting share-manager, a stale handle after this pod moved,
  // a listing that has not yet seen a file another node wrote — and ending a
  // conversation over a lag of that kind would turn a storage nicety into a
  // correctness bug. Only an answer of "not there" is unavailability.
  const contextId = contextIdOf(unit);
  if (!(await confirmContextMissing(contextId))) {
    console.log('[runtime] context reappeared on re-check — the store was slow, not empty');
    const retried = await attempt(args);
    return strip({ ...retried, continuity: 'continued' });
  }

  // Genuinely gone. DO NOT answer without it: a conversation without its context
  // is not that conversation, it is a new one wearing the same name and thread.
  // Answering anyway presents the second as the first to the person replying,
  // and an agent asked to undo something it has no memory of will guess. Fail,
  // say why, and spend no second invocation on an answer that should not exist.
  const reason =
    'the stored context for this conversation could not be reached — no session files under ' +
    `${SESSIONS_DIR} (is /data/context backed by a volume? without one it dies with the pod)`;
  console.log(`[runtime] ${reason} — failing rather than answering without it`);
  return strip({
    status: 'failed',
    exitCode: first.exitCode ?? 1,
    // The handle is still surrendered when the CLI produced one: continuing a
    // partial context beats starting over.
    sessionId: first.sessionId,
    continuity: 'unavailable',
    continuityReason: reason,
    // NEVER an empty result. Failing with nothing to read is precisely why this
    // path used to answer without context instead, and reintroducing it as the
    // fix would be the same mistake wearing the opposite decision.
    result:
      '⚠️ **This conversation cannot be continued.**\n\n' +
      'Its stored context is no longer available, so answering now would mean answering with no ' +
      'memory of what came before — including anything already done. Start a new conversation to ' +
      `continue.\n\nReason: ${reason}`,
    stderr: first.stderr,
  });
}

/**
 * confirmContextMissing re-checks after short delays, distinguishing a store
 * that says the context is GONE from one that merely did not ANSWER.
 *
 * Bounded and short on purpose: a person is waiting on a chat reply, so the
 * budget is seconds. Long enough for a share-manager restart or an attribute
 * cache to settle; nowhere near long enough to read as a hung reply.
 */
async function confirmContextMissing(contextId) {
  if (!contextId) return true;
  for (const waitMs of [500, 1500, 3000]) {
    await new Promise((r) => setTimeout(r, waitMs));
    let found;
    try {
      found = await sessionFileExists(SESSIONS_DIR, contextId);
    } catch {
      // Unreadable is NOT absent: a stale mount throws here, and treating that
      // as "gone" is exactly the conflation this exists to prevent.
      return false;
    }
    if (found) return false;
  }
  return true;
}

/** sessionFileExists looks for a context's transcript anywhere under dir. */
async function sessionFileExists(dir, contextId) {
  const entries = await fs.promises.readdir(dir, { withFileTypes: true }).catch(() => null);
  if (entries === null) return false;
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

/** strip removes the internal stderr capture from what is reported. */
function strip({ stderr, ...rest }) {
  return rest
}

(async () => {
  console.log(`[runtime] claude runtime — convo=${CONVO_ID} pod=${POD_NAME} ttl=${TTL_MS / 60000}m workspace=${WORKSPACE}`);
  try { await syncRepo(); } catch (e) { console.error(`[runtime] initial sync: ${e.message}`); }

  let lastWork = Date.now();
  let idle = false;
  while (!idle) {
    if (Date.now() - lastWork > TTL_MS) {
      console.log('[runtime] idle TTL reached — exiting');
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
    const out = await runClaude(unit);
    lastWork = Date.now();
    // Report the handle under BOTH names for one release: the current one, and
    // the retired spelling so this image also works against an older manager.
    // `continuity` rides along from spawnClaude — the manager cannot infer it,
    // since it sends a handle and gets a handle back.
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
