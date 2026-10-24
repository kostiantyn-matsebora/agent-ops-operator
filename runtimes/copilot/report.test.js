'use strict';

const test = require('node:test');
const assert = require('node:assert');
const fs = require('fs');
const path = require('path');

const { newCallRecorder, toolIdentity, extractBrief, MAX_TURNS, MAX_TOOL_CALLS, MAX_FIELD, MAX_BRIEF } = require('./report');

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

// ---- extractBrief: the `<brief>` contract field (P1) ------------------------

test('extractBrief pulls a standalone brief tag out and removes it from the text', () => {
  const { text, brief } = extractBrief('the disk is fine\n\n<brief>\nthe api pod restart loop\n</brief>\n\nnothing else to do');
  assert.strictEqual(brief, 'the api pod restart loop');
  assert.ok(!text.includes('<brief>'));
  assert.ok(!text.includes('the api pod restart loop'), 'the brief text must not survive duplicated in result');
});

test('extractBrief joins a multi-line brief into one line', () => {
  const { brief } = extractBrief('<brief>\nthe api pod,\nrestarting every few minutes\n</brief>');
  assert.strictEqual(brief, 'the api pod, restarting every few minutes');
});

test('extractBrief reports empty when no tag is present', () => {
  const { text, brief } = extractBrief('just an ordinary answer');
  assert.strictEqual(brief, '');
  assert.strictEqual(text, 'just an ordinary answer');
});

test('extractBrief ignores a brief-shaped mention that is not a standalone tag', () => {
  const { text, brief } = extractBrief('say `<brief>` on its own line to set one');
  assert.strictEqual(brief, '');
  assert.strictEqual(text, 'say `<brief>` on its own line to set one');
});

test('extractBrief bounds the reported brief to MAX_BRIEF', () => {
  const long = 'x'.repeat(MAX_BRIEF + 50);
  const { brief } = extractBrief(`<brief>\n${long}\n</brief>`);
  assert.strictEqual(brief.length, MAX_BRIEF);
});

test('toolIdentity maps back to the agent-ops vocabulary', () => {
  assert.deepStrictEqual(toolIdentity({ toolName: 'view' }), { tool: 'Read' });
  assert.deepStrictEqual(toolIdentity({ toolName: 'report_intent' }), { tool: 'report_intent' });
  assert.deepStrictEqual(toolIdentity({ toolName: 'ha-GetLiveContext', mcpServerName: 'ha', mcpToolName: 'GetLiveContext' }),
    { tool: 'mcp__ha__GetLiveContext', server: 'ha' });
});
