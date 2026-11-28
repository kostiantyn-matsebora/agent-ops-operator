// cd runtimes/copilot && node --test
//
// multi-repo-workspace: START_DIR (set only when the conversation's
// `workingDir` resolved one) decides where the Copilot session starts;
// absent, WORKSPACE itself does. Both consts are read at require() time, so
// this lives in its own file/process rather than sharing runtime.test.js's
// environment.
'use strict';

const test = require('node:test');
const assert = require('node:assert');
const fs = require('fs');
const os = require('os');
const path = require('path');

const WORKSPACE = fs.mkdtempSync(path.join(os.tmpdir(), 'agentops-copilot-startdir-ws-'));
const START_DIR = path.join(WORKSPACE, 'services', 'api');
process.env.WORKSPACE = WORKSPACE;
process.env.START_DIR = START_DIR;
process.env.HOME = fs.mkdtempSync(path.join(os.tmpdir(), 'agentops-copilot-startdir-home-'));

const { startDirectory, sessionConfig, WORKSPACE: exportedWorkspace, START_DIR: exportedStartDir } = require('./runtime');
const { translate } = require('./vocabulary');
const { loadMcpServers } = require('./mcp');

test('WORKSPACE and START_DIR are read from the environment this runtime no longer resolves itself', () => {
  assert.strictEqual(exportedWorkspace, WORKSPACE);
  assert.strictEqual(exportedStartDir, START_DIR);
});

test('startDirectory prefers START_DIR over WORKSPACE when the conversation named a workingDir', () => {
  assert.strictEqual(startDirectory(), START_DIR);
});

test('sessionConfig.workingDirectory is the resolved start directory, not the workspace root', () => {
  const grant = translate([]);
  const mcp = loadMcpServers('/does/not/exist/mcp.json');
  const cfg = sessionConfig({}, grant, mcp, []);
  assert.strictEqual(cfg.workingDirectory, START_DIR);
});
