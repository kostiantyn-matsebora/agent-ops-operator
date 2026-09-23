'use strict';

const test = require('node:test');
const assert = require('node:assert');
const fs = require('fs');
const path = require('path');

const { newCallRecorder, mcpServer, resultBytes, MAX_TURNS, MAX_TOOL_CALLS, MAX_FIELD } = require('./report');

const FIXTURE = path.join(__dirname, '..', '..', 'test', 'fixtures', 'claude-stream-json.jsonl');

function replay(lines, step = 100) {
  let t = 0;
  const rec = newCallRecorder(() => t);
  for (const line of lines) {
    t += step;
    rec.note(JSON.parse(line));
  }
  return rec.report();
}

test('the captured stream produces the expected report', () => {
  const lines = fs.readFileSync(FIXTURE, 'utf8').split('\n').filter(Boolean);
  const report = replay(lines);
  assert.deepStrictEqual(report, {
    turns: [
      { model: 'claude-sonnet-4-5', tokensIn: 9312, tokensOut: 64, cacheReadTokens: 9000, stopReason: 'tool_use' },
      { model: 'claude-sonnet-4-5', tokensIn: 9340, tokensOut: 31, cacheReadTokens: 9300, stopReason: 'tool_use' },
      { model: 'claude-sonnet-4-5', tokensIn: 9455, tokensOut: 14, cacheReadTokens: 9400, stopReason: 'end_turn' },
    ],
    toolCalls: [
      { tool: 'mcp__kubernetes__pods_list', server: 'kubernetes', durationMs: 100, resultBytes: 49 },
      { tool: 'Bash', durationMs: 100, resultBytes: 43 },
    ],
  });
});

test('no tool input or output reaches the report', () => {
  const lines = fs.readFileSync(FIXTURE, 'utf8').split('\n').filter(Boolean);
  const text = JSON.stringify(replay(lines));
  assert.ok(!text.includes('kubectl'), 'the command a tool ran is content');
  assert.ok(!text.includes('agentops-manager-0'), 'what a tool returned is content');
});

test('an unknown fact is omitted, never zero', () => {
  const report = replay([JSON.stringify({ type: 'assistant', message: { id: 'm', model: 'x', content: [] } })]);
  assert.deepStrictEqual(report.turns, [{ model: 'x' }]);
});

test('a synthetic message is not a model call, and a silent stream reports nothing', () => {
  assert.deepStrictEqual(replay([JSON.stringify({ type: 'assistant', message: { model: '<synthetic>', content: [] } })]), {});
  assert.deepStrictEqual(replay([]), {});
});

test('the report is trimmed to the manager bounds before it is posted', () => {
  const lines = [];
  for (let i = 0; i < MAX_TURNS + 5; i++) {
    lines.push(JSON.stringify({ type: 'assistant', message: { id: `m${i}`, model: 'x'.repeat(MAX_FIELD + 10),
      content: [{ type: 'tool_use', id: `t${i}`, name: 'Read', input: {} }] } }));
  }
  const report = replay(lines);
  assert.strictEqual(report.turns.length, MAX_TURNS);
  assert.strictEqual(report.toolCalls.length, MAX_TOOL_CALLS);
  assert.ok(Buffer.byteLength(report.turns[0].model) <= MAX_FIELD);
  assert.ok(!('durationMs' in report.toolCalls[0]), 'a call with no result has no duration');
});

test('mcpServer and resultBytes', () => {
  assert.strictEqual(mcpServer('mcp__home-assistant__GetLiveContext'), 'home-assistant');
  assert.strictEqual(mcpServer('Read'), '');
  assert.strictEqual(mcpServer('mcp__broken'), '');
  assert.strictEqual(resultBytes('abc'), 3);
  assert.strictEqual(resultBytes([{ type: 'text', text: 'ab' }, { type: 'text', text: 'c' }]), 3);
  assert.strictEqual(resultBytes(null), 0);
});
