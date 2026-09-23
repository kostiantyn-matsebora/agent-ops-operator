'use strict';

// The run's turns and tool calls, read from the SDK's session events, for
// the work result's `turns` and `toolCalls`.
//
// The manager never sees a model call or a tool call — this process does — so
// the report is the only way either reaches the activity graph. FACTS, never
// content: a tool's name and how long it took, never its arguments or result.
//
// A fact the SDK does not carry is OMITTED rather than written as zero,
// because zero tokens is a real, different value.

const { BUILTIN } = require('./vocabulary');

/** The manager's bounds. Over them it refuses the whole report. */
const MAX_TURNS = 200;
const MAX_TOOL_CALLS = 200;
const MAX_FIELD = 200;

/** Copilot's built-in names, back in the agent-ops vocabulary. */
const FROM_BUILTIN = Object.freeze(Object.fromEntries(Object.entries(BUILTIN).map(([ours, theirs]) => [theirs, ours])));

function field(s) {
  if (typeof s !== 'string' || !s) return undefined;
  return Buffer.byteLength(s) <= MAX_FIELD ? s : Buffer.from(s).subarray(0, MAX_FIELD).toString().replace(/�+$/, '');
}

function count(n) {
  return Number.isFinite(n) && n >= 0 ? n : undefined;
}

function drop(obj) {
  for (const k of Object.keys(obj)) if (obj[k] === undefined) delete obj[k];
  return obj;
}

/** toolIdentity names a call in the agent-ops vocabulary, with its server. */
function toolIdentity(data) {
  if (data.mcpServerName && data.mcpToolName) {
    return { tool: `mcp__${data.mcpServerName}__${data.mcpToolName}`, server: data.mcpServerName };
  }
  return { tool: FROM_BUILTIN[data.toolName] || data.toolName };
}

/**
 * newCallRecorder consumes session events and reports the run's calls. An
 * event's own timestamp times a tool call where it has one, so a captured
 * fixture measures what the session measured; `now` is the fallback.
 */
function newCallRecorder(now = Date.now) {
  const turns = [];
  const calls = [];
  const pending = new Map();
  const at = (ev) => {
    const t = Date.parse(ev.timestamp);
    return Number.isFinite(t) ? t : now();
  };

  return {
    note(ev) {
      const d = ev && ev.data;
      if (!d) return;
      switch (ev.type) {
        case 'assistant.usage':
          turns.push({
            model: field(d.model),
            tokensIn: count(d.inputTokens),
            tokensOut: count(d.outputTokens),
            cacheReadTokens: count(d.cacheReadTokens),
            stopReason: field(d.finishReason),
          });
          return;
        case 'tool.execution_start': {
          if (!d.toolName) return;
          const id = toolIdentity(d);
          const call = { tool: field(id.tool), server: field(id.server) };
          calls.push(call);
          if (d.toolCallId) pending.set(d.toolCallId, { call, started: at(ev) });
          return;
        }
        case 'tool.execution_complete': {
          const open = pending.get(d.toolCallId);
          if (!open) return;
          pending.delete(d.toolCallId);
          open.call.durationMs = Math.max(0, at(ev) - open.started);
          if (d.result && typeof d.result.content === 'string') open.call.resultBytes = Buffer.byteLength(d.result.content);
          return;
        }
        default:
      }
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

module.exports = { newCallRecorder, toolIdentity, MAX_TURNS, MAX_TOOL_CALLS, MAX_FIELD };
