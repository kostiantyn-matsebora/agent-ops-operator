// Tool-allowlist composition for the claude runtime.
//
// The manager sends HALF the allowlist — the tools the conversation's wiring
// (its Pipeline's MCPToolsets) contributes — plus the mode saying how it
// composes with the other half: what the agent's own definition declares in
// the `tools:` frontmatter of .claude/agents/<agent>.md. Only the runtime holds
// the repository, so only the runtime can read that half and finish the job.
//
// Verified against the real CLI (claude 2.1.226): a definition's `tools:` does
// not widen or narrow the main session on its own — `--allowedTools` is the
// sole permission authority. Nothing composes these two unless we do it here.
//
// Deliberately NOT a YAML parser. It reads one field of one shape and treats
// everything it does not understand as "declares nothing" — an unreadable role
// file must never stop an agent from answering.

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

// resolveBin looks `name` up on PATH once and returns the first absolute
// hit, falling back to the bare name so a missing binary still fails with
// the ordinary ENOENT a reader expects. go:S4036's ask, one process over:
// say explicitly where a spawned binary comes from, rather than leaving
// PATH to answer it implicitly at the call site.
function resolveBin(name, pathEnv = process.env.PATH || '') {
  for (const dir of pathEnv.split(path.delimiter)) {
    if (!dir) continue;
    const candidate = path.join(dir, name);
    try {
      fs.accessSync(candidate, fs.constants.X_OK);
      return candidate;
    } catch {
      // not here — keep looking
    }
  }
  return name;
}

// splitList turns a comma/newline separated allowlist into trimmed entries.
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

// parseFrontmatterTools extracts the `tools:` declaration from an agent
// definition's YAML frontmatter.
//
// Returns { tools: string[] } when it understood the file (an absent `tools:`
// key yields an empty list), or { error: <reason> } when it did not. Callers
// treat both as "contributes nothing"; the error only exists so it can be
// logged rather than passing silently.
function parseFrontmatterTools(text) {
  const lines = String(text).replace(/^﻿/, '').split(/\r?\n/);
  let i = 0;
  while (i < lines.length && lines[i].trim() === '') i++;
  if (i >= lines.length || lines[i].trim() !== '---') {
    return { tools: [] }; // no frontmatter at all — declares nothing, not an error
  }
  const start = i + 1;
  let end = -1;
  for (let j = start; j < lines.length; j++) {
    const t = lines[j].trim();
    if (t === '---' || t === '...') { end = j; break; }
  }
  if (end < 0) return { error: 'frontmatter opened with --- but never closed' };

  for (let j = start; j < end; j++) {
    const m = /^tools:(.*)$/.exec(lines[j]);
    if (!m) continue; // top-level key only: an indented `tools:` belongs to something else
    const inline = m[1].trim();

    if (inline.startsWith('[')) {
      if (!inline.endsWith(']')) return { error: 'tools: flow list is not closed on one line' };
      return { tools: splitList(inline.slice(1, -1)).map(unquote).filter(Boolean) };
    }
    if (inline !== '') {
      return { tools: splitList(inline).map(unquote).filter(Boolean) };
    }
    // block form: the following indented "- item" lines
    const tools = [];
    for (let k = j + 1; k < end; k++) {
      const raw = lines[k];
      if (raw.trim() === '') continue;
      // No adjacent quantifiers (javascript:S8786): the leading indent, the
      // dash and the item text are each matched by their own single-quantifier
      // step instead of one regex where \s* and .* could both claim the same
      // characters.
      const indent = /^\s+/.exec(raw);
      if (!indent || raw[indent[0].length] !== '-') break; // next key — the block ended
      const v = unquote(raw.slice(indent[0].length + 1).replace(/^\s+/, ''));
      if (v) tools.push(v);
    }
    return { tools };
  }
  return { tools: [] }; // frontmatter present, no tools: key — declares nothing
}

// agentDeclaredTools reads .claude/agents/<agent>.md under workspace and
// returns what it declares. Absent file, absent frontmatter, absent `tools:`,
// and unparseable frontmatter all yield [] — the run continues either way.
// log is called with a one-line reason whenever something was there but could
// not be used, so a typo in a role file is visible in the pod log.
function agentDeclaredTools(workspace, agent, log = () => {}) {
  if (!workspace || !agent) return [];
  const file = safeJoin(workspace, '.claude', 'agents', `${agent}.md`);
  if (!file) {
    log(`[runtime] agent definition ${agent}.md: path escapes the workspace — treating it as declaring no tools`);
    return [];
  }
  let text;
  try {
    text = fs.readFileSync(file, 'utf8');
  } catch {
    return []; // no definition — the wiring's tools stand alone
  }
  const parsed = parseFrontmatterTools(text);
  if (parsed.error) {
    log(`[runtime] agent definition ${agent}.md: ${parsed.error} — treating it as declaring no tools`);
    return [];
  }
  return parsed.tools;
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

// buildClaudeArgs is the CLI invocation's argv, pulled out of runtime.js so
// its exact shape -- in particular the jssecurity:S6350 fix -- is asserted
// directly rather than only exercised end to end. `contextId` and `prompt`
// both arrive from a work unit and are values a naive `--flag value`
// pairing hands the CLI's parser openly: a prompt beginning with "-" is
// misread as an unrecognised OPTION ("error: unknown option ..."), verified
// against the real CLI, and `--resume` takes an OPTIONAL argument ([value]
// in --help) so the same misparse hits a dash-leading context id too, one
// path over -- the value reads as absent and the id is parsed as its own
// token. `--resume=<id>` (one `=`-joined token) and a `--` separator
// immediately before the trailing positional `prompt` are the fixes;
// `--append-system-prompt <value>` needs neither, since a REQUIRED-argument
// option already consumes the very next token unconditionally.
function buildClaudeArgs({ contextId, systemPrompt, allowed, maxTurns, mcpConfig, prompt }) {
  return [
    ...(contextId ? [`--resume=${contextId}`] : []),
    ...(systemPrompt ? ['--append-system-prompt', systemPrompt] : []),
    '-p',
    '--allowedTools', allowed.join(','),
    '--permission-mode', 'dontAsk',
    '--max-turns', String(maxTurns || 60),
    '--output-format', 'stream-json',
    '--verbose',
    '--strict-mcp-config',
    '--mcp-config', mcpConfig,
    '--', prompt,
  ];
}

module.exports = {
  MODE_MERGE, MODE_OVERWRITE,
  splitList, parseFrontmatterTools, agentDeclaredTools, composeAllowedTools,
  safeJoin, sanitizeLog, resolveBin, buildClaudeArgs,
};
