// cd runtime-claude && node --test
'use strict';

const test = require('node:test');
const assert = require('node:assert');

const {
  DEFAULT_LIMIT, newSpinWatch, noteToolUse, spinMessage, discardedNotice, unparsedInput,
} = require('./spin');

// What is pinned here is WHEN repetition becomes a spin. The failure this
// catches produced a live incident: ten identical malformed calls to one tool,
// none of which ran, and a reply built from readings the session already held
// and presented as current.

const bad = (raw) => ({ __unparsedToolInput: { raw, len: raw.length } });
const good = { domain: 'sensor' };

// ---- reading the marker -------------------------------------------------------

test('a parsed input carries no marker', () => {
  assert.strictEqual(unparsedInput(good), null);
  assert.strictEqual(unparsedInput({}), null);
  assert.strictEqual(unparsedInput(undefined), null);
});

test('the raw text is returned, because it is what shows the mistake', () => {
  assert.strictEqual(unparsedInput(bad('{"domain": sensor}')), '{"domain": sensor}');
});

test('a marker without raw text still reads as unparsable', () => {
  const got = unparsedInput({ __unparsedToolInput: { len: 3 } });
  assert.ok(got && got.includes('len'), `want something readable, got ${got}`);
});

// ---- the judgement ------------------------------------------------------------

test('a healthy run never trips', () => {
  const w = newSpinWatch();
  for (let i = 0; i < 20; i++) {
    assert.strictEqual(noteToolUse(w, 'GetLiveContext', good), null);
  }
  assert.strictEqual(w.total, 0);
});

test('the same unparsable call, limit times over, ends the run', () => {
  const w = newSpinWatch(3);
  assert.strictEqual(noteToolUse(w, 'GetLiveContext', bad('{"domain": sensor}')), null);
  assert.strictEqual(noteToolUse(w, 'GetLiveContext', bad('{"domain": sensor}')), null);
  const verdict = noteToolUse(w, 'GetLiveContext', bad('{"domain": sensor}'));
  assert.ok(verdict, 'the third identical call is the spin');
  assert.strictEqual(verdict.name, 'GetLiveContext');
  assert.strictEqual(verdict.repeats, 3);
  assert.strictEqual(verdict.raw, '{"domain": sensor}');
});

// A model that changes its arguments is TRYING something. Ending the run there
// would kill the recovery, which is the ordinary outcome — 59 malformed calls
// in the incident window, and every run still answered.
test('varying the arguments is not a spin', () => {
  const w = newSpinWatch(3);
  assert.strictEqual(noteToolUse(w, 'GetLiveContext', bad('{"domain": sensor}')), null);
  assert.strictEqual(noteToolUse(w, 'GetLiveContext', bad('{"domain": weather}')), null);
  assert.strictEqual(noteToolUse(w, 'GetLiveContext', bad('{"domain": climate}')), null);
  assert.strictEqual(w.total, 3, 'all three still count as never having run');
});

test('a different tool with the same text is not the same call', () => {
  const w = newSpinWatch(2);
  assert.strictEqual(noteToolUse(w, 'GetLiveContext', bad('{"domain": sensor}')), null);
  assert.strictEqual(noteToolUse(w, 'HassGetState', bad('{"domain": sensor}')), null);
});

// One call that parses is recovery, and the count starts again. Otherwise a
// long conversation would accumulate its way into a false verdict.
test('a call that parses clears the streak', () => {
  const w = newSpinWatch(3);
  noteToolUse(w, 'GetLiveContext', bad('{"domain": sensor}'));
  noteToolUse(w, 'GetLiveContext', bad('{"domain": sensor}'));
  assert.strictEqual(noteToolUse(w, 'GetLiveContext', good), null);
  assert.strictEqual(noteToolUse(w, 'GetLiveContext', bad('{"domain": sensor}')), null);
  assert.strictEqual(w.repeats, 1);
});

test('a zero limit disables the breaker without disabling the count', () => {
  const w = newSpinWatch(0);
  for (let i = 0; i < 50; i++) {
    assert.strictEqual(noteToolUse(w, 'GetLiveContext', bad('{"domain": sensor}')), null);
  }
  assert.strictEqual(w.total, 50, 'the run is still known to have executed nothing');
});

test('the default limit is above a transient and below the observed spin', () => {
  assert.ok(DEFAULT_LIMIT >= 3 && DEFAULT_LIMIT <= 10, `unreasonable default: ${DEFAULT_LIMIT}`);
});

// ---- what the person reads ----------------------------------------------------

test('the message says nothing ran, and shows what was written', () => {
  const msg = spinMessage({ name: 'mcp__homeassistant__GetLiveContext', raw: '{"domain": sensor}', repeats: 5, total: 5 });
  assert.match(msg, /^❌ \*\*Stopped/, 'it leads with the status line the surfaces expect');
  assert.match(msg, /none of them ran/);
  assert.match(msg, /mcp__homeassistant__GetLiveContext/);
  assert.match(msg, /\{"domain": sensor\}/);
  assert.ok(msg.length < 3800, 'within the message budget every surface holds');
});

test('a huge argument blob does not become the message', () => {
  const msg = spinMessage({ name: 'x', raw: 'y'.repeat(10_000), repeats: 5, total: 5 });
  assert.ok(msg.length < 1000, `message ran away: ${msg.length} chars`);
});

// ---- what a SUCCESSFUL run still owes -----------------------------------------

// Twice out of twice observed, a model that could not form a call gave up on
// the tool and answered from what the session already held — and the run was
// reported a success. The answer does not say that. This does.

test('a clean run adds nothing', () => {
  const w = newSpinWatch();
  noteToolUse(w, 'GetLiveContext', good);
  assert.strictEqual(discardedNotice(w), null);
});

test('a run with discarded calls says how many, and which tool', () => {
  const w = newSpinWatch();
  noteToolUse(w, 'mcp__homeassistant__GetLiveContext', bad('{"domain": sensor}'));
  noteToolUse(w, 'mcp__homeassistant__GetLiveContext', bad('{"domain": weather}'));
  const notice = discardedNotice(w);
  assert.match(notice, /2 tool calls never ran/);
  assert.match(notice, /mcp__homeassistant__GetLiveContext/);
  assert.match(notice, /missing from this answer/);
});

test('one call reads as one', () => {
  const w = newSpinWatch();
  noteToolUse(w, 'GetLiveContext', bad('{"domain": sensor}'));
  assert.match(discardedNotice(w), /1 tool call never ran/);
});

test('many tools are named without listing them all', () => {
  const w = newSpinWatch();
  for (const name of ['a', 'b', 'c', 'd']) noteToolUse(w, name, bad('{'));
  const notice = discardedNotice(w);
  assert.match(notice, /and others/);
  assert.ok(notice.length < 300, `notice ran away: ${notice.length} chars`);
});
