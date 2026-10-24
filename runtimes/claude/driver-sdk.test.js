// cd runtime-claude && node --test
//
// driver-sdk.js's pure/stateless helpers: the SDK-import seam, the
// streaming-input channel and message shape, the transcript formatter
// reused from driver-cli.js (D7), the MCP-config reader, the configuration
// fingerprint (D4) and the permission-callback matcher (D3). None of these
// touch the module-level live session, so they need no reset between tests
// — see driver-sdk.session.test.js for the stateful executeUnit() suite.
'use strict';

const test = require('node:test');
const assert = require('node:assert');
const fs = require('fs');
const os = require('os');
const path = require('path');

function tmp(prefix) {
  return fs.mkdtempSync(path.join(os.tmpdir(), prefix));
}

process.env.CONTROL_URL = 'http://127.0.0.1:1';
process.env.CONVO_ID = 'conv-sdk-pure';
process.env.REPO_URL = '';
process.env.HOME = tmp('agentops-sdkpure-home-');
process.env.WORKSPACE = tmp('agentops-sdkpure-ws-');

const sdk = require('./driver-sdk');

// ---- the SDK-import seam --------------------------------------------------

test('getQuery returns whatever __setQueryForTest installed, never requiring the real package', () => {
  const fake = () => {};
  sdk.__setQueryForTest(fake);
  assert.strictEqual(sdk.getQuery(), fake);
});

// ---- makeInputChannel: streaming input (D2) -------------------------------

test('makeInputChannel delivers a push made before anyone reads', async () => {
  const ch = sdk.makeInputChannel();
  ch.push('first');
  const { value, done } = await ch.iterator.next();
  assert.strictEqual(value, 'first');
  assert.strictEqual(done, false);
});

test('makeInputChannel delivers a push made while a read is already pending', async () => {
  const ch = sdk.makeInputChannel();
  const pending = ch.iterator.next();
  ch.push('second');
  assert.strictEqual((await pending).value, 'second');
});

test('makeInputChannel queues a second push ahead of a third read', async () => {
  const ch = sdk.makeInputChannel();
  ch.push('a');
  ch.push('b');
  assert.strictEqual((await ch.iterator.next()).value, 'a');
  assert.strictEqual((await ch.iterator.next()).value, 'b');
});

test('makeSdkUserMessage builds the minimal valid SDKUserMessage shape', () => {
  const msg = sdk.makeSdkUserMessage('hello there');
  assert.strictEqual(msg.type, 'user');
  assert.deepStrictEqual(msg.message, { role: 'user', content: 'hello there' });
  assert.strictEqual(msg.parent_tool_use_id, null);
});

// ---- formatSdkEvent: transcript rendering, reused from driver-cli.js (D7) ----

test('formatSdkEvent renders an init message exactly as driver-cli.js\'s formatEvent does', () => {
  const line = sdk.formatSdkEvent({
    type: 'system', subtype: 'init', model: 'claude-x',
    tools: ['Read', 'Bash'], mcp_servers: [{ name: 'k8s', status: 'connected' }],
  });
  assert.match(line, /\[init\] model=claude-x tools=2 mcp=k8s:connected/);
});

test('formatSdkEvent renders an assistant text block', () => {
  const line = sdk.formatSdkEvent({ type: 'assistant', message: { content: [{ type: 'text', text: 'hi there' }] } });
  assert.strictEqual(line, '[claude] hi there\n');
});

test('formatSdkEvent renders a result message with subtype, turns and duration', () => {
  const line = sdk.formatSdkEvent({ type: 'result', subtype: 'success', num_turns: 2, duration_ms: 2000, result: 'done' });
  assert.match(line, /=== RESULT \(success, 2 turns, 2s\) ===/);
  assert.match(line, /done/);
});

test('formatSdkEvent is literally driver-cli.js\'s formatEvent, not a reimplementation', () => {
  const { formatEvent } = require('./driver-cli');
  assert.strictEqual(sdk.formatSdkEvent, formatEvent);
});

// ---- readMcpServers --------------------------------------------------------

test('readMcpServers reads the mcpServers object out of the composed mcp.json', () => {
  const file = path.join(tmp('agentops-sdkmcp-'), 'mcp.json');
  fs.writeFileSync(file, JSON.stringify({ mcpServers: { k8s: { command: 'x' } } }));
  assert.deepStrictEqual(sdk.readMcpServers(file), { k8s: { command: 'x' } });
});

test('readMcpServers treats an empty-but-valid document as no MCP servers', () => {
  const file = path.join(tmp('agentops-sdkmcp-'), 'mcp.json');
  fs.writeFileSync(file, JSON.stringify({ mcpServers: {} }));
  assert.deepStrictEqual(sdk.readMcpServers(file), {});
});

test('readMcpServers treats a missing file as no MCP servers, logging why', () => {
  const logs = [];
  const out = sdk.readMcpServers(path.join(os.tmpdir(), 'agentops-never-created', 'mcp.json'), (m) => logs.push(m));
  assert.deepStrictEqual(out, {});
  assert.ok(logs.some((l) => l.includes('mcp config')), `expected a reason logged, got: ${logs.join(' | ')}`);
});

test('readMcpServers treats unparsable JSON as no MCP servers, logging why', () => {
  const file = path.join(tmp('agentops-sdkmcp-'), 'mcp.json');
  fs.writeFileSync(file, '{not json');
  const logs = [];
  assert.deepStrictEqual(sdk.readMcpServers(file, (m) => logs.push(m)), {});
  assert.ok(logs.length > 0);
});

// ---- configuration fingerprint (D4), field by field -------------------------

test('computeFingerprint carries systemPrompt, mcpServers, maxTurns, cwd and agent', () => {
  const fp = sdk.computeFingerprint({ systemPrompt: 'be terse', maxTurns: 20, agent: 'ops' }, { k8s: {} });
  assert.strictEqual(fp.systemPrompt, 'be terse');
  assert.deepStrictEqual(fp.mcpServers, { k8s: {} });
  assert.strictEqual(fp.maxTurns, 20);
  assert.strictEqual(fp.agent, 'ops');
  assert.strictEqual(fp.cwd, sdk.WORKSPACE);
});

test('computeFingerprint normalizes an absent maxTurns to the same 60-turn floor driver-cli.js passes', () => {
  const fp = sdk.computeFingerprint({ agent: 'ops' }, {});
  assert.strictEqual(fp.maxTurns, 60);
});

test('changedFingerprintFields names only systemPrompt when only it differs', () => {
  const a = sdk.computeFingerprint({ systemPrompt: 'x', agent: 'ops' }, {});
  const b = sdk.computeFingerprint({ systemPrompt: 'y', agent: 'ops' }, {});
  assert.deepStrictEqual(sdk.changedFingerprintFields(a, b), ['systemPrompt']);
});

test('changedFingerprintFields names mcpServers when the composed MCP config changed', () => {
  const a = sdk.computeFingerprint({ agent: 'ops' }, { k8s: {} });
  const b = sdk.computeFingerprint({ agent: 'ops' }, { weather: {} });
  assert.deepStrictEqual(sdk.changedFingerprintFields(a, b), ['mcpServers']);
});

test('changedFingerprintFields names maxTurns when it changed', () => {
  const a = sdk.computeFingerprint({ agent: 'ops', maxTurns: 10 }, {});
  const b = sdk.computeFingerprint({ agent: 'ops', maxTurns: 20 }, {});
  assert.deepStrictEqual(sdk.changedFingerprintFields(a, b), ['maxTurns']);
});

test('changedFingerprintFields names agent when it changed', () => {
  const a = sdk.computeFingerprint({ agent: 'ops' }, {});
  const b = sdk.computeFingerprint({ agent: 'release-notes' }, {});
  assert.deepStrictEqual(sdk.changedFingerprintFields(a, b), ['agent']);
});

test('changedFingerprintFields names cwd when it differs between two fingerprints', () => {
  const a = sdk.computeFingerprint({ agent: 'ops' }, {});
  const b = { ...sdk.computeFingerprint({ agent: 'ops' }, {}), cwd: '/data/elsewhere' };
  assert.deepStrictEqual(sdk.changedFingerprintFields(a, b), ['cwd']);
});

test('fingerprintsEqual is true only when every field matches, false on any single difference', () => {
  const a = sdk.computeFingerprint({ agent: 'ops' }, { k8s: {} });
  const b = sdk.computeFingerprint({ agent: 'ops' }, { k8s: {} });
  assert.strictEqual(sdk.fingerprintsEqual(a, b), true);
  const c = sdk.computeFingerprint({ agent: 'other' }, { k8s: {} });
  assert.strictEqual(sdk.fingerprintsEqual(a, c), false);
});

test('an allowlist-only difference never changes the fingerprint — allowedTools/toolsMode are not fingerprint inputs (4.2/4.4)', () => {
  const a = sdk.computeFingerprint({ agent: 'ops', allowedTools: 'Bash', toolsMode: 'merge' }, {});
  const b = sdk.computeFingerprint({ agent: 'ops', allowedTools: 'Read,Write', toolsMode: 'overwrite' }, {});
  assert.strictEqual(sdk.fingerprintsEqual(a, b), true);
  assert.deepStrictEqual(sdk.changedFingerprintFields(a, b), []);
});

// ---- permission matching (D3) -----------------------------------------------

test('allowlistMatch matches a bare tool name exactly', () => {
  assert.strictEqual(sdk.allowlistMatch(['Bash', 'Read'], 'Bash'), 'Bash');
  assert.strictEqual(sdk.allowlistMatch(['Bash', 'Read'], 'Write'), null);
});

test('allowlistMatch matches a scoped entry by its base name (5.3) — a documented, narrower guarantee than the CLI\'s own sub-pattern enforcement', () => {
  assert.strictEqual(sdk.allowlistMatch(['Bash(git *)'], 'Bash'), 'Bash(git *)');
});

test('allowlistMatch matches an exact MCP tool name', () => {
  assert.strictEqual(sdk.allowlistMatch(['mcp__k8s__get_pods'], 'mcp__k8s__get_pods'), 'mcp__k8s__get_pods');
  assert.strictEqual(sdk.allowlistMatch(['mcp__k8s__get_pods'], 'mcp__k8s__delete_pod'), null);
});

test('allowlistMatch treats a bare mcp__<server> entry as a server-wide grant', () => {
  assert.strictEqual(sdk.allowlistMatch(['mcp__weather'], 'mcp__weather__forecast'), 'mcp__weather');
  assert.strictEqual(sdk.allowlistMatch(['mcp__weather'], 'mcp__weather'), 'mcp__weather');
});

test('allowlistMatch denies everything against an empty allowlist', () => {
  assert.strictEqual(sdk.allowlistMatch([], 'Bash'), null);
  assert.strictEqual(sdk.allowlistMatch(undefined, 'Bash'), null);
});

test('createCanUseTool allows a tool matching the current allowlist', async () => {
  const cb = sdk.createCanUseTool({ current: ['Bash'] });
  assert.deepStrictEqual(await cb('Bash', { command: 'ls' }), { behavior: 'allow' });
});

test('createCanUseTool denies a tool with no matching entry', async () => {
  const cb = sdk.createCanUseTool({ current: ['Bash'] });
  const decision = await cb('Read', {});
  assert.strictEqual(decision.behavior, 'deny');
  assert.match(decision.message, /Read/);
});

test('createCanUseTool denies everything when the composition is empty — no tool substitutes for one nobody declared (RT-3.2)', async () => {
  const cb = sdk.createCanUseTool({ current: [] });
  assert.strictEqual((await cb('Bash', {})).behavior, 'deny');
  assert.strictEqual((await cb('mcp__k8s__get_pods', {})).behavior, 'deny');
});

test('createCanUseTool reads the CURRENT allowlist, not the one its holder had when the callback was created (D4\'s mutable holder)', async () => {
  const holder = { current: ['Bash'] };
  const cb = sdk.createCanUseTool(holder);
  assert.strictEqual((await cb('Read', {})).behavior, 'deny');
  holder.current = ['Read'];
  assert.strictEqual((await cb('Read', {})).behavior, 'allow');
  assert.strictEqual((await cb('Bash', {})).behavior, 'deny', 'Bash left the allowlist when the holder was updated');
});

test('createCanUseTool logs a denial naming the tool and the current allowlist (5.4)', async () => {
  const logs = [];
  const cb = sdk.createCanUseTool({ current: ['Bash'] }, (m) => logs.push(m));
  await cb('Read', {});
  assert.ok(logs.some((l) => l.includes('Read') && l.includes('Bash')), `expected the tool and allowlist logged, got: ${logs.join(' | ')}`);
});

test('createCanUseTool logs "(none)" for an empty allowlist rather than an empty string', async () => {
  const logs = [];
  const cb = sdk.createCanUseTool({ current: [] }, (m) => logs.push(m));
  await cb('Bash', {});
  assert.ok(logs.some((l) => l.includes('(none)')));
});
