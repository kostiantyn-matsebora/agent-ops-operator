'use strict';

const test = require('node:test');
const assert = require('node:assert');
const fs = require('fs');
const path = require('path');

const { newCallRecorder, toolIdentity, MAX_TURNS, MAX_TOOL_CALLS, MAX_FIELD } = require('./report');

const FIXTURE = path.join(__dirname, '..', '..', 'test', 'fixtures', 'copilot-session-events.jsonl');

function replay(events) {
  const rec = newCallRecorder(() => 0);
  for (const ev of events) rec.note(ev);
  return rec.report();
}

function fixture() {
  return fs.readFileSync(FIXTURE, 'utf8').split('\n').filter(Boolean).map((l) => JSON.parse(l));
}

test('the captured session events produce the expected report', () => {
  assert.deepStrictEqual(replay(fixture()), {
    turns: [
      { model: 'gpt-5', tokensIn: 2400, tokensOut: 42, cacheReadTokens: 1800, stopReason: 'tool_calls' },
      { model: 'gpt-5', tokensIn: 2600, tokensOut: 18, stopReason: 'stop' },
    ],
    toolCalls: [
      { tool: 'mcp__kubernetes__pods_list', server: 'kubernetes', durationMs: 350, resultBytes: 49 },
      { tool: 'Bash', durationMs: 20, resultBytes: 43 },
    ],
  });
});

test('no tool input or output reaches the report', () => {
  const text = JSON.stringify(replay(fixture()));
  assert.ok(!text.includes('kubectl'), 'the command a tool ran is content');
  assert.ok(!text.includes('agentops-manager-0'), 'what a tool returned is content');
});

test('a session with no usage and no tools reports nothing', () => {
  assert.deepStrictEqual(replay([{ type: 'assistant.message', data: { content: 'hi' } }]), {});
});

test('the report is trimmed to the manager bounds', () => {
  const events = [];
  for (let i = 0; i < MAX_TURNS + 4; i++) {
    events.push({ type: 'assistant.usage', data: { model: 'm'.repeat(MAX_FIELD + 3) } });
    events.push({ type: 'tool.execution_start', data: { toolName: 'view', toolCallId: `c${i}` } });
  }
  const r = replay(events);
  assert.strictEqual(r.turns.length, MAX_TURNS);
  assert.strictEqual(r.toolCalls.length, MAX_TOOL_CALLS);
  assert.ok(Buffer.byteLength(r.turns[0].model) <= MAX_FIELD);
  assert.deepStrictEqual(r.toolCalls[0], { tool: 'Read' }, 'an unfinished call reports no duration and no size');
});

test('toolIdentity maps back to the agent-ops vocabulary', () => {
  assert.deepStrictEqual(toolIdentity({ toolName: 'view' }), { tool: 'Read' });
  assert.deepStrictEqual(toolIdentity({ toolName: 'report_intent' }), { tool: 'report_intent' });
  assert.deepStrictEqual(toolIdentity({ toolName: 'ha-GetLiveContext', mcpServerName: 'ha', mcpToolName: 'GetLiveContext' }),
    { tool: 'mcp__ha__GetLiveContext', server: 'ha' });
});
