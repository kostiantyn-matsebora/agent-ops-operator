// cd runtimes/pi && node --test
//
// Live tests against the real `@earendil-works/pi-coding-agent` SDK and a
// local fake OpenAI-compatible provider (test/fake-provider.js) — the same
// shape design.md's spikes used to confirm the SDK surface this runtime
// relies on. No mocking of the SDK itself: `getSdk()`/`getModelContext()`/
// `openSession()`/`attempt()` run for real, against a real pi session, a
// real on-disk sessions root and a real HTTP provider on loopback.
'use strict';

const test = require('node:test');
const assert = require('node:assert');
const fs = require('fs');
const os = require('os');
const path = require('path');

const { startFakeProvider } = require('./test/fake-provider');

function tmp(prefix) {
  return fs.mkdtempSync(path.join(os.tmpdir(), prefix));
}

// Each test gets its OWN HOME/WORKSPACE and re-requires the module fresh —
// runtime.js resolves PI_AGENT_DIR/PI_API_KEY/PI_MODEL etc. into module-level
// consts at require time, exactly as the other env-dependent test files
// here already rely on, so a fresh require per fake-provider port keeps one
// test's provider from leaking into another's model/provider wiring.
function freshRuntime(env) {
  delete require.cache[require.resolve('./runtime')];
  process.env.CONTROL_URL = 'http://127.0.0.1:1';
  process.env.CONVO_ID = 'conv-live';
  process.env.PI_API_KEY = 'test-key';
  process.env.PI_PROVIDER = 'fake';
  process.env.PI_PROVIDER_API = 'openai-completions';
  process.env.PI_BASE_URL = '';
  process.env.REPO_URL = '';
  process.env.RUNTIME_IDLE_TTL_M = '10';
  for (const [k, v] of Object.entries(env)) process.env[k] = v;
  return require('./runtime');
}

async function withProvider(opts, fn) {
  const { port, requests, close } = await startFakeProvider(opts);
  try {
    await fn({ port, requests });
  } finally {
    await close();
  }
}

// ---- getModelContext / getSdk: the file-based models.json path -------------

test('getModelContext resolves a model through a models.json file, never registerProvider', async () => {
  await withProvider({ reply: 'hello from the fake' }, async ({ port }) => {
    const home = tmp('agentops-pi-live-home-');
    const ws = tmp('agentops-pi-live-ws-');
    const rt = freshRuntime({ HOME: home, WORKSPACE: ws, PI_MODEL: 'fake-model', PI_BASE_URL: `http://127.0.0.1:${port}/v1` });
    const ctx = await rt.getModelContext();
    assert.ok(ctx.model, 'getModel must resolve the model this runtime just wrote into models.json');
    assert.ok(fs.existsSync(path.join(rt.PI_AGENT_DIR, 'models.json')), 'the runtime writes its own models.json, never a hand-authored one');
  });
});

// ---- the full run: openSession + attempt, first run and resume -------------

test('a first run creates a session, answers, and mints a runtimeContextId', async () => {
  await withProvider({ reply: 'hello from the fake' }, async ({ port }) => {
    const home = tmp('agentops-pi-live-home-');
    const ws = tmp('agentops-pi-live-ws-');
    fs.mkdirSync(ws, { recursive: true });
    const rt = freshRuntime({ HOME: home, WORKSPACE: ws, PI_MODEL: 'fake-model', PI_BASE_URL: `http://127.0.0.1:${port}/v1` });

    const out = await rt.runPi({ runId: 'r1', promptText: 'say hi', allowedTools: 'Read', toolsMode: 'merge' });
    assert.strictEqual(out.status, 'succeeded');
    assert.strictEqual(out.continuity, 'new');
    assert.ok(out.sessionId, 'a fresh run mints its own runtimeContextId');
    assert.match(out.result, /hello from the fake/);
  });
});

test('a resumed run reuses the same session id end to end', async () => {
  await withProvider({ reply: 'second answer' }, async ({ port }) => {
    const home = tmp('agentops-pi-live-home-');
    const ws = tmp('agentops-pi-live-ws-');
    fs.mkdirSync(ws, { recursive: true });
    const rt = freshRuntime({ HOME: home, WORKSPACE: ws, PI_MODEL: 'fake-model', PI_BASE_URL: `http://127.0.0.1:${port}/v1` });

    const first = await rt.runPi({ runId: 'r1', promptText: 'first', allowedTools: '', toolsMode: 'merge' });
    assert.strictEqual(first.continuity, 'new');

    const second = await rt.runPi({ runId: 'r2', promptText: 'second', runtimeContextId: first.sessionId, allowedTools: '', toolsMode: 'merge' });
    assert.strictEqual(second.continuity, 'continued');
    assert.strictEqual(second.sessionId, first.sessionId);
  });
});

test('resuming a context whose session state is gone fails as unavailable, never answers fresh', async () => {
  await withProvider({ reply: 'should never be read' }, async ({ port }) => {
    const home = tmp('agentops-pi-live-home-');
    const ws = tmp('agentops-pi-live-ws-');
    fs.mkdirSync(ws, { recursive: true });
    const rt = freshRuntime({ HOME: home, WORKSPACE: ws, PI_MODEL: 'fake-model', PI_BASE_URL: `http://127.0.0.1:${port}/v1` });

    const out = await rt.runPi({ runId: 'r1', promptText: 'anything', runtimeContextId: 'never-existed', allowedTools: '', toolsMode: 'merge' });
    assert.strictEqual(out.continuity, 'unavailable');
    assert.match(out.continuityReason, /could not be reached/);
    assert.match(out.result, /cannot be continued/);
  });
});

// ---- task 2.6: the system prompt is the definition's body, then the ----
// ---- unit's own systemPrompt, resupplied on resume too -------------------

test('the composed system prompt carries the agent definition body and the unit systemPrompt, on first run and on resume', async () => {
  await withProvider({ reply: 'ok' }, async ({ port }) => {
    const home = tmp('agentops-pi-live-home-');
    const ws = tmp('agentops-pi-live-ws-');
    fs.mkdirSync(ws, { recursive: true });
    const rt = freshRuntime({ HOME: home, WORKSPACE: ws, PI_MODEL: 'fake-model', PI_BASE_URL: `http://127.0.0.1:${port}/v1` });
    const s = await rt.getSdk();
    const ctx1 = await rt.getModelContext();
    const grant = { builtinNames: [] };

    const unit1 = { runId: 'r1', promptText: 'go', systemPrompt: 'MARKER-ONE', allowedTools: '', toolsMode: 'merge' };
    const loader1 = await rt.buildResourceLoader(s, unit1, grant, 'You are a terse reviewer.', []);
    const opened1 = await rt.openSession(s, ctx1, unit1, grant, loader1);
    assert.match(opened1.session.systemPrompt, /terse reviewer/);
    assert.match(opened1.session.systemPrompt, /MARKER-ONE/);
    // A prompt must actually run once before the session's `.jsonl` lands on
    // disk — `SessionManager.create` mints the handle, but `findById` below
    // has nothing to find until a turn is recorded.
    await opened1.session.prompt('warm up');
    opened1.session.dispose();

    // Resume WITH the override resupplied — this is what runtime.js always
    // does, first run and resume alike, because S2 found neither override
    // persists in the session file on its own.
    const unit2 = { runId: 'r2', promptText: 'go again', runtimeContextId: opened1.id, systemPrompt: 'MARKER-TWO', allowedTools: '', toolsMode: 'merge' };
    const loader2 = await rt.buildResourceLoader(s, unit2, grant, 'You are a terse reviewer.', []);
    const opened2 = await rt.openSession(s, ctx1, unit2, grant, loader2);
    assert.match(opened2.session.systemPrompt, /terse reviewer/);
    assert.match(opened2.session.systemPrompt, /MARKER-TWO/);
    assert.ok(!opened2.session.systemPrompt.includes('MARKER-ONE'), 'only the current call\'s own systemPrompt is present, not a stale one');
    opened2.session.dispose();

    // Resume with NO override resupplied: the marker must NOT survive on
    // its own — confirming S2's live finding directly against this
    // runtime's own loader-building code, not just the spike's scratch
    // script.
    const bareLoader = new s.DefaultResourceLoader({ cwd: ws, agentDir: rt.PI_AGENT_DIR, noContextFiles: true });
    await bareLoader.reload();
    const ctx = await rt.getModelContext();
    const sessionDir = path.join(rt.SESSIONS_ROOT, opened1.id);
    const found = s.SessionManager.findById(ws, opened1.id, sessionDir);
    const bareManager = s.SessionManager.open(found, sessionDir, ws);
    const { session: resumedBare } = await s.createAgentSession({
      cwd: ws, agentDir: rt.PI_AGENT_DIR, modelRuntime: ctx.modelRuntime, model: ctx.model,
      tools: [], resourceLoader: bareLoader, sessionManager: bareManager,
    });
    assert.ok(!resumedBare.systemPrompt.includes('MARKER-ONE'), 'the vendor session must not persist the override on its own');
    assert.ok(!resumedBare.systemPrompt.includes('MARKER-TWO'), 'the vendor session must not persist the override on its own');
    resumedBare.dispose();
  });
});

// ---- task 2.7: every tool_call is answered from the composed allowlist, ---
// ---- deny by default, with the run continuing past a denial --------------
//
// A built-in a session never DECLARES is never offered to the model at all
// (S1) — pi short-circuits an undeclared name as unavailable before the
// `tool_call` hook is ever reached, confirmed live. The hook's own
// deny-by-default behaviour is exercised here the way it is actually
// reachable: the tool IS declared (so the model can call it, and pi routes
// the call through the hook), while the hook's own grant — built
// independently from what was declared, exactly as a future MCP custom tool
// (section 3) would reach it — denies it anyway.

test('a tool call the hook does not grant is denied and logged, and the run continues to answer', async () => {
  await withProvider({ reply: 'fallback answer', toolCall: { name: 'read', args: { path: 'x.txt' } }, toolCallOnce: true }, async ({ port }) => {
    const home = tmp('agentops-pi-live-home-');
    const ws = tmp('agentops-pi-live-ws-');
    fs.mkdirSync(ws, { recursive: true });
    fs.writeFileSync(path.join(ws, 'x.txt'), 'should never be read');
    const rt = freshRuntime({ HOME: home, WORKSPACE: ws, PI_MODEL: 'fake-model', PI_BASE_URL: `http://127.0.0.1:${port}/v1` });

    const s = await rt.getSdk();
    const ctx = await rt.getModelContext();
    const denyingGrant = { builtinNames: [] }; // what the hook decides against
    const unit = { promptText: 'call read on x.txt' };
    const loader = await rt.buildResourceLoader(s, unit, denyingGrant, '', []);
    // `read` is declared to the model (openSession's own `tools:`), but
    // decided against the EMPTY grant the loader's hook was built with.
    const opened = await rt.openSession(s, ctx, unit, { builtinNames: ['read'] }, loader);

    // Drive the real session directly rather than through attempt(), since
    // attempt() always builds its OWN loader from one grant — this test's
    // whole point is a hook whose grant differs from what was declared.
    let lastText = '';
    const off = opened.session.subscribe((ev) => {
      if (ev.type === 'turn_end') {
        const text = (ev.message.content || []).filter((c) => c.type === 'text').map((c) => c.text).join('');
        if (text) lastText = text;
      }
    });
    const logged = [];
    const origLog = console.log;
    console.log = (...args) => { logged.push(args.join(' ')); };
    try {
      await opened.session.prompt(unit.promptText);
    } finally {
      console.log = origLog;
    }
    off();
    assert.ok(logged.some((l) => l.includes('denied tool_call') && l.includes('read')), 'the denial is logged');
    assert.match(lastText, /fallback answer/, 'the run continues to answer past the denial, rather than aborting');
    opened.session.dispose();
  });
});

// ---- task 2.9: pi's own AGENTS.md/CLAUDE.md auto-load is turned off --------

test('an AGENTS.md in the checkout is never composed into the system prompt the session receives', async () => {
  await withProvider({ reply: 'ok' }, async ({ port }) => {
    const home = tmp('agentops-pi-live-home-');
    const ws = tmp('agentops-pi-live-ws-');
    fs.mkdirSync(ws, { recursive: true });
    fs.writeFileSync(path.join(ws, 'AGENTS.md'), '# haiku instructions\nplease answer only in haiku');
    const rt = freshRuntime({ HOME: home, WORKSPACE: ws, PI_MODEL: 'fake-model', PI_BASE_URL: `http://127.0.0.1:${port}/v1` });

    const s = await rt.getSdk();
    const ctx = await rt.getModelContext();
    const grant = { builtinNames: [] };
    const loader = await rt.buildResourceLoader(s, { promptText: 'go' }, grant, '', []);
    const opened = await rt.openSession(s, ctx, { promptText: 'go' }, grant, loader);
    assert.ok(!opened.session.systemPrompt.toLowerCase().includes('haiku'), 'AGENTS.md must never reach the composed system prompt (task 2.9, S6)');
    opened.session.dispose();
  });
});

// ---- task 2.5 (live confirmation): the inventory check against a real session --

test('logInventoryGaps logs nothing for a correctly granted built-in, against a real session', async () => {
  await withProvider({ reply: 'ok' }, async ({ port }) => {
    const home = tmp('agentops-pi-live-home-');
    const ws = tmp('agentops-pi-live-ws-');
    fs.mkdirSync(ws, { recursive: true });
    const rt = freshRuntime({ HOME: home, WORKSPACE: ws, PI_MODEL: 'fake-model', PI_BASE_URL: `http://127.0.0.1:${port}/v1` });

    const s = await rt.getSdk();
    const ctx = await rt.getModelContext();
    const grant = { builtinNames: ['read'] };
    const loader = await rt.buildResourceLoader(s, { promptText: 'go' }, grant, '', []);
    const opened = await rt.openSession(s, ctx, { promptText: 'go' }, grant, loader);
    const logged = [];
    const origLog = console.log;
    console.log = (...args) => logged.push(args.join(' '));
    try {
      rt.logInventoryGaps(opened.session, grant.builtinNames);
    } finally {
      console.log = origLog;
    }
    assert.deepStrictEqual(logged, []);
    opened.session.dispose();
  });
});

test('logInventoryGaps logs a gap when a granted name was never registered by the session', async () => {
  await withProvider({ reply: 'ok' }, async ({ port }) => {
    const home = tmp('agentops-pi-live-home-');
    const ws = tmp('agentops-pi-live-ws-');
    fs.mkdirSync(ws, { recursive: true });
    const rt = freshRuntime({ HOME: home, WORKSPACE: ws, PI_MODEL: 'fake-model', PI_BASE_URL: `http://127.0.0.1:${port}/v1` });

    const s = await rt.getSdk();
    const ctx = await rt.getModelContext();
    const grant = { builtinNames: ['read'] };
    const loader = await rt.buildResourceLoader(s, { promptText: 'go' }, grant, '', []);
    const opened = await rt.openSession(s, ctx, { promptText: 'go' }, grant, loader);
    const logged = [];
    const origLog = console.log;
    console.log = (...args) => logged.push(args.join(' '));
    try {
      rt.logInventoryGaps(opened.session, ['read', 'a-stale-mapped-name']);
    } finally {
      console.log = origLog;
    }
    assert.ok(logged.some((l) => l.includes('a-stale-mapped-name') && l.includes('WARNING')));
    opened.session.dispose();
  });
});

// ---- section 3: the MCP extension, wired through buildResourceLoader -----
// ---- (mcp.test.js drives mcp.js's own connect/register logic directly; ---
// ---- these two prove runtime.js actually wires it onto a live session) ---

test('task 3.4: the session loads exactly the two factories this runtime ships, no other extension', async () => {
  await withProvider({ reply: 'ok' }, async ({ port }) => {
    const home = tmp('agentops-pi-live-home-');
    const ws = tmp('agentops-pi-live-ws-');
    fs.mkdirSync(ws, { recursive: true });
    const rt = freshRuntime({
      HOME: home, WORKSPACE: ws, PI_MODEL: 'fake-model', PI_BASE_URL: `http://127.0.0.1:${port}/v1`,
      MCP_CONFIG: path.join(home, 'nonexistent-mcp.json'),
    });
    const s = await rt.getSdk();
    const grant = { builtinNames: [], mcpPatterns: [] };
    const loader = await rt.buildResourceLoader(s, { promptText: 'go' }, grant, '', []);
    const result = loader.getExtensions();
    assert.deepStrictEqual(result.errors, []);
    assert.strictEqual(
      result.extensions.length, 2,
      'only the tool_call-permission factory and the MCP extension — nothing pi loads on its own'
    );
  });
});

test('task 3.3: an MCP tool the composed allowlist permits is reachable on the live session, named mcp__<server>__<tool>', async () => {
  await withProvider({ reply: 'ok' }, async ({ port }) => {
    const home = tmp('agentops-pi-live-home-');
    const ws = tmp('agentops-pi-live-ws-');
    fs.mkdirSync(ws, { recursive: true });
    const mcpConfig = path.join(home, 'mcp.json');
    fs.writeFileSync(mcpConfig, JSON.stringify({
      mcpServers: { fixture: { command: process.execPath, args: [path.join(__dirname, 'test', 'fake-mcp-server.js')] } },
    }));
    const rt = freshRuntime({ HOME: home, WORKSPACE: ws, PI_MODEL: 'fake-model', PI_BASE_URL: `http://127.0.0.1:${port}/v1`, MCP_CONFIG: mcpConfig });
    const s = await rt.getSdk();
    const ctx = await rt.getModelContext();
    const grant = { builtinNames: [], mcpPatterns: ['mcp__fixture__echo'] };
    const mcpHandle = { toolNames: [], clients: [] };
    const loader = await rt.buildResourceLoader(s, { promptText: 'go' }, grant, '', [], mcpHandle);
    assert.deepStrictEqual(mcpHandle.toolNames, ['mcp__fixture__echo'], 'buildResourceLoader must hand back every tool name the MCP extension registered, before openSession builds its own activation list');
    assert.strictEqual(mcpHandle.clients.length, 1, 'and the connected McpClient, so the caller can close it once this run ends');
    const opened = await rt.openSession(s, ctx, { promptText: 'go' }, grant, loader, mcpHandle.toolNames);
    try {
      assert.ok(
        opened.session.getActiveToolNames().includes('mcp__fixture__echo'),
        'the MCP extension factory must register and activate the permitted tool on this very session'
      );
    } finally {
      opened.session.dispose();
      await rt.closeMcpClients(mcpHandle);
    }
  });
});
