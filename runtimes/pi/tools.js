// Tool-allowlist composition for the pi runtime.
//
// The manager sends HALF the allowlist — the tools the conversation's wiring
// (its Pipeline's MCPToolsets) contributes — plus the mode saying how it
// composes with the other half: what the agent's own definition declares in
// the `tools:` frontmatter of .claude/agents/<agent>.md. Only the runtime
// holds the repository, so only the runtime can read that half and finish
// the job. Same path as runtimes/claude and runtimes/ollama read (RT-3.4) —
// kept in step by hand, since a runtime is its own component and copies
// nothing across a directory boundary.
//
// pi has no `tools:`-frontmatter convention of its own to collide with — the
// SDK takes the system prompt as plain text, so this file ALSO returns the
// definition's body, which runtime.js appends to the unit's own
// `systemPrompt` (task 2.6). agentDeclaredTools's copilot/claude siblings
// never needed the body; pi's does, since nothing else carries the role text
// into the session here.
//
// Deliberately NOT a YAML parser. It reads one field of one shape and treats
// everything it does not understand as "declares nothing" — an unreadable
// role file must never stop an agent from answering.

'use strict';

const fs = require('fs');
const path = require('path');

const MODE_MERGE = 'merge';
const MODE_OVERWRITE = 'overwrite';

// safeJoin resolves base/...segments and refuses a result that escapes base
// -- jssecurity:S2083's ask, since the caller below joins a directory this
// process controls with a name that arrives from data it does not (a CR's
// agent name); runtime.js's promptFile caller does the same for a work
// unit's prompt file. null on escape, so a caller can treat it exactly like
// a missing file rather than reading outside its intended tree.
function safeJoin(base, ...segments) {
  const root = path.resolve(base);
  const target = path.resolve(root, ...segments);
  if (target !== root && !target.startsWith(root + path.sep)) return null;
  return target;
}

// sanitizeLog strips CR/LF from a value before it reaches a log line --
// jssecurity:S5145's ask, since a crafted runId, agent name or thread id in a
// work unit could otherwise forge a second log line that reads as the
// runtime's own. /[\n\r]/g is the rule's own documented compliant pattern.
function sanitizeLog(v) {
  return String(v).replace(/[\n\r]/g, '_');
}

// splitList turns a comma-separated allowlist into trimmed entries.
function splitList(s) {
  if (!s) return [];
  return String(s).split(',').map((t) => t.trim()).filter(Boolean);
}

// unquote strips one layer of matching quotes from a scalar.
function unquote(s) {
  const t = s.trim();
  if (t.length >= 2 && ((t[0] === '"' && t.endsWith('"')) || (t[0] === "'" && t.endsWith("'")))) {
    return t.slice(1, -1).trim();
  }
  return t;
}

// parseFrontmatter extracts the `tools:` declaration AND the body text (the
// definition's own prompt) from an agent definition's YAML frontmatter.
//
// Returns { tools: string[], body: string } when it understood the file (an
// absent `tools:` key yields an empty list; no frontmatter at all yields the
// whole text as the body), or { error: <reason> } when it did not. Callers
// treat an error exactly like an empty composition — contributes nothing to
// either half — logging it rather than letting it pass silently.
function parseFrontmatter(text) {
  const raw = String(text).replace(/^﻿/, '');
  const lines = raw.split(/\r?\n/);
  let i = 0;
  while (i < lines.length && lines[i].trim() === '') i++;
  if (i >= lines.length || lines[i].trim() !== '---') {
    return { tools: [], body: raw.trim() }; // no frontmatter at all — the whole file is body
  }
  const start = i + 1;
  let end = -1;
  for (let j = start; j < lines.length; j++) {
    const t = lines[j].trim();
    if (t === '---' || t === '...') { end = j; break; }
  }
  if (end < 0) return { error: 'frontmatter opened with --- but never closed' };
  const body = lines.slice(end + 1).join('\n').trim();

  for (let j = start; j < end; j++) {
    const m = /^tools:(.*)$/.exec(lines[j]);
    if (!m) continue; // top-level key only: an indented `tools:` belongs to something else
    const inline = m[1].trim();

    if (inline.startsWith('[')) {
      if (!inline.endsWith(']')) return { error: 'tools: flow list is not closed on one line' };
      return { tools: splitList(inline.slice(1, -1)).map(unquote).filter(Boolean), body };
    }
    if (inline !== '') {
      return { tools: splitList(inline).map(unquote).filter(Boolean), body };
    }
    // block form: the following indented "- item" lines
    const tools = [];
    for (let k = j + 1; k < end; k++) {
      const line = lines[k];
      if (line.trim() === '') continue;
      // No adjacent quantifiers (javascript:S8786): the leading indent, the
      // dash and the item text are each matched by their own single-quantifier
      // step instead of one regex where \s* and .* could both claim the same
      // characters.
      const indent = /^\s+/.exec(line);
      if (!indent || line[indent[0].length] !== '-') break; // next key — the block ended
      const v = unquote(line.slice(indent[0].length + 1).replace(/^\s+/, ''));
      if (v) tools.push(v);
    }
    return { tools, body };
  }
  return { tools: [], body }; // frontmatter present, no tools: key — declares nothing
}

// definitionPath is where every .claude/agents-reading runtime (claude,
// ollama, pi — RT-3.4) keeps an agent definition. null when the agent name
// would escape the workspace.
function definitionPath(workspace, agent) {
  return safeJoin(workspace, '.claude', 'agents', `${agent}.md`);
}

// readAgentDefinition reads .claude/agents/<agent>.md under workspace and
// returns what it declares: { tools, body }. Absent, unreadable or
// unparseable all yield { tools: [], body: '' } — the run continues either
// way (RT-3.3). log is called with a one-line reason whenever something was
// there but could not be used, so a typo in a role file is visible in the
// pod log.
function readAgentDefinition(workspace, agent, log = () => {}) {
  if (!workspace || !agent) return { tools: [], body: '' };
  const file = definitionPath(workspace, agent);
  if (!file) {
    log(`[runtime] agent definition ${agent}.md: path escapes the workspace — treating it as declaring no tools and no body`);
    return { tools: [], body: '' };
  }
  let text;
  try {
    text = fs.readFileSync(file, 'utf8');
  } catch {
    return { tools: [], body: '' }; // no definition — the wiring's tools stand alone, no body to append
  }
  const parsed = parseFrontmatter(text);
  if (parsed.error) {
    log(`[runtime] agent definition ${agent}.md: ${parsed.error} — treating it as declaring no tools and no body`);
    return { tools: [], body: '' };
  }
  return { tools: parsed.tools, body: parsed.body };
}

// agentFileDefinition reads { tools, body } from an explicit absolute path —
// the resolved `agentFile` a work unit carries in place of `agent` once the
// wiring names one (multi-repo-workspace). Exclusive with
// readAgentDefinition's own workspace+name lookup: the caller picks one or
// the other per unit, never both. Same tolerant contract as
// readAgentDefinition — a missing or unparseable file yields { tools: [],
// body: '' }, logged rather than failing the run.
function agentFileDefinition(agentFile, log = () => {}) {
  if (!agentFile) return { tools: [], body: '' };
  let text;
  try {
    text = fs.readFileSync(agentFile, 'utf8');
  } catch {
    return { tools: [], body: '' }; // no definition — the wiring's tools stand alone, no body to append
  }
  const parsed = parseFrontmatter(text);
  if (parsed.error) {
    log(`[runtime] agent file ${agentFile}: ${parsed.error} — treating it as declaring no tools and no body`);
    return { tools: [], body: '' };
  }
  return { tools: parsed.tools, body: parsed.body };
}

// composeAllowedTools joins the agent's declared tools with the wiring's per
// mode: overwrite passes the wiring's alone, merge unions them with the
// agent's keeping their position. Any mode we do not recognise — including an
// absent one, which is what an object stored before the field existed sends —
// is merge, because reading it as overwrite would silently strip what the
// agent declared.
function composeAllowedTools(agentTools, wiringTools, mode) {
  const wiring = Array.isArray(wiringTools) ? wiringTools : splitList(wiringTools);
  if (mode === MODE_OVERWRITE) return dedup(wiring);
  const agent = Array.isArray(agentTools) ? agentTools : splitList(agentTools);
  return dedup(agent.concat(wiring));
}

function dedup(list) {
  const seen = new Set();
  const out = [];
  for (const raw of list) {
    const t = String(raw).trim();
    if (!t || seen.has(t)) continue;
    seen.add(t);
    out.push(t);
  }
  return out;
}

module.exports = {
  MODE_MERGE, MODE_OVERWRITE,
  splitList, parseFrontmatter, definitionPath, readAgentDefinition, agentFileDefinition, composeAllowedTools,
  safeJoin, sanitizeLog,
};
