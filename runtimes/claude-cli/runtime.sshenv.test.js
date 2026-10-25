// cd runtime-claude && node --test
//
// gitEnv()'s ssh branch reads GIT_AUTH_TYPE/GIT_SSH_KEY, both captured into
// module-level consts at require time — a separate process/file from the
// default-env suite in runtime.test.js, which exercises the false branch.
'use strict';

const test = require('node:test');
const assert = require('node:assert');

process.env.CONTROL_URL = 'http://127.0.0.1:1';
process.env.CONVO_ID = 'conv-ssh';
process.env.GIT_AUTH_TYPE = 'ssh';
process.env.GIT_SSH_KEY = '/tmp/agentops-fake-deploy-key';

const { gitEnv } = require('./runtime');

test('gitEnv sets GIT_SSH_COMMAND to the configured key with a lenient known_hosts policy', () => {
  const env = gitEnv();
  assert.match(env.GIT_SSH_COMMAND, /^ssh -i \/tmp\/agentops-fake-deploy-key /);
  assert.match(env.GIT_SSH_COMMAND, /StrictHostKeyChecking=accept-new/);
});

test('gitEnv still carries the rest of process.env alongside the override', () => {
  const env = gitEnv();
  assert.strictEqual(env.PATH, process.env.PATH);
});
