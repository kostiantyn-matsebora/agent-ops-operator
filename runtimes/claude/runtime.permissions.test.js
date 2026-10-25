// cd runtime-claude && node --test
//
// openspec/changes/claude-runtime-sdk/tasks.md 9.3 — the canUseTool/
// createCanUseTool/allowlistMatch permission callback (design.md D3): no
// `allowedTools`, no `dontAsk`, deny by default, the composed allowlist
// (tools.js, unchanged) as the callback's own rule set. Pure functions and a
// bound callback, no SDK import reached — require() alone proves that.
'use strict';

const test = require('node:test');
const assert = require('node:assert');

const { allowlistMatch, createCanUseTool } = require('./runtime');

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

// allowedHolder is a MUTABLE box specifically so the allowlist excluded from
// the fingerprint (D4) still reaches a callback already bound at session
// open — this is the property that makes that exclusion safe.
test('canUseTool reads the CURRENT allowlist from the mutable holder, not the one bound at creation', async () => {
  const holder = { current: ['Read'] };
  const canUseTool = createCanUseTool(holder);
  assert.strictEqual((await canUseTool('Bash')).behavior, 'deny');
  holder.current = ['Bash']; // a later unit updates allowedHolder.current with no re-open
  assert.strictEqual((await canUseTool('Bash')).behavior, 'allow');
});
