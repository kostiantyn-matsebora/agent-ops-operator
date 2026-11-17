// cd runtimes/pi && node --test
'use strict';

const test = require('node:test');
const assert = require('node:assert');
const fs = require('fs');
const os = require('os');
const path = require('path');

const { readAgentDefinition, composeAllowedTools, parseFrontmatter, MODE_OVERWRITE } = require('./tools');

function mkWorkspace() {
  return fs.mkdtempSync(path.join(os.tmpdir(), 'agentops-pi-tools-'));
}

function writeAgent(ws, name, text) {
  const dir = path.join(ws, '.claude', 'agents');
  fs.mkdirSync(dir, { recursive: true });
  fs.writeFileSync(path.join(dir, `${name}.md`), text);
}

test('readAgentDefinition returns the declared tools and the body text', () => {
  const ws = mkWorkspace();
  writeAgent(ws, 'reviewer', '---\ntools: Read, Grep\n---\nYou are a careful reviewer.\nStay terse.');
  const got = readAgentDefinition(ws, 'reviewer');
  assert.deepStrictEqual(got.tools, ['Read', 'Grep']);
  assert.strictEqual(got.body, 'You are a careful reviewer.\nStay terse.');
});

test('a definition with no frontmatter is entirely body, declaring no tools', () => {
  const ws = mkWorkspace();
  writeAgent(ws, 'plain', 'Just be helpful.');
  const got = readAgentDefinition(ws, 'plain');
  assert.deepStrictEqual(got.tools, []);
  assert.strictEqual(got.body, 'Just be helpful.');
});

test('a missing file contributes nothing and never blocks', () => {
  const ws = mkWorkspace();
  const got = readAgentDefinition(ws, 'nobody-home');
  assert.deepStrictEqual(got.tools, []);
  assert.strictEqual(got.body, '');
});

test('unreadable/unparseable frontmatter (unclosed ---) contributes nothing and logs once', () => {
  const ws = mkWorkspace();
  writeAgent(ws, 'broken', '---\ntools: Read\nStay terse.');
  const logs = [];
  const got = readAgentDefinition(ws, 'broken', (m) => logs.push(m));
  assert.deepStrictEqual(got.tools, []);
  assert.strictEqual(got.body, '');
  assert.strictEqual(logs.length, 1);
  assert.match(logs[0], /never closed/);
});

test('an agent name that would escape the workspace is refused, not traversed', () => {
  const ws = mkWorkspace();
  const logs = [];
  const got = readAgentDefinition(ws, '../../../etc/passwd', (m) => logs.push(m));
  assert.deepStrictEqual(got.tools, []);
  assert.strictEqual(got.body, '');
  assert.strictEqual(logs.length, 1);
  assert.match(logs[0], /escapes the workspace/);
});

test('an absent workspace or agent name short-circuits to empty, no filesystem access', () => {
  assert.deepStrictEqual(readAgentDefinition('', 'x'), { tools: [], body: '' });
  assert.deepStrictEqual(readAgentDefinition('/tmp', ''), { tools: [], body: '' });
});

test('composeAllowedTools merge unions the agent declaration with the wiring, agent first', () => {
  const got = composeAllowedTools(['Read'], ['Bash', 'Read'], 'merge');
  assert.deepStrictEqual(got, ['Read', 'Bash']);
});

test('composeAllowedTools overwrite passes the wiring alone, dropping the agent declaration', () => {
  const got = composeAllowedTools(['Read', 'Edit'], ['Bash'], MODE_OVERWRITE);
  assert.deepStrictEqual(got, ['Bash']);
});

test('composeAllowedTools with neither side produces an empty, explicit composition', () => {
  assert.deepStrictEqual(composeAllowedTools([], [], 'merge'), []);
});

test('composeAllowedTools treats an unrecognised mode as merge, never silently stripping the agent', () => {
  const got = composeAllowedTools(['Read'], ['Bash'], 'not-a-real-mode');
  assert.deepStrictEqual(got, ['Read', 'Bash']);
});

test('parseFrontmatter reads a flow-style tools list', () => {
  const got = parseFrontmatter('---\ntools: [Read, "Edit"]\n---\nbody text');
  assert.deepStrictEqual(got.tools, ['Read', 'Edit']);
  assert.strictEqual(got.body, 'body text');
});

test('parseFrontmatter reads a block-style tools list', () => {
  const got = parseFrontmatter('---\ntools:\n  - Read\n  - Bash\n---\nbody');
  assert.deepStrictEqual(got.tools, ['Read', 'Bash']);
});
