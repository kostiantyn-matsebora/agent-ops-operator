'use strict';

// The run's turns and tool calls, read from pi's own AgentSession events, for
// the work result's `turns` and `toolCalls` (design.md's D6, spike S5).
//
// The manager never sees a model call or a tool call — this process does — so
// the report is the only way either reaches the activity graph. FACTS, never
// content: a tool's name and how long it took, never its arguments or result
// (`RT-3`).
//
// A fact pi does not report is OMITTED rather than written as zero, because
// zero tokens is a real, different value (`API-7`) — true of this function's
// own logic (`count()` below), but narrower once driven against the real
// openai-completions provider path: confirmed live (conformance lane,
// `runtimePiAssertAPI7OmitsWhatPiCannotDetermine`), `ModelRuntime`'s usage
// accumulator default-fills `input`/`output`/`cacheRead`/`cacheWrite` to 0
// when the wire sends no `usage` object at all, so this module cannot tell
// a real zero from a gap pi papered over for those four. `reasoning` is the
// one sub-fact that stays genuinely absent in that same case, and omission
// is still exact for it.
//
// Shaped like runtimes/copilot/report.js's `newCallRecorder` (task 4.1's own
// instruction) — same bounds, same drop-undefined-at-the-end structure — over
// pi's own event names and fields, confirmed live against SDK 1.5.2:
//   turn_end                  -> one turns[] entry, from message.usage/.stopReason
//   tool_execution_start/_end -> one toolCalls[] entry, keyed by toolCallId
//
// pi's own `tool_execution_end.durationMs` is already the SDK's own
// monotonic measurement (confirmed in `pi-agent-core/dist/types.d.ts`), so —
// unlike copilot's recorder, which times a call itself from two event
// timestamps — this one only ever reads the figure pi already computed, and
// reports nothing when pi reports nothing ("absent when the tool did not
// run", the end event's own doc comment).

const { BUILTIN, parseMcpPattern } = require('./vocabulary');

/** The manager's bounds (docs/contracts/runtime.md#bounds). Over them it refuses the whole report. */
const MAX_TURNS = 200;
const MAX_TOOL_CALLS = 200;
const MAX_FIELD = 200;

/** Pi's own built-in names, back in the agent-ops vocabulary. */
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

/** totalIn is every input token a call consumed, cache parts included. */
function totalIn(u) {
  const input = count(u.input);
  if (input === undefined) return undefined;
  return input + (count(u.cacheRead) ?? 0) + (count(u.cacheWrite) ?? 0);
}

/** resultBytes measures what a tool returned, without keeping it. */
function resultBytes(result) {
  if (result == null) return 0;
  const content = result.content !== undefined ? result.content : result;
  if (typeof content === 'string') return Buffer.byteLength(content);
  if (Array.isArray(content)) {
    return content.reduce((n, b) => n + (typeof b?.text === 'string' ? Buffer.byteLength(b.text) : Buffer.byteLength(JSON.stringify(b ?? null))), 0);
  }
  return Buffer.byteLength(JSON.stringify(content));
}

/**
 * toolIdentity names a call in the agent-ops vocabulary, with its server.
 * An MCP tool's identity is its own registered name, `mcp__<server>__<tool>`
 * (design D3) — recovered here by parsing that name, no separate field. A
 * built-in's pi-native name (`read`, `bash`, ...) is mapped back through
 * `vocabulary.js`'s own table, matching `ToolCall.Tool`'s contract ("the
 * tool's name in the agent-ops vocabulary"). A name neither shape recognises
 * is reported as pi gave it, same fallback copilot's recorder takes.
 */
function toolIdentity(toolName) {
  const mcp = parseMcpPattern(String(toolName || ''));
  if (mcp) return { tool: toolName, server: mcp.server };
  return { tool: FROM_BUILTIN[toolName] || toolName };
}

/**
 * newCallRecorder consumes AgentSession events and reports the run's calls.
 */
function newCallRecorder() {
  const turns = [];
  const calls = [];
  const pending = new Map();

  return {
    note(ev) {
      if (!ev) return;
      switch (ev.type) {
        case 'turn_end': {
          const msg = ev.message;
          if (!msg) return;
          const u = msg.usage || {};
          turns.push({
            model: field(msg.model),
            // The contract's `tokensIn` INCLUDES the cache parts (runtime.md);
            // pi reports `input` as the uncached remainder, so the total is
            // rebuilt here, as runtimes/claude does. Left as pi gave it, the
            // console's derived "fresh" figure went negative.
            tokensIn: totalIn(u),
            tokensOut: count(u.output),
            cacheReadTokens: count(u.cacheRead),
            // Reported under the CONTRACT's existing field for this exact
            // fact (platform/manager/internal/httpapi/workreport.go's
            // `Turn.CacheCreationTokens`) rather than the new
            // `cacheWriteTokens` name design.md's S5 finding proposed —
            // `runtimes/claude`/`runtimes/claude-cli` already report the
            // identical quantity (tokens WRITTEN into the provider's prompt
            // cache) under this name. Flagged in this change's hand-back:
            // minting a second name for one fact the manager already has a
            // field for would make it a dead key, invisible to the activity
            // graph and the metrics mapping (`cacheReadTokens` there too)
            // that already key on this one.
            cacheCreationTokens: count(u.cacheWrite),
            // `reasoning` has NO existing contract field (unlike cacheWrite
            // above) — genuinely new, so a name is minted rather than reused.
            // Present only when the provider reports a thinking-token
            // breakdown (`Usage.reasoning`'s own doc comment); omitted,
            // never zero, otherwise. A manager that does not know this field
            // ignores it (`COMPAT-2`) until a later change gives it one.
            reasoningTokens: count(u.reasoning),
            stopReason: field(msg.stopReason),
          });
          return;
        }
        case 'tool_execution_start': {
          if (!ev.toolName) return;
          const id = toolIdentity(ev.toolName);
          const call = { tool: field(id.tool), server: field(id.server) };
          calls.push(call);
          if (ev.toolCallId) pending.set(ev.toolCallId, call);
          return;
        }
        case 'tool_execution_end': {
          const call = pending.get(ev.toolCallId);
          if (!call) return;
          pending.delete(ev.toolCallId);
          const d = count(ev.durationMs);
          if (d !== undefined) call.durationMs = d;
          call.resultBytes = resultBytes(ev.result);
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

module.exports = { newCallRecorder, totalIn, resultBytes, toolIdentity, MAX_TURNS, MAX_TOOL_CALLS, MAX_FIELD };
