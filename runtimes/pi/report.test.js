'use strict';

const test = require('node:test');
const assert = require('node:assert');
const fs = require('fs');
const path = require('path');

const { newCallRecorder, toolIdentity, MAX_TURNS, MAX_TOOL_CALLS, MAX_FIELD } = require('./report');

const FIXTURE = path.join(__dirname, '..', '..', 'test', 'fixtures', 'pi-session-events.jsonl');

function replay(events) {
  const rec = newCallRecorder();
  for (const ev of events) rec.note(ev);
  return rec.report();
}

function fixture() {
  return fs.readFileSync(FIXTURE, 'utf8').split('\n').filter(Boolean).map((l) => JSON.parse(l));
}

// ---- the captured S5 fixture, ported from copilot's own shape -------------

test('the captured session events produce the expected report', () => {
  assert.deepStrictEqual(replay(fixture()), {
    turns: [
      // Turn one both READS and WRITES cache, and the provider reports a
      // reasoning breakdown — all three are real, present facts, not
      // omitted (API-7 is about an UNKNOWN fact, not a reported zero).
      { model: 'fake-model', tokensIn: 2400, tokensOut: 42, cacheReadTokens: 1800, cacheCreationTokens: 300, reasoningTokens: 64, stopReason: 'toolUse' },
      // Turn two's usage carries no cache or reasoning keys at all — omitted,
      // never reported as zero.
      { model: 'fake-model', tokensIn: 2600, tokensOut: 18, stopReason: 'stop' },
    ],
    toolCalls: [
      { tool: 'mcp__kubernetes__pods_list', server: 'kubernetes', durationMs: 350 },
      { tool: 'Bash', durationMs: 20 },
    ],
  });
});

test('no tool input or output reaches the report', () => {
  const text = JSON.stringify(replay(fixture()));
  assert.ok(!text.includes('kubectl'), 'the command a tool ran is content');
  assert.ok(!text.includes('agentops-manager-0'), 'what a tool returned is content');
});

test('no events at all — neither a model call nor a tool call — reports nothing (API-9)', () => {
  assert.deepStrictEqual(replay([]), {});
});

test('an event type this runtime does not expect is ignored, not recorded', () => {
  assert.deepStrictEqual(replay([{ type: 'message_start', message: { role: 'assistant', content: [] } }]), {});
});

// ---- bounds (API-4) ---------------------------------------------------------

test('the report is trimmed to the manager bounds', () => {
  const events = [];
  for (let i = 0; i < MAX_TURNS + 4; i++) {
    events.push({ type: 'turn_end', message: { model: 'm'.repeat(MAX_FIELD + 3), stopReason: 'stop' } });
    events.push({ type: 'tool_execution_start', toolCallId: `c${i}`, toolName: 'read' });
  }
  const r = replay(events);
  assert.strictEqual(r.turns.length, MAX_TURNS);
  assert.strictEqual(r.toolCalls.length, MAX_TOOL_CALLS);
  assert.ok(Buffer.byteLength(r.turns[0].model) <= MAX_FIELD);
  assert.deepStrictEqual(r.toolCalls[0], { tool: 'Read' }, 'an unfinished call reports no duration (absent, per tool_execution_end\'s own doc comment)');
});

// ---- toolIdentity: pi's own names, mapped back to the agent-ops vocabulary -

test('toolIdentity maps a pi built-in name back to the agent-ops vocabulary', () => {
  assert.deepStrictEqual(toolIdentity('read'), { tool: 'Read' });
  assert.deepStrictEqual(toolIdentity('bash'), { tool: 'Bash' });
  assert.deepStrictEqual(toolIdentity('find'), { tool: 'Glob' });
});

test('toolIdentity leaves a name neither table recognises exactly as pi gave it', () => {
  assert.deepStrictEqual(toolIdentity('ls'), { tool: 'ls' });
});

test('toolIdentity recovers an MCP tool\'s server from its own registered name, no separate field', () => {
  assert.deepStrictEqual(toolIdentity('mcp__kubernetes__pods_list'), { tool: 'mcp__kubernetes__pods_list', server: 'kubernetes' });
  assert.deepStrictEqual(toolIdentity('mcp__ha__GetLiveContext'), { tool: 'mcp__ha__GetLiveContext', server: 'ha' });
});

// ---- newCallRecorder: durationMs is pi's own measurement, read not timed --

test('tool_execution_end with no durationMs (the tool never ran) reports none', () => {
  const r = replay([
    { type: 'tool_execution_start', toolCallId: 'c1', toolName: 'read', args: { path: 'x' } },
    { type: 'tool_execution_end', toolCallId: 'c1', toolName: 'read', result: '', isError: true },
  ]);
  assert.deepStrictEqual(r.toolCalls, [{ tool: 'Read' }]);
});

test('a tool_execution_end for an id never started is ignored rather than recorded as a new call', () => {
  const r = replay([{ type: 'tool_execution_end', toolCallId: 'unknown', toolName: 'read', result: '', isError: false, durationMs: 5 }]);
  assert.deepStrictEqual(r, {});
});

test('a tool_execution_start with no toolName is never recorded', () => {
  const r = replay([{ type: 'tool_execution_start', toolCallId: 'c1' }]);
  assert.deepStrictEqual(r, {});
});
