// cd runtimes/copilot && node --test
'use strict';

const test = require('node:test');
const assert = require('node:assert');
const fs = require('fs');
const os = require('os');
const path = require('path');

const { expand, translateServers, loadMcpServers } = require('./mcp');

test('a stdio server carries command, args and env', () => {
  const { servers, failed } = translateServers({ mcpServers: { k8s: { command: 'mcp-k8s', args: ['--ro'], env: { A: '1' } } } }, {});
  assert.deepStrictEqual(failed, []);
  assert.deepStrictEqual(servers.k8s, { type: 'stdio', command: 'mcp-k8s', args: ['--ro'], env: { A: '1' } });
});

test('an http server carries url and headers', () => {
  const { servers, failed } = translateServers({ mcpServers: { prom: { type: 'http', url: 'http://prom:8080/mcp', headers: { 'X-A': 'b' } } } }, {});
  assert.deepStrictEqual(failed, []);
  assert.deepStrictEqual(servers.prom, { type: 'http', url: 'http://prom:8080/mcp', headers: { 'X-A': 'b' } });
});

test('a url without a type is http', () => {
  const { servers } = translateServers({ mcpServers: { s: { url: 'http://x/mcp' } } }, {});
  assert.strictEqual(servers.s.type, 'http');
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

test('a malformed server is reported, not registered', () => {
  const { servers, failed } = translateServers({ mcpServers: { a: { type: 'http' }, b: {}, c: 'nope' } }, {});
  assert.deepStrictEqual(servers, {});
  assert.deepStrictEqual(failed.map((f) => f.name).sort(), ['a', 'b', 'c']);
});

test('an absent file is an empty record', () => {
  assert.deepStrictEqual(loadMcpServers('/nonexistent/agentops/mcp.json', {}), { servers: {}, failed: [] });
});

test('an unparseable file is one reported failure', () => {
  const f = path.join(fs.mkdtempSync(path.join(os.tmpdir(), 'agentops-mcp-')), 'mcp.json');
  fs.writeFileSync(f, '{not json');
  const got = loadMcpServers(f, {});
  assert.deepStrictEqual(got.servers, {});
  assert.strictEqual(got.failed.length, 1);
  assert.match(got.failed[0].reason, /parse/);
});

test('the manager-written shape round-trips', () => {
  const f = path.join(fs.mkdtempSync(path.join(os.tmpdir(), 'agentops-mcp-')), 'mcp.json');
  fs.writeFileSync(f, JSON.stringify({ mcpServers: { kubernetes: { type: 'http', url: 'http://agentops-mcp-kubernetes:8080/mcp', headers: { Authorization: 'Bearer ${AGENTOPS_MCP_KUBERNETES_AUTHORIZATION}' } } } }));
  const got = loadMcpServers(f, { AGENTOPS_MCP_KUBERNETES_AUTHORIZATION: 'Bearer t' });
  assert.deepStrictEqual(got.failed, []);
  assert.strictEqual(got.servers.kubernetes.headers.Authorization, 'Bearer Bearer t');
});
