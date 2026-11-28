// cd runtime-claude && node --test
//
// multi-repo-workspace: a work unit's `agentFile` (an absolute path) is
// exclusive with `agent` and, when present, is what this runtime reads for
// the agent's declared tools — never `agent` resolved under WORKSPACE.
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
process.env.CONVO_ID = 'conv-agentfile';
const WORKSPACE = tmp('agentops-agentfile-ws-');
process.env.WORKSPACE = WORKSPACE;

const {
  executeUnit, __setQueryForTest, __resetLiveSessionForTest,
} = require('./runtime');

function resultEvent(sessionId, text) {
  return { type: 'result', subtype: 'success', num_turns: 1, duration_ms: 1, result: text, is_error: false, session_id: sessionId };
}

function fakeQueryRecordingOptions() {
  const calls = [];
  function fakeQuery({ options } = {}) {
    calls.push(options);
    const events = [resultEvent('sess-1', 'ok')];
    return {
      async next() { return events.length ? { value: events.shift(), done: false } : { value: undefined, done: true }; },
      async close() {},
      async interrupt() {},
    };
  }
  fakeQuery.calls = calls;
  return fakeQuery;
}

test('a unit carrying agentFile reads tools from that absolute path, ignoring an agent name that names a different declaration', async (t) => {
  __resetLiveSessionForTest();
  t.after(() => __resetLiveSessionForTest());

  // .claude/agents/<agent>.md under WORKSPACE declares a DIFFERENT tool —
  // if this were ever read instead of agentFile, the offered built-in set
  // below would be ['Bash'], not ['Read'].
  const agentsDir = path.join(WORKSPACE, '.claude', 'agents');
  fs.mkdirSync(agentsDir, { recursive: true });
  fs.writeFileSync(path.join(agentsDir, 'k8s-ops.md'), '---\ntools: Bash\n---\n');

  const agentFileDir = tmp('agentops-agentfile-def-');
  const agentFile = path.join(agentFileDir, 'other-repo-agent.md');
  fs.writeFileSync(agentFile, '---\ntools: Read\n---\n');

  const fakeQuery = fakeQueryRecordingOptions();
  __setQueryForTest(fakeQuery);

  const out = await executeUnit({ runId: 'r1', promptText: 'hi', agent: 'k8s-ops', agentFile });
  assert.strictEqual(out.status, 'succeeded');
  assert.deepStrictEqual(fakeQuery.calls[0].tools, ['Read'], 'the agentFile declaration, not the agent name under WORKSPACE, must drive the offered built-ins');
});

test('a unit naming no agentFile falls back to resolving agent under WORKSPACE as before', async (t) => {
  __resetLiveSessionForTest();
  t.after(() => __resetLiveSessionForTest());

  const agentsDir = path.join(WORKSPACE, '.claude', 'agents');
  fs.mkdirSync(agentsDir, { recursive: true });
  fs.writeFileSync(path.join(agentsDir, 'plain-agent.md'), '---\ntools: Grep\n---\n');

  const fakeQuery = fakeQueryRecordingOptions();
  __setQueryForTest(fakeQuery);

  const out = await executeUnit({ runId: 'r2', promptText: 'hi', agent: 'plain-agent' });
  assert.strictEqual(out.status, 'succeeded');
  assert.deepStrictEqual(fakeQuery.calls[0].tools, ['Grep']);
});
