'use strict';

// The run's turns and tool calls, read from the stream-json claude-code
// already prints, for the work result's `turns` and `toolCalls`.
//
// The manager never sees a model call or a tool call — this process does — so
// the report is the only way either reaches the activity graph. It carries
// FACTS, never content: a tool's name and how long it took, never the command
// it ran or what it returned.
//
// A fact the stream does not carry is OMITTED rather than written as zero,
// because zero tokens is a real, different value.

/** The manager's bounds. Over them it refuses the whole report. */
const MAX_TURNS = 200;
const MAX_TOOL_CALLS = 200;
const MAX_FIELD = 200;

/** A synthetic message is claude-code's own, not a model call. */
const SYNTHETIC_MODEL = '<synthetic>';

function field(s) {
  if (typeof s !== 'string' || !s) return undefined;
  return Buffer.byteLength(s) <= MAX_FIELD ? s : Buffer.from(s).subarray(0, MAX_FIELD).toString().replace(/�+$/, '');
}

function count(n) {
  return Number.isFinite(n) && n >= 0 ? n : undefined;
}

/** mcpServer names the server of an `mcp__<server>__<tool>` name, or ''. */
function mcpServer(tool) {
  if (typeof tool !== 'string' || !tool.startsWith('mcp__')) return '';
  const rest = tool.slice('mcp__'.length);
  const at = rest.indexOf('__');
  return at > 0 ? rest.slice(0, at) : '';
}

/** resultBytes measures what a tool returned, without keeping it. */
function resultBytes(content) {
  if (content == null) return 0;
  if (typeof content === 'string') return Buffer.byteLength(content);
  if (Array.isArray(content)) {
    return content.reduce((n, b) => n + (typeof b?.text === 'string' ? Buffer.byteLength(b.text) : Buffer.byteLength(JSON.stringify(b ?? null))), 0);
  }
  return Buffer.byteLength(JSON.stringify(content));
}

function drop(obj) {
  for (const k of Object.keys(obj)) if (obj[k] === undefined) delete obj[k];
  return obj;
}

/**
 * newCallRecorder consumes stream-json events in order and reports the run's
 * turns and tool calls. `now` is injectable so a captured stream measures
 * deterministic durations.
 *
 * claude-code prints one `assistant` event per content block, each repeating
 * the message's id and its usage so far, so a turn is keyed on the message id
 * and keeps the last usage it saw.
 */
function newCallRecorder(now = Date.now) {
  const turns = [];
  const turnById = new Map();
  const calls = [];
  const pending = new Map();

  function noteAssistant(msg) {
    if (!msg || msg.model === SYNTHETIC_MODEL) return;
    let turn = msg.id ? turnById.get(msg.id) : undefined;
    if (!turn) {
      turn = {};
      turns.push(turn);
      if (msg.id) turnById.set(msg.id, turn);
    }
    if (msg.model) turn.model = field(msg.model);
    const u = msg.usage;
    if (u) {
      const input = count(u.input_tokens);
      const created = count(u.cache_creation_input_tokens) ?? 0;
      const read = count(u.cache_read_input_tokens);
      if (input !== undefined) turn.tokensIn = input + created + (read ?? 0);
      if (count(u.output_tokens) !== undefined) turn.tokensOut = u.output_tokens;
      if (read !== undefined) turn.cacheReadTokens = read;
    }
    if (msg.stop_reason) turn.stopReason = field(msg.stop_reason);
    for (const b of msg.content || []) {
      if (b?.type !== 'tool_use' || !b.name) continue;
      const call = { tool: field(b.name), server: field(mcpServer(b.name)) };
      calls.push(call);
      if (b.id) pending.set(b.id, { call, started: now() });
    }
  }

  function noteResults(msg) {
    for (const b of msg?.content || []) {
      if (b?.type !== 'tool_result') continue;
      const open = pending.get(b.tool_use_id);
      if (!open) continue;
      pending.delete(b.tool_use_id);
      open.call.durationMs = Math.max(0, now() - open.started);
      open.call.resultBytes = resultBytes(b.content);
    }
  }

  return {
    note(ev) {
      if (ev?.type === 'assistant') noteAssistant(ev.message);
      else if (ev?.type === 'user') noteResults(ev.message);
    },
    /** report returns the fields to merge into the work result. */
    report() {
      const out = {};
      if (turns.length) out.turns = turns.slice(0, MAX_TURNS).map((t) => drop({ ...t }));
      if (calls.length) out.toolCalls = calls.slice(0, MAX_TOOL_CALLS).map((c) => drop({ ...c }));
      return out;
    },
  };
}

module.exports = { newCallRecorder, mcpServer, resultBytes, MAX_TURNS, MAX_TOOL_CALLS, MAX_FIELD };
