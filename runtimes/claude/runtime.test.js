// cd runtime-claude && node --test
//
// runtime.js self-executes its long-poll loop as a bottom-of-file IIFE and
// exits the process when CONTROL_URL/CONVO_ID are unset — both guarded on
// `require.main === module` (see runtime.js) so this suite can `require()`
// the module for its pure/subprocess-driving pieces without either firing.
// Every env var runtime.js reads is captured into a module-level const at
// require time, so each distinct environment this suite needs gets its own
// test FILE (node's test runner gives every file its own process).
'use strict';

const test = require('node:test');
const assert = require('node:assert');
const fs = require('fs');
const os = require('os');
const path = require('path');

function tmp(prefix) {
  return fs.mkdtempSync(path.join(os.tmpdir(), prefix));
}

process.env.CONTROL_URL = 'http://127.0.0.1:1';
process.env.CONVO_ID = 'conv-test';
process.env.REPO_URL = ''; // exercises syncRepo's early no-op branch
const HOME = tmp('agentops-home-');
process.env.HOME = HOME; // controls SESSIONS_DIR

const {
  gitEnv, repoURL, run, clearDir, syncRepo, formatEvent,
  confirmContextMissing, sessionFileExists, contextIdOf, strip, SESSIONS_DIR,
} = require('./runtime');

// ---- gitEnv / repoURL, default (no auth configured) ---------------------------

test('gitEnv with no GIT_AUTH_TYPE adds no SSH command, and passes process.env through', () => {
  const env = gitEnv();
  assert.strictEqual(env.GIT_SSH_COMMAND, undefined);
  assert.strictEqual(env.PATH, process.env.PATH);
});

test('repoURL with no https auth type returns the configured URL unchanged', () => {
  // REPO_URL is '' in this process — the branch under test is the `&&` chain
  // being false because GIT_AUTH_TYPE !== 'https', not because REPO_URL is empty.
  assert.strictEqual(repoURL(), '');
});

// ---- syncRepo: no REPO_URL configured -----------------------------------------

test('syncRepo is a no-op when no REPO_URL is configured', async () => {
  // Must not throw and must not touch the filesystem.
  await assert.doesNotReject(syncRepo());
});

// ---- run(): a real subprocess, both success and failure -----------------------

test('run() captures stdout and a zero exit code from a real subprocess', async () => {
  const r = await run(process.execPath, ['-e', "process.stdout.write('run-ok')"]);
  assert.strictEqual(r.code, 0);
  assert.strictEqual(r.out, 'run-ok');
});

test('run() captures stderr and a non-zero exit code', async () => {
  const r = await run(process.execPath, ['-e', "process.stderr.write('run-err'); process.exit(3)"]);
  assert.strictEqual(r.code, 3);
  assert.strictEqual(r.err, 'run-err');
});

test('run() resolves rather than throwing when the binary does not exist', async () => {
  const r = await run('agentops-definitely-not-a-real-binary', []);
  assert.strictEqual(r.code, -1);
  assert.ok(r.err.length > 0, 'the spawn error message is surfaced');
});

// ---- clearDir -------------------------------------------------------------------

test('clearDir empties a directory recursively without removing the directory itself', () => {
  const dir = tmp('agentops-cleardir-');
  fs.writeFileSync(path.join(dir, 'a.txt'), 'x');
  fs.mkdirSync(path.join(dir, 'sub'));
  fs.writeFileSync(path.join(dir, 'sub', 'b.txt'), 'y');
  clearDir(dir);
  assert.deepStrictEqual(fs.readdirSync(dir), []);
  assert.ok(fs.existsSync(dir), 'the mount point itself must survive');
});

// ---- formatEvent ------------------------------------------------------------------

test('formatEvent renders an init event with model, tool count and mcp status', () => {
  const line = formatEvent({
    type: 'system', subtype: 'init', model: 'claude-x',
    tools: ['Read', 'Bash'], mcp_servers: [{ name: 'k8s', status: 'connected' }],
  });
  assert.match(line, /\[init\] model=claude-x tools=2 mcp=k8s:connected/);
});

test('formatEvent renders assistant text blocks', () => {
  const line = formatEvent({ type: 'assistant', message: { content: [{ type: 'text', text: 'hi there' }] } });
  assert.strictEqual(line, '[claude] hi there\n');
});

test('formatEvent renders a tool_use block, truncating long input at 160 chars', () => {
  const bigInput = { path: 'x'.repeat(300) };
  const line = formatEvent({ type: 'assistant', message: { content: [{ type: 'tool_use', name: 'Read', input: bigInput }] } });
  assert.match(line, /^\[tool\] Read /);
  assert.match(line, /…\n$/, 'a long argument blob is truncated with an ellipsis');
});

test('formatEvent ignores a content block of an unrecognised type', () => {
  const line = formatEvent({ type: 'assistant', message: { content: [{ type: 'thinking', text: 'internal' }] } });
  assert.strictEqual(line, '');
});

test('formatEvent handles an assistant event with no content at all', () => {
  assert.strictEqual(formatEvent({ type: 'assistant' }), '');
});

test('formatEvent renders a result event with subtype, turns and duration', () => {
  const line = formatEvent({ type: 'result', subtype: 'success', num_turns: 3, duration_ms: 4500, result: 'the answer' });
  assert.match(line, /=== RESULT \(success, 3 turns, 5s\) ===/);
  assert.match(line, /the answer/);
});

test('formatEvent returns empty string for an event type it does not know', () => {
  assert.strictEqual(formatEvent({ type: 'stream_event' }), '');
});

test('formatEvent falls back to the raw line when the event throws while being read', () => {
  // A null event makes `ev.type` throw — this is the one path that reaches
  // the catch block, and it must reproduce the line rather than crash the
  // stdout parser for one bad line.
  assert.strictEqual(formatEvent(null, 'the raw text'), 'the raw text\n');
});

// ---- contextIdOf / strip -----------------------------------------------------------

test('contextIdOf prefers runtimeContextId over the retired resumeSessionId', () => {
  assert.strictEqual(contextIdOf({ runtimeContextId: 'new-id', resumeSessionId: 'old-id' }), 'new-id');
});

test('contextIdOf falls back to resumeSessionId for one release', () => {
  assert.strictEqual(contextIdOf({ resumeSessionId: 'old-id' }), 'old-id');
});

test('contextIdOf is empty when the work unit carries neither', () => {
  assert.strictEqual(contextIdOf({}), '');
});

test('strip removes only the internal stderr capture', () => {
  const out = strip({ status: 'succeeded', result: 'ok', stderr: 'internal noise' });
  assert.deepStrictEqual(out, { status: 'succeeded', result: 'ok' });
  assert.ok(!('stderr' in out));
});

// ---- sessionFileExists: real filesystem, recursive ---------------------------------

test('sessionFileExists finds a match nested under a subdirectory', async () => {
  const dir = tmp('agentops-sessions-');
  fs.mkdirSync(path.join(dir, 'proj1'), { recursive: true });
  fs.writeFileSync(path.join(dir, 'proj1', 'sess-abc123.jsonl'), '{}');
  assert.strictEqual(await sessionFileExists(dir, 'abc123'), true);
});

test('sessionFileExists returns false when nothing matches', async () => {
  const dir = tmp('agentops-sessions-');
  fs.mkdirSync(path.join(dir, 'proj1'), { recursive: true });
  fs.writeFileSync(path.join(dir, 'proj1', 'sess-zzz.jsonl'), '{}');
  assert.strictEqual(await sessionFileExists(dir, 'not-there'), false);
});

test('sessionFileExists returns false rather than throwing for a directory that does not exist', async () => {
  assert.strictEqual(await sessionFileExists(path.join(os.tmpdir(), 'agentops-never-created'), 'x'), false);
});

// ---- confirmContextMissing: real timers, real filesystem ---------------------------

test('confirmContextMissing returns true immediately for an empty context id', async () => {
  const start = Date.now();
  assert.strictEqual(await confirmContextMissing(''), true);
  assert.ok(Date.now() - start < 100, 'no ladder is walked for an id that was never set');
});

test('confirmContextMissing finds a reappearing file on its first check', async () => {
  const projDir = path.join(SESSIONS_DIR, 'proj-reappear');
  fs.mkdirSync(projDir, { recursive: true });
  fs.writeFileSync(path.join(projDir, 'sess-here-now.jsonl'), '{}');
  const start = Date.now();
  const missing = await confirmContextMissing('here-now');
  assert.strictEqual(missing, false, 'the file is present, so it is not missing');
  assert.ok(Date.now() - start < 3000, 'found on the first check, so the full ladder is not walked');
});

test('confirmContextMissing walks the full ladder and confirms loss when nothing ever appears', async () => {
  const start = Date.now();
  const missing = await confirmContextMissing('genuinely-nowhere');
  assert.strictEqual(missing, true);
  assert.ok(Date.now() - start >= 5000, 'all three waits (500+1500+3000ms) must be spent before concluding loss');
});
