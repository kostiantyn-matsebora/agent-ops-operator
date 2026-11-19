'use strict';

const test = require('node:test');
const assert = require('node:assert');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { piCodeEnabled, piCodeDir, loaderOptions } = require('./picode');

test('pi-code is on unless the env says off', () => {
  assert.strictEqual(piCodeEnabled({}), true);
  assert.strictEqual(piCodeEnabled({ PI_CODE_ENABLED: '' }), true);
  assert.strictEqual(piCodeEnabled({ PI_CODE_ENABLED: 'true' }), true);
  for (const off of ['false', 'FALSE', '0', 'no', 'off', ' false ']) {
    assert.strictEqual(piCodeEnabled({ PI_CODE_ENABLED: off }), false, off);
  }
});

test('off contributes nothing: no extension path, no approval, no env', () => {
  const calls = [];
  const sdk = { ProjectTrustStore: class { set() { calls.push('set'); } } };
  const env = { PI_CODE_ENABLED: 'false' };
  assert.deepStrictEqual(loaderOptions(sdk, { agentDir: '/a', workspace: '/w', env }), {});
  assert.deepStrictEqual(calls, []);
  assert.strictEqual(env.PI_CODING_AGENT_DIR, undefined);
});

test('on approves the workspace in the agent dir and returns the package path', () => {
  const calls = [];
  const sdk = { ProjectTrustStore: class {
    constructor(dir) { this.dir = dir; }
    set(cwd, decision) { calls.push([this.dir, cwd, decision]); }
  } };
  const env = {};
  const opts = loaderOptions(sdk, { agentDir: '/ctx/.pi/agent', workspace: '/data/workspace', env });
  assert.deepStrictEqual(calls, [['/ctx/.pi/agent', '/data/workspace', true]]);
  assert.strictEqual(env.PI_CODING_AGENT_DIR, '/ctx/.pi/agent', 'pi-code must read the dir the approval was written to');
  assert.deepStrictEqual(opts, { additionalExtensionPaths: [piCodeDir()] });
  assert.ok(fs.existsSync(path.join(piCodeDir(), 'package.json')));
});

// The real SDK loads the real package: no load error, and the Claude-compat
// surface is registered. Skipped, with its reason, when the SDK is absent.
test('the real package loads through the SDK without a load error', async (t) => {
  let sdk;
  try { sdk = await import('@earendil-works/pi-coding-agent'); } catch (e) { return t.skip(`SDK not installed: ${e.message}`); }
  const dir = fs.mkdtempSync(path.join(process.env.TMPDIR || os.tmpdir(), 'picode-'));
  const env = {};
  const agentDir = path.join(dir, 'agent');
  fs.mkdirSync(agentDir, { recursive: true });
  const loader = new sdk.DefaultResourceLoader({
    cwd: dir, agentDir, agentsFilesOverride: () => ({ agentsFiles: [] }),
    ...loaderOptions(sdk, { agentDir, workspace: dir, env }),
  });
  await loader.reload();
  const { extensions, errors } = loader.getExtensions();
  assert.deepStrictEqual(errors, []);
  const commands = new Set(extensions.flatMap((e) => [...e.commands.keys()]));
  for (const c of ['hooks', 'output-style', 'init']) assert.ok(commands.has(c), `command ${c} registered`);
  assert.strictEqual(new sdk.ProjectTrustStore(agentDir).get(dir), true);
});
