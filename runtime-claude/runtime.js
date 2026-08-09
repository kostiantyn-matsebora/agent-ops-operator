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
const { agentDeclaredTools, composeAllowedTools } = require('./tools');

const CONTROL_URL = process.env.CONTROL_URL || '';
const CONVO_ID = process.env.CONVO_ID || '';
const POD_NAME = process.env.POD_NAME || '';
const REPO_URL = process.env.REPO_URL || '';
const REPO_REF = process.env.REPO_REF || 'master';
const GIT_AUTH_TYPE = process.env.GIT_AUTH_TYPE || '';
const GIT_SSH_KEY = process.env.GIT_SSH_KEY || '';
const GIT_TOKEN = process.env.GIT_TOKEN || '';
const TTL_MS = (parseInt(process.env.RUNTIME_IDLE_TTL_M || '10', 10)) * 60 * 1000;
const WORKSPACE = process.env.WORKSPACE || '/data/workspace';
const MCP_CONFIG = process.env.MCP_CONFIG || '/etc/agentops/mcp.json';

if (!CONTROL_URL || !CONVO_ID) {
  console.error('[runtime] CONTROL_URL and CONVO_ID are required');
  process.exit(1);
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

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
        prompt = fs.readFileSync(path.join(WORKSPACE, unit.promptFile), 'utf8');
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
    const declared = agentDeclaredTools(WORKSPACE, unit.agent, (m) => console.log(m));
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
      ...(unit.resumeSessionId ? ['--resume', unit.resumeSessionId] : []),
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
    console.log(`\n[runtime] run ${unit.runId}${unit.resumeSessionId ? ' resume=' + unit.resumeSessionId : ''} thread=${unit.threadId ?? 'general'}`);
    console.log(`[runtime] tools agent=${unit.agent || '-'} declared=${declared.length} wiring=${(unit.allowedTools || '').split(',').filter(Boolean).length} mode=${unit.toolsMode || 'merge'} -> ${allowed.length ? allowed.join(',') : '(none)'}`);
    if (unit.systemPrompt) console.log(`[runtime] appending system prompt (${unit.systemPrompt.length} chars)`);
    resolve(spawnClaude(args, unit, Boolean(unit.resumeSessionId)));
  });
}

// spawnClaude runs one `claude` invocation.
//
// A RESUME whose session no longer exists is retried ONCE without --resume.
// Sessions live in $HOME/.claude/projects/-data-workspace/, so they vanish
// whenever /data/home does not outlive the pod — an install without a home PVC,
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
      const p = spawn('claude', argv, {
        cwd: WORKSPACE,
        env: { ...process.env, RUN_ID: unit.runId, TG_THREAD_ID: unit.threadId != null ? String(unit.threadId) : '' },
        stdio: ['ignore', 'pipe', 'pipe'],
      });
      let buf = '', sessionId = null, result = '', stderr = '';
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
        }
      });
      p.stderr.on('data', (c) => {
        stderr += c;
        process.stderr.write(c);
      });
      p.on('error', (e) => resolve({ status: 'failed', exitCode: -1, sessionId, result: `spawn: ${e.message}`, stderr }));
      p.on('close', (code) =>
        resolve({ status: code === 0 ? 'succeeded' : 'failed', exitCode: code, sessionId, result, stderr }));
    });

  const first = await attempt(args);
  if (!isResume || first.status === 'succeeded') return strip(first);

  // A session that was found emits session_id early, even on a later failure.
  // No session id AND no result is what a missing session looks like; the
  // stderr text is the direct confirmation when the CLI gives one.
  const lost = !first.sessionId && !first.result;
  const saysSoOnStderr = /no conversation found|session .* not found|could not find session/i.test(first.stderr || '');
  if (!lost && !saysSoOnStderr) return strip(first);

  console.log(
    `[runtime] resume of session ${unit.resumeSessionId} produced no output — the session is gone ` +
      `(is /data/home persisted? without a home PVC it dies with the pod). Retrying as a NEW session; ` +
      `earlier context is lost.`,
  );
  const fresh = args.filter((a, i) => a !== '--resume' && args[i - 1] !== '--resume');
  const second = await attempt(fresh);
  if (second.status === 'succeeded') {
    return strip({
      ...second,
      // Said in the ANSWER, not only the log: the person reading it needs to
      // know the agent has no memory of what came before.
      result: `⚠️ The previous session could not be resumed, so this was answered without its history.\n\n${second.result}`,
    });
  }
  return strip(second);
}

/** strip removes the internal stderr capture from what is reported. */
function strip({ stderr, ...rest }) {
  return rest
}

(async () => {
  console.log(`[runtime] claude runtime — convo=${CONVO_ID} pod=${POD_NAME} ttl=${TTL_MS / 60000}m workspace=${WORKSPACE}`);
  try { await syncRepo(); } catch (e) { console.error(`[runtime] initial sync: ${e.message}`); }

  let lastWork = Date.now();
  for (;;) {
    if (Date.now() - lastWork > TTL_MS) {
      console.log('[runtime] idle TTL reached — exiting');
      process.exit(0);
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
    const done = { convo: CONVO_ID, runId: unit.runId, ...out };
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
})();
