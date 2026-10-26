// cd runtime-claude && node --test
//
// openspec/changes/claude-runtime-sdk/tasks.md 9.3 — the canUseTool/
// createCanUseTool/allowlistMatch permission callback (design.md D3): no
// `allowedTools`, no `dontAsk`, deny by default, the composed allowlist
// (tools.js, unchanged) as the callback's own rule set. Pure functions and a
// bound callback, no SDK import reached — require() alone proves that.
//
// coordinator-move-per-turn 6.2 (design D-F) adds the OFFERED side: the
// SDK's `tools` option, derived from the same allowlist, asserted at the end
// against a fake query() that records the options it is handed.
'use strict';

const test = require('node:test');
const assert = require('node:assert');

const {
  allowlistMatch, createCanUseTool, openSession, computeFingerprint,
  __setQueryForTest, __resetLiveSessionForTest,
} = require('./runtime');

// ---- allowlistMatch ---------------------------------------------------------

test('allowlistMatch allows a bare tool name present in the allowlist', () => {
  assert.strictEqual(allowlistMatch(['Read', 'Bash'], 'Read'), 'Read');
});

test('allowlistMatch denies a tool with no matching entry', () => {
  assert.strictEqual(allowlistMatch(['Read'], 'Bash'), null);
});

test('allowlistMatch allows a server-wide MCP grant covering any tool on that server', () => {
  assert.strictEqual(allowlistMatch(['mcp__kubernetes'], 'mcp__kubernetes__pods_list'), 'mcp__kubernetes');
});

test('allowlistMatch denies an MCP tool on a different server than the one granted', () => {
  assert.strictEqual(allowlistMatch(['mcp__kubernetes'], 'mcp__prometheus__query'), null);
});

// Every shipped MCPToolset (chart/values.yaml's `aops-coordinate` among them)
// authors a whole-server grant as `mcp__<server>__*`, never the bare
// `mcp__<server>` form above — measured live: a real coordinator conversation
// had every mcp__aops__* call denied until allowlistMatch stripped this
// trailing glob before comparing (PR #426 round 2).
test('allowlistMatch allows a server-wide MCP grant authored with the trailing __* glob chart values actually use', () => {
  assert.strictEqual(allowlistMatch(['mcp__aops__*'], 'mcp__aops__list_agents'), 'mcp__aops__*');
  assert.strictEqual(allowlistMatch(['mcp__aops__*'], 'mcp__aops__invoke'), 'mcp__aops__*');
});

test('allowlistMatch denies an MCP tool on a different server than a __* grant names', () => {
  assert.strictEqual(allowlistMatch(['mcp__aops__*'], 'mcp__prometheus__query'), null);
});

// A scoped CLI-style entry like `Bash(git *)` is matched (and therefore
// granted) at BASE TOOL NAME granularity only — not the real CLI's own
// sub-command glob. That is a deliberate, narrower guarantee documented on
// allowlistMatch itself (runtime.js), not a bug this test failed to catch:
// `Bash(git *)` here grants every Bash call, including one the real CLI's
// own --allowedTools parser would refuse for running something other than
// `git`.
test('a scoped entry like `Bash(git *)` matches at base-tool-name granularity, narrower than the CLI\'s own sub-command parser', () => {
  assert.strictEqual(allowlistMatch(['Bash(git *)'], 'Bash'), 'Bash(git *)');
});

test('an empty composition denies everything, including an ordinary built-in tool', () => {
  assert.strictEqual(allowlistMatch([], 'Read'), null);
  assert.strictEqual(allowlistMatch(undefined, 'Read'), null);
});

// ---- createCanUseTool / the canUseTool callback itself ----------------------

test('canUseTool allows a tool matched in the current allowlist', async () => {
  const canUseTool = createCanUseTool({ current: ['Read'] });
  assert.deepStrictEqual(await canUseTool('Read'), { behavior: 'allow' });
});

// The SDK's own type requires `message` on a deny decision — asserting its
// presence and type here, not a shape that omits it.
test('canUseTool denies a tool with no matching allowlist entry, carrying the message the SDK type requires', async () => {
  const canUseTool = createCanUseTool({ current: ['Read'] });
  const decision = await canUseTool('Bash');
  assert.strictEqual(decision.behavior, 'deny');
  assert.strictEqual(typeof decision.message, 'string');
  assert.ok(decision.message.length > 0, 'the SDK requires a non-empty message on a deny decision');
  assert.match(decision.message, /Bash/);
});

test('canUseTool denies every tool, built-in or MCP, when the composed allowlist is empty', async () => {
  const canUseTool = createCanUseTool({ current: [] });
  assert.strictEqual((await canUseTool('Read')).behavior, 'deny');
  assert.strictEqual((await canUseTool('mcp__kubernetes__pods_list')).behavior, 'deny');
});

test('canUseTool logs the denial with the tool name and the allowlist it failed to match', async () => {
  const logged = [];
  const canUseTool = createCanUseTool({ current: ['Read'] }, (m) => logged.push(m));
  await canUseTool('Bash');
  assert.ok(
    logged.some((l) => l.includes('Bash') && l.includes('Read')),
    `expected the denial logged with the pattern that failed to match, got: ${logged.join(' | ')}`,
  );
});

// allowedHolder is a MUTABLE box specifically so the allowlist kept out of
// the fingerprint (D4) still reaches a callback already bound at session
// open — this is the property that makes that exclusion safe.
test('canUseTool reads the CURRENT allowlist from the mutable holder, not the one bound at creation', async () => {
  const holder = { current: ['Read'] };
  const canUseTool = createCanUseTool(holder);
  assert.strictEqual((await canUseTool('Bash')).behavior, 'deny');
  holder.current = ['Bash']; // a later unit updates allowedHolder.current with no re-open
  assert.strictEqual((await canUseTool('Bash')).behavior, 'allow');
});

// ---- the offered built-ins: the SDK's `tools` option (D-F) -----------------

// recordingQuery substitutes query() with one that only records the options
// it was handed — the real SDK is never require()'d here.
function recordingQuery(t) {
  const calls = [];
  __setQueryForTest(({ options }) => {
    calls.push(options);
    return { async next() { return { value: undefined, done: true }; }, async close() {}, async interrupt() {} };
  });
  t.after(() => { __setQueryForTest(null); __resetLiveSessionForTest(); });
  return calls;
}

function optionsFor(t, allowed) {
  const calls = recordingQuery(t);
  const unit = { runId: 'r1' };
  openSession(unit, computeFingerprint(unit, {}, allowed), allowed, {});
  assert.strictEqual(calls.length, 1, 'openSession calls query() exactly once');
  return calls[0];
}

// The coordinator's shape: `mcp__aops__*` alone. No Bash/Read/Glob/Edit/
// Write/web tool is offered — `[]` is the SDK's "disable all built-in tools"
// (sdk.d.ts, Options.tools) — while the aops server stays bound as before.
test('openSession offers no built-in for an aops-only allowlist: `tools` is the empty list', (t) => {
  const options = optionsFor(t, ['mcp__aops__*']);
  assert.deepStrictEqual(options.tools, []);
});

test('openSession offers exactly the granted built-ins for a Read,Grep allowlist', (t) => {
  const options = optionsFor(t, ['Read', 'Grep']);
  assert.deepStrictEqual(options.tools, ['Read', 'Grep']);
});

// Offered and allowed are two decisions. Setting `tools` must not have
// turned into `allowedTools` (which would auto-approve before the callback,
// D3) or moved the permission mode, and the MCP binding is untouched.
test('openSession keeps canUseTool, permissionMode default, mcpServers and strictMcpConfig exactly as before, with no allowedTools', (t) => {
  const calls = recordingQuery(t);
  const mcpServers = { aops: { type: 'http', url: 'http://aops' } };
  const allowed = ['mcp__aops__*', 'Read'];
  const unit = { runId: 'r1' };
  openSession(unit, computeFingerprint(unit, mcpServers, allowed), allowed, mcpServers);
  const options = calls[0];
  assert.strictEqual(typeof options.canUseTool, 'function');
  assert.strictEqual(options.permissionMode, 'default');
  assert.strictEqual(options.allowedTools, undefined, 'D3: nothing pre-approves, `tools` is not `allowedTools`');
  assert.strictEqual(options.disallowedTools, undefined, 'the pinned SDK has `tools`; the disallowedTools fallback is not wired');
  assert.deepStrictEqual(options.mcpServers, mcpServers);
  assert.strictEqual(options.strictMcpConfig, true);
  assert.deepStrictEqual(options.tools, ['Read']);
});
