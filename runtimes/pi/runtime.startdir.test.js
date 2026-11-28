// cd runtimes/pi && node --test
//
// multi-repo-workspace: START_DIR (set only when the conversation's
// workingDir resolved one) decides where the pi agent starts; absent,
// WORKSPACE itself does. Both consts are read at require() time, so this
// lives in its own file/process rather than sharing runtime.test.js's
// environment.
'use strict';

const test = require('node:test');
const assert = require('node:assert');
const fs = require('fs');
const os = require('os');
const path = require('path');

const WORKSPACE = fs.mkdtempSync(path.join(os.tmpdir(), 'agentops-pi-startdir-ws-'));
const START_DIR = path.join(WORKSPACE, 'services', 'api');
process.env.CONTROL_URL = 'http://127.0.0.1:1';
process.env.CONVO_ID = 'conv-startdir';
process.env.PI_API_KEY = 'test-key';
process.env.PI_MODEL = 'fake-model';
process.env.WORKSPACE = WORKSPACE;
process.env.START_DIR = START_DIR;
process.env.HOME = fs.mkdtempSync(path.join(os.tmpdir(), 'agentops-pi-startdir-home-'));

const rt = require('./runtime');
const { startDirectory, WORKSPACE: exportedWorkspace, START_DIR: exportedStartDir } = rt;

test('WORKSPACE and START_DIR are read from the environment this runtime no longer resolves itself', () => {
  assert.strictEqual(exportedWorkspace, WORKSPACE);
  assert.strictEqual(exportedStartDir, START_DIR);
});

test('startDirectory prefers START_DIR over WORKSPACE when the conversation named a workingDir', () => {
  assert.strictEqual(startDirectory(), START_DIR);
});

test("buildResourceLoader's cwd is the resolved start directory, not the workspace root", async (t) => {
  // pi-code off for this test only: its own extension would reach for a
  // real sdk.ProjectTrustStore this fake sdk does not provide.
  const prevPiCode = process.env.PI_CODE_ENABLED;
  process.env.PI_CODE_ENABLED = 'false';
  t.after(() => { process.env.PI_CODE_ENABLED = prevPiCode; });

  const recorded = {};
  class FakeResourceLoader {
    constructor(opts) { recorded.opts = opts; }
    async reload() { /* extensionFactories are never driven by this fake */ }
  }
  const fakeSdk = { DefaultResourceLoader: FakeResourceLoader };
  const grant = { builtinNames: [], mcpPatterns: [] };

  await rt.buildResourceLoader(fakeSdk, { promptText: 'go' }, grant, '', []);
  assert.strictEqual(recorded.opts.cwd, START_DIR);
});
