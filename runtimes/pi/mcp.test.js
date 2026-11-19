// cd runtimes/pi && node --test
//
// Task 3.1/3.2: translation tests ported field for field from
// runtimes/copilot/mcp.test.js (same `expand`/`translateServers`/
// `loadMcpServers` shape). Task 3.3/3.4: this runtime's own connect-and-
// register logic, driven for real against test/fake-mcp-server.js over a
// real stdio StdioTransport/McpClient (@earendil-works/pi-mcp) — no mocking
// of the MCP client itself, same "drive the real thing" style
// runtime.live.test.js already uses for the pi SDK.
'use strict';

const test = require('node:test');
const assert = require('node:assert');
const fs = require('fs');
const os = require('os');
const path = require('path');

const { expand, translateServers, loadMcpServers, allowlistFrom, registerMcpTools } = require('./mcp');

const FAKE_SERVER = path.join(__dirname, 'test', 'fake-mcp-server.js');

function tmpFile(name = 'mcp.json') {
  return path.join(fs.mkdtempSync(path.join(os.tmpdir(), 'agentops-pi-mcp-')), name);
}

function stdioServer(extra = {}) {
  return { command: process.execPath, args: [FAKE_SERVER], ...extra };
}

// A fake pi ExtensionAPI: collects every registerTool() call and the
// session_shutdown handler registerMcpTools installs to close its clients.
function fakePi() {
  const tools = [];
  const handlers = {};
  return {
    tools,
    registerTool: (def) => tools.push(def),
    on: (event, handler) => { handlers[event] = handler; },
    async shutdown() { if (handlers.session_shutdown) await handlers.session_shutdown(); },
  };
}

// ---- task 3.1: translation, ported field for field from copilot's mcp.js --

test('a stdio server carries command, args and env', () => {
  const { servers, failed } = translateServers({ mcpServers: { k8s: { command: 'mcp-k8s', args: ['--ro'], env: { A: '1' } } } }, {});
  assert.deepStrictEqual(failed, []);
  assert.deepStrictEqual(servers.k8s, { type: 'stdio', tools: ['*'], command: 'mcp-k8s', args: ['--ro'], env: { A: '1' } });
});

test('an http server carries url and headers', () => {
  const { servers, failed } = translateServers({ mcpServers: { prom: { type: 'http', url: 'http://prom:8080/mcp', headers: { 'X-A': 'b' } } } }, {});
  assert.deepStrictEqual(failed, []);
  assert.deepStrictEqual(servers.prom, { type: 'http', url: 'http://prom:8080/mcp', tools: ['*'], headers: { 'X-A': 'b' } });
});

test('a url without a type is http', () => {
  const { servers } = translateServers({ mcpServers: { s: { url: 'http://x/mcp' } } }, {});
  assert.strictEqual(servers.s.type, 'http');
});

test('an "sse" type server is translated the same as http — internal/mcpcompile\'s own name for it', () => {
  const { servers, failed } = translateServers({ mcpServers: { vl: { type: 'sse', url: 'http://vl' } } }, {});
  assert.deepStrictEqual(failed, []);
  assert.strictEqual(servers.vl.type, 'sse');
  assert.strictEqual(servers.vl.url, 'http://vl');
});

test('${VAR} is expanded from the environment', () => {
  const env = { TOKEN: 'secret-value' };
  assert.deepStrictEqual(expand('Bearer ${TOKEN}', env), { value: 'Bearer secret-value' });
  const { servers } = translateServers({ mcpServers: {
    h: { type: 'http', url: 'http://x', headers: { Authorization: 'Bearer ${TOKEN}' } },
    s: { command: 'c', env: { KEY: '${TOKEN}' } },
  } }, env);
  assert.strictEqual(servers.h.headers.Authorization, 'Bearer secret-value');
  assert.strictEqual(servers.s.env.KEY, 'secret-value');
});

test('an unresolvable placeholder fails THAT server only, naming the variable and never its value', () => {
  const { servers, failed } = translateServers({ mcpServers: {
    ok: { command: 'c' },
    bad: { type: 'http', url: 'http://x', headers: { Authorization: 'Bearer ${MISSING}' } },
  } }, { OTHER: 'v' });
  assert.deepStrictEqual(Object.keys(servers), ['ok']);
  assert.strictEqual(failed.length, 1);
  assert.strictEqual(failed[0].name, 'bad');
  assert.match(failed[0].reason, /MISSING/);
  assert.doesNotMatch(failed[0].reason, /Bearer/);
});

test('an unresolvable placeholder in a STDIO server env fails that server, naming the variable', () => {
  const { servers, failed } = translateServers({ mcpServers: {
    ok: { command: 'c' },
    bad: { command: 'c', env: { KEY: '${MISSING}' } },
  } }, {});
  assert.deepStrictEqual(Object.keys(servers), ['ok']);
  assert.strictEqual(failed.length, 1);
  assert.strictEqual(failed[0].name, 'bad');
  assert.match(failed[0].reason, /env/);
  assert.match(failed[0].reason, /MISSING/);
});

test('a malformed server is reported, not registered', () => {
  const { servers, failed } = translateServers({ mcpServers: { a: { type: 'http' }, b: {}, c: 'nope' } }, {});
  assert.deepStrictEqual(servers, {});
  assert.deepStrictEqual(failed.map((f) => f.name).sort(), ['a', 'b', 'c']);
});

test('a parsed document with no mcpServers key at all is an empty record, not a throw', () => {
  assert.deepStrictEqual(translateServers({}, {}), { servers: {}, failed: [] });
  assert.deepStrictEqual(translateServers(null, {}), { servers: {}, failed: [] });
});

test('an absent file is an empty record', () => {
  assert.deepStrictEqual(loadMcpServers('/nonexistent/agentops/mcp.json', {}), { servers: {}, failed: [] });
});

test('a read failure that is NOT ENOENT is one reported failure, distinct from an absent file', () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'agentops-pi-mcp-'));
  const got = loadMcpServers(dir, {});
  assert.deepStrictEqual(got.servers, {});
  assert.strictEqual(got.failed.length, 1);
  assert.strictEqual(got.failed[0].name, dir);
  assert.match(got.failed[0].reason, /^read: /);
});

test('an unparseable file is one reported failure', () => {
  const f = tmpFile();
  fs.writeFileSync(f, '{not json');
  const got = loadMcpServers(f, {});
  assert.deepStrictEqual(got.servers, {});
  assert.strictEqual(got.failed.length, 1);
  assert.match(got.failed[0].reason, /parse/);
});

test('the manager-written shape round-trips', () => {
  const f = tmpFile();
  fs.writeFileSync(f, JSON.stringify({ mcpServers: { kubernetes: { type: 'http', url: 'http://agentops-mcp-kubernetes:8080/mcp', headers: { Authorization: 'Bearer ${AGENTOPS_MCP_KUBERNETES_AUTHORIZATION}' } } } }));
  const got = loadMcpServers(f, { AGENTOPS_MCP_KUBERNETES_AUTHORIZATION: 'Bearer t' });
  assert.deepStrictEqual(got.failed, []);
  assert.strictEqual(got.servers.kubernetes.headers.Authorization, 'Bearer Bearer t');
});

// ---- task 3.3: allowlistFrom — the mcp__ gating predicate -----------------

test('allowlistFrom permits an exact mcp__<server>__<tool> pattern and nothing else of that server', () => {
  const allow = allowlistFrom(['mcp__github__search']);
  assert.strictEqual(allow('github', 'search'), true);
  assert.strictEqual(allow('github', 'other_tool'), false);
  assert.strictEqual(allow('other', 'search'), false);
});

test('allowlistFrom\'s mcp__<server>__* wildcard permits every tool of that server and no other\'s', () => {
  const allow = allowlistFrom(['mcp__github__*']);
  assert.strictEqual(allow('github', 'search'), true);
  assert.strictEqual(allow('github', 'anything'), true);
  assert.strictEqual(allow('other', 'search'), false);
});

test('allowlistFrom permits nothing from an empty or non-mcp allowlist', () => {
  assert.strictEqual(allowlistFrom([])('github', 'search'), false);
  assert.strictEqual(allowlistFrom(['Read', 'Bash(git:*)'])('github', 'search'), false);
});

// ---- task 3.3: connect, list, register — driven live over a real stdio ---
// ---- MCP server (test/fake-mcp-server.js), never a discovery of any -------
// ---- server MCP_CONFIG does not list --------------------------------------

test('a named tool the allowlist permits is registered as mcp__<server>__<tool>, and it actually calls the server', async () => {
  const f = tmpFile();
  fs.writeFileSync(f, JSON.stringify({ mcpServers: { github: stdioServer() } }));
  const pi = fakePi();
  const handle = await registerMcpTools(pi, f, ['mcp__github__echo'], {}, () => {});
  try {
    assert.strictEqual(pi.tools.length, 1);
    assert.strictEqual(pi.tools[0].name, 'mcp__github__echo');
    // registerMcpTools hands back every registered name, and every
    // connected client — runtime.js's openSession folds the former into
    // its own explicit `tools:` activation list, since pi.registerTool()
    // alone never activates one (task 3.3), and closes the latter once
    // the run ends (session_shutdown alone does not fire here).
    assert.deepStrictEqual(handle.toolNames, ['mcp__github__echo']);
    assert.strictEqual(handle.clients.length, 1);
    const result = await pi.tools[0].execute('call-1', { message: 'hi there' }, undefined);
    assert.deepStrictEqual(result.content, [{ type: 'text', text: 'hi there' }]);
    assert.deepStrictEqual(result.details, { server: 'github', tool: 'echo' });
  } finally {
    await pi.shutdown();
  }
});

test('a server wildcard permits every tool of that server', async () => {
  const f = tmpFile();
  const tools = JSON.stringify([
    { name: 'echo', description: 'echoes', inputSchema: { type: 'object', properties: { message: { type: 'string' } } } },
    { name: 'env-marker', description: 'reports env presence', inputSchema: { type: 'object', properties: { name: { type: 'string' } } } },
  ]);
  fs.writeFileSync(f, JSON.stringify({ mcpServers: { github: stdioServer({ env: { FAKE_MCP_TOOLS: tools } }) } }));
  const pi = fakePi();
  await registerMcpTools(pi, f, ['mcp__github__*'], {}, () => {});
  try {
    assert.deepStrictEqual(pi.tools.map((t) => t.name).sort(), ['mcp__github__echo', 'mcp__github__env-marker']);
  } finally {
    await pi.shutdown();
  }
});

test('a server nobody\'s allowlist entry names connects but registers none of its tools, and costs its sibling nothing', async () => {
  const f = tmpFile();
  fs.writeFileSync(f, JSON.stringify({ mcpServers: { github: stdioServer(), unbound: stdioServer() } }));
  const pi = fakePi();
  const logged = [];
  await registerMcpTools(pi, f, ['mcp__github__echo'], {}, (m) => logged.push(m));
  try {
    assert.deepStrictEqual(pi.tools.map((t) => t.name), ['mcp__github__echo']);
    assert.ok(!pi.tools.some((t) => t.name.startsWith('mcp__unbound__')));
    // "unbound" still connected (task 3.3: connect to every LISTED server) —
    // visible as a connected-with-zero-registered log line, not a failure.
    assert.ok(logged.some((l) => l.includes('unbound') && l.includes('0 registered')));
  } finally {
    await pi.shutdown();
  }
});

test('no MCP_CONFIG at all is no servers and no crash', async () => {
  const pi = fakePi();
  await registerMcpTools(pi, '/nonexistent/agentops/mcp.json', ['mcp__github__*'], {}, () => {});
  assert.deepStrictEqual(pi.tools, []);
  // No client was ever opened, so registerMcpTools never even asked for a
  // session_shutdown hook — nothing to shut down.
  assert.strictEqual(typeof pi.on, 'function');
});

test('a server that cannot be connected is logged and costs its sibling nothing', async () => {
  const f = tmpFile();
  fs.writeFileSync(f, JSON.stringify({ mcpServers: {
    broken: { command: '/nonexistent/agentops-mcp-binary-xyz', args: [] },
    github: stdioServer(),
  } }));
  const pi = fakePi();
  const logged = [];
  await registerMcpTools(pi, f, ['mcp__broken__*', 'mcp__github__echo'], {}, (m) => logged.push(m));
  try {
    assert.deepStrictEqual(pi.tools.map((t) => t.name), ['mcp__github__echo']);
    assert.ok(logged.some((l) => l.includes('broken') && l.includes('could not be connected')));
  } finally {
    await pi.shutdown();
  }
});

// ---- task 3.2: ${ENV} expansion happens in-process, right before --------
// ---- connecting, and the real secret value never reaches a log line, ----
// ---- a tool result, or anything that could end up in the transcript -----

test('a missing-var failure never lets the real secret value of a WORKING sibling server reach any log line or tool result', async () => {
  const f = tmpFile();
  const tools = JSON.stringify([{ name: 'env-marker', description: 'reports env presence', inputSchema: { type: 'object', properties: { name: { type: 'string' } } } }]);
  fs.writeFileSync(f, JSON.stringify({ mcpServers: {
    good: stdioServer({ env: { TOKEN: '${REAL_SECRET}', FAKE_MCP_TOOLS: tools } }),
    bad: { type: 'http', url: 'http://x', headers: { Authorization: 'Bearer ${MISSING_SECRET}' } },
  } }));
  const SECRET_VALUE = 'super-secret-value-must-never-be-logged';
  const logged = [];
  const pi = fakePi();
  await registerMcpTools(pi, f, ['mcp__good__*'], { REAL_SECRET: SECRET_VALUE }, (m) => logged.push(m));
  try {
    // The missing-var failure is logged, naming the variable, not bad's secret.
    assert.ok(logged.some((l) => l.includes('bad') && l.includes('MISSING_SECRET')));

    // The real secret, used to connect "good", appears in NO log line at all.
    const everyLoggedLine = logged.join('\n');
    assert.doesNotMatch(everyLoggedLine, new RegExp(SECRET_VALUE));

    // Drive the registered tool for real: the secret reached the server
    // over its environment, and the server reports only whether it was
    // SET, never the value — proving the value never flows back through a
    // result this runtime would log or carry into the transcript either.
    const envMarker = pi.tools.find((t) => t.name === 'mcp__good__env-marker');
    assert.ok(envMarker, 'the working server\'s tool must still register');
    const result = await envMarker.execute('call-1', { name: 'TOKEN' }, undefined);
    assert.strictEqual(result.content[0].text, 'present');
    assert.doesNotMatch(JSON.stringify(result), new RegExp(SECRET_VALUE));
  } finally {
    await pi.shutdown();
  }
});
