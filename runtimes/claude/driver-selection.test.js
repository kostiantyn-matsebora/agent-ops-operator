// cd runtime-claude && node --test
//
// runtime.js's own job is choosing between the two drivers and nothing else
// — tested here as the pure resolveDriver(envValue) it exports, in
// isolation from start()'s actual delegation. driver-sdk.js does not exist
// yet (a later change adds it), so this file deliberately never requires
// the whole entrypoint end to end — only the resolution logic a later
// `start()` call reads to decide which module to require.
'use strict';

const test = require('node:test');
const assert = require('node:assert');

const { resolveDriver, DEFAULT_DRIVER } = require('./runtime');

test('DEFAULT_DRIVER is sdk', () => {
  assert.strictEqual(DEFAULT_DRIVER, 'sdk');
});

test('RUNTIME_CLAUDE_DRIVER=sdk resolves to sdk, no warning', () => {
  assert.deepStrictEqual(resolveDriver('sdk'), { driver: 'sdk', warning: false });
});

test('RUNTIME_CLAUDE_DRIVER=cli resolves to cli, no warning', () => {
  assert.deepStrictEqual(resolveDriver('cli'), { driver: 'cli', warning: false });
});

test('an unset value resolves to the default (sdk), no warning', () => {
  assert.deepStrictEqual(resolveDriver(undefined), { driver: 'sdk', warning: false });
});

test('an empty string resolves to the default (sdk), no warning', () => {
  assert.deepStrictEqual(resolveDriver(''), { driver: 'sdk', warning: false });
});

test('whitespace-only resolves to the default (sdk), no warning', () => {
  assert.deepStrictEqual(resolveDriver('   '), { driver: 'sdk', warning: false });
});

test('an unrecognized value falls back to the default (sdk), WITH a warning', () => {
  assert.deepStrictEqual(resolveDriver('vendor-sdk-v2'), { driver: 'sdk', warning: true });
});

test('resolution is exact-match, not case-insensitive — SDK is unrecognized', () => {
  assert.deepStrictEqual(resolveDriver('SDK'), { driver: 'sdk', warning: true });
});
