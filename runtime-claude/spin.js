'use strict';

// A tool call the model cannot FORM.
//
// claude-code parses the arguments a model writes for a tool before it calls
// anything. When that text is not valid JSON it hands the block through with an
// `__unparsedToolInput` marker instead, and NOTHING IS EXECUTED — no MCP
// server sees it, no allowlist refuses it, no network carries it. From outside
// it looks like a tool that answers nothing.
//
// A model that writes one such call usually writes it again, identically: the
// feedback it gets does not say which character was wrong. On 2026-08-22 one
// conversation spent ten turns and ninety seconds on the same malformed call,
// then answered from readings it already had and presented them as current.
// Across three days, 59 of 110 calls to one tool never ran.
//
// The cause was upstream of us and is worth naming, because it decides what a
// fix looks like: Home Assistant advertises `GetLiveContext`'s `domain` filter
// with an `anyOf` whose first branch is an EMPTY schema, so that parameter has
// no declared type. Every neighbouring parameter that says `string` was quoted
// correctly in the same call. A model told nothing writes a bare word.
//
// This module decides when repetition has become a spin. It reads events and
// judges — ending the run belongs to the caller.

/** How many identical unparsable calls in a row end the run. */
const DEFAULT_LIMIT = 5;

/**
 * unparsedInput returns the text claude-code could not parse, or null.
 *
 * The raw text is what makes the report usable: "arguments were not valid
 * JSON" sends somebody to the logs, `{"domain": sensor}` shows them the missing
 * quotes.
 */
function unparsedInput(input) {
  const marker = input && input.__unparsedToolInput;
  if (!marker) return null;
  if (typeof marker.raw === 'string') return marker.raw;
  try {
    return JSON.stringify(marker);
  } catch {
    return '(unreadable)';
  }
}

/** newSpinWatch tracks one invocation's tool calls. */
function newSpinWatch(limit = DEFAULT_LIMIT) {
  return { limit, key: null, repeats: 0, total: 0, names: new Set() };
}

/**
 * noteToolUse records one tool_use block.
 *
 * Returns null while the run is healthy, and a verdict once the SAME tool has
 * been called with the SAME unparsable arguments `limit` times in a row.
 *
 * CONSECUTIVE and IDENTICAL, both deliberately. A model that varies its
 * arguments is trying something, and a model whose next call parses has
 * recovered — neither is the failure this catches, which is a loop that cannot
 * end because nothing about it changes.
 */
function noteToolUse(watch, name, input) {
  const raw = unparsedInput(input);
  if (raw === null) {
    // A call that parsed. Whatever came before it was not a spin.
    watch.key = null;
    watch.repeats = 0;
    return null;
  }
  watch.total += 1;
  watch.names.add(name);
  const key = `${name} ${raw}`;
  watch.repeats = key === watch.key ? watch.repeats + 1 : 1;
  watch.key = key;
  if (watch.limit <= 0 || watch.repeats < watch.limit) return null;
  return { name, raw, repeats: watch.repeats, total: watch.total };
}

/**
 * spinMessage is what the person who asked reads.
 *
 * It follows the message format the agents themselves use, because it arrives
 * on the same surface — and it leads with NOTHING RAN, which is the fact a
 * stale answer hides.
 */
function spinMessage(verdict) {
  return [
    '❌ **Stopped: a tool call that could not be formed**',
    '',
    `\`${verdict.name}\` was called ${verdict.repeats} times in a row with arguments that are not valid JSON, so none of them ran.`,
    '',
    '**Evidence**',
    `• \`${verdict.raw.slice(0, 200)}\``,
    '',
    '**Next**',
    "Ask again. If it repeats, the tool's own schema is the thing to fix — a parameter with no declared type is what makes a model write an unquoted value.",
  ].join('\n');
}

/**
 * discardedNotice is what a SUCCESSFUL run owes the person who asked.
 *
 * A model that gives up on a call it cannot form does not usually say so — it
 * answers from what the session already holds and presents it as current. Both
 * runs observed on 2026-08-22 did exactly that, and both were reported as
 * successes.
 *
 * So the runtime states the fact the answer omits: these calls never ran, and
 * whatever they would have fetched is not in what you are reading. It is
 * appended, never substituted — the agent's answer is still the answer.
 */
function discardedNotice(watch) {
  if (!watch || watch.total === 0) return null;
  const names = [...watch.names].slice(0, 2).map((n) => `\`${n}\``).join(', ');
  const more = watch.names.size > 2 ? ' and others' : '';
  const calls = watch.total === 1 ? '1 tool call' : `${watch.total} tool calls`;
  return (
    `⚠️ **${calls} never ran** — the arguments were not valid JSON (${names}${more}), ` +
    'so anything they would have fetched is missing from this answer.'
  );
}

module.exports = { DEFAULT_LIMIT, newSpinWatch, noteToolUse, spinMessage, discardedNotice, unparsedInput };
