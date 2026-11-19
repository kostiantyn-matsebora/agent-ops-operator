// cd runtimes/pi && node --test
'use strict';

const test = require('node:test');
const assert = require('node:assert');

const { BUILTIN, translate, checkInventory, decide } = require('./vocabulary');

test('each built-in maps to pi\'s own wire name (design D2)', () => {
  assert.deepStrictEqual(BUILTIN, {
    Read: 'read', Write: 'write', Edit: 'edit', Bash: 'bash', Grep: 'grep', Glob: 'find',
  });
  for (const [agentOpsName, wireName] of Object.entries(BUILTIN)) {
    const got = translate([agentOpsName]);
    assert.deepStrictEqual(got.builtinNames, [wireName], `${agentOpsName} -> ${wireName}`);
    assert.deepStrictEqual(got.unmapped, []);
  }
});

test('a scoped Bash(<prefix>:*) pattern enables nothing and is logged separately from unmapped', () => {
  const got = translate(['Bash(git:*)']);
  assert.deepStrictEqual(got.builtinNames, []);
  assert.deepStrictEqual(got.scoped, ['Bash(git:*)']);
  assert.deepStrictEqual(got.unmapped, []);
});

test('an unrecognised pattern contributes nothing and is logged as unmapped', () => {
  const got = translate(['SomeVendorTool']);
  assert.deepStrictEqual(got.builtinNames, []);
  assert.deepStrictEqual(got.unmapped, ['SomeVendorTool']);
});

test('an mcp__ pattern is recognised as its own category, neither scoped nor unmapped', () => {
  const got = translate(['mcp__kubernetes__pods_list']);
  assert.deepStrictEqual(got.builtinNames, []);
  assert.deepStrictEqual(got.mcpPatterns, ['mcp__kubernetes__pods_list']);
  assert.deepStrictEqual(got.unmapped, []);
  assert.deepStrictEqual(got.scoped, []);
});

test('translate dedups and preserves first-seen order across a mixed allowlist', () => {
  const got = translate(['Read', 'Bash(git:*)', 'Write', 'Read', 'NotReal']);
  assert.deepStrictEqual(got.builtinNames, ['read', 'write']);
  assert.deepStrictEqual(got.scoped, ['Bash(git:*)']);
  assert.deepStrictEqual(got.unmapped, ['NotReal']);
});

test('translate ignores blank/empty entries', () => {
  const got = translate(['', '   ', 'Read']);
  assert.deepStrictEqual(got.builtinNames, ['read']);
});

test('checkInventory logs an inventory gap for a granted name the session never registered', () => {
  const gaps = checkInventory(['read', 'write'], ['read']);
  assert.strictEqual(gaps.length, 1);
  assert.match(gaps[0], /"write" is not registered/);
});

test('checkInventory reports nothing when every granted name is registered', () => {
  assert.deepStrictEqual(checkInventory(['read'], ['read', 'bash']), []);
});

test('decide grants an exact match and denies everything else, deny by default', () => {
  const granted = ['read', 'bash'];
  assert.strictEqual(decide(granted, 'read').allow, true);
  assert.strictEqual(decide(granted, 'write').allow, false);
  assert.match(decide(granted, 'write').why, /not in the composed allowlist/);
});

test('decide denies an unnamed/empty tool call rather than defaulting open', () => {
  const got = decide(['read'], '');
  assert.strictEqual(got.allow, false);
});
