// cd runtime-claude && node --test
//
// A CLI binary that cannot be found at all: PATH is pointed at an empty
// directory before requiring runtime.js, so resolveBin('claude') falls back
// to the bare name and the real spawn() call ENOENTs — exercising
// spawnClaude's `p.on('error', ...)` path with a genuine failed spawn, not a
// mock standing in for one.
'use strict';

const test = require('node:test');
const assert = require('node:assert');
const fs = require('fs');
const os = require('os');
const path = require('path');

function tmp(prefix) {
  return fs.mkdtempSync(path.join(os.tmpdir(), prefix));
}

process.env.PATH = tmp('agentops-emptybin-'); // guaranteed to hold no `claude`
process.env.CONTROL_URL = 'http://127.0.0.1:1';
process.env.CONVO_ID = 'conv-spawnerror';
process.env.WORKSPACE = tmp('agentops-ws-');

const { runClaude } = require('./runtime');

test('a CLI binary that cannot be resolved fails the run with the spawn error, rather than throwing', async () => {
  const out = await runClaude({ promptText: 'hi', runId: 'r1' });
  assert.strictEqual(out.status, 'failed');
  assert.strictEqual(out.exitCode, -1);
  assert.match(out.result, /^spawn: /);
});
