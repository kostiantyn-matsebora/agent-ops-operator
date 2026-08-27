// Vocabulary translation: agent-ops tool patterns → Copilot's two layers.
//
// MCPToolset patterns are opaque and claude-flavoured cluster-wide (`Read`,
// `Bash(kubectl:*)`, `mcp__kubernetes__pods_list`). A Pipeline binds the same
// toolsets whichever runtime serves it, so the vendor difference is absorbed
// HERE, at the one boundary where vendor knowledge already lives.
//
// Copilot splits what claude-code fuses:
//   - AVAILABILITY: `availableTools` — which tools exist for the session, as
//     source-qualified filters `builtin:<name>` / `mcp:<server>-<tool>`.
//   - PERMISSION: `onPermissionRequest` — whether one invocation is approved.
// A shell pattern therefore becomes two things: `builtin:bash` available, and a
// callback that approves only the commands the pattern names.
//
// THREE RULES, each paid for by reasoning through the alternative:
//   1. UNMAPPED DENIES. A pattern this file does not understand contributes
//      nothing and is returned so the caller logs it. Passing it through would
//      hand Copilot a string it reads as some other tool; dropping it silently
//      would narrow (or, for a wildcard, widen) a route with no record.
//   2. A PER-SERVER WILDCARD IS REFUSED, never widened. Copilot admits `mcp:*`
//      (every server) or an exact wire name — there is no per-server form, and
//      `mcp:*` would grant every other MCP server bound to that conversation.
//   3. A NARROWING SHELL PATTERN IS HONOURED, in the permission layer. The
//      ollama runtime has no per-invocation hook and grants nothing on it; this
//      one has one and uses it. What a runtime can enforce is its own fact.
//
// The wire names are PINNED against SDK 1.0.11 / CLI 1.0.80 (2026-08-27) by
// advertising `builtin:*` to a fake model and reading the request. They are a
// vendor fact that moves, which is why the runtime also checks them against the
// registered inventory at start and logs any target that is not there.

'use strict';

// BUILTIN maps an agent-ops built-in name to Copilot's wire name.
const BUILTIN = Object.freeze({
  Read: 'view',
  Grep: 'grep',
  Glob: 'glob',
  Edit: 'edit',
  Write: 'create',
  Bash: 'bash',
});

// Shell metacharacters that could smuggle a second command past a prefix
// match. A narrowed shell grant denies any command carrying one; a bare `Bash`
// grant approves everything and never reaches this check.
const SMUGGLE = /[;&|`$<>\n\r]/;

// parseShellPattern reads `Bash(<prefix>:*)` (claude-code's form) or
// `Bash(<prefix> *)` and returns the command prefix it scopes to, or null for
// a shape it does not recognise.
function parseShellPattern(p) {
  const m = /^Bash\((.*)\)$/.exec(p);
  if (!m) return null;
  let inner = m[1].trim();
  if (inner.endsWith(':*')) inner = inner.slice(0, -2);
  else if (inner.endsWith(' *')) inner = inner.slice(0, -2);
  else if (inner.endsWith('*')) inner = inner.slice(0, -1);
  inner = inner.trim();
  if (!inner || /[;&|`$<>]/.test(inner)) return null;
  return inner;
}

// parseMcpPattern reads `mcp__<server>__<tool>` and returns {server, tool},
// with tool '*' for the per-server wildcard, or null for a non-MCP pattern.
function parseMcpPattern(p) {
  const m = /^mcp__([^_].*?)__(.+)$/.exec(p);
  if (!m) return null;
  return { server: m[1], tool: m[2] };
}

// translate maps a composed allowlist into what the session is created with.
//
// Returns:
//   available   — the `availableTools` filters, in first-seen order
//   shell       — { all: true } | { prefixes: string[] } | null (no shell)
//   builtins    — the agent-ops names that were granted (for the log line)
//   mcpTools    — Set of `<server>-<tool>` wire names granted
//   unmapped    — patterns nothing here understands (logged, withheld)
//   refused     — per-server wildcards (logged, withheld, never widened)
function translate(patterns) {
  const available = [];
  const seen = new Set();
  const add = (f) => { if (!seen.has(f)) { seen.add(f); available.push(f); } };
  const builtins = [];
  const mcpTools = new Set();
  const unmapped = [];
  const refused = [];
  let shell = null;

  for (const raw of patterns || []) {
    const p = String(raw).trim();
    if (!p) continue;

    if (p === 'Bash') {
      add('builtin:bash');
      shell = { all: true };
      builtins.push(p);
      continue;
    }
    const prefix = parseShellPattern(p);
    if (prefix !== null) {
      add('builtin:bash');
      if (!shell) shell = { prefixes: [] };
      if (!shell.all) shell.prefixes.push(prefix);
      builtins.push(p);
      continue;
    }
    if (p.startsWith('Bash(')) { unmapped.push(p); continue; }

    if (Object.prototype.hasOwnProperty.call(BUILTIN, p)) {
      add(`builtin:${BUILTIN[p]}`);
      builtins.push(p);
      continue;
    }

    const mcp = parseMcpPattern(p);
    if (mcp) {
      if (mcp.tool === '*' || mcp.tool.includes('*') || mcp.server.includes('*')) {
        refused.push(p);
        continue;
      }
      const wire = `${mcp.server}-${mcp.tool}`;
      add(`mcp:${wire}`);
      mcpTools.add(wire);
      continue;
    }

    unmapped.push(p);
  }
  return { available, shell, builtins, mcpTools, unmapped, refused };
}

// commandAllowed decides one shell invocation against a shell grant.
// A bare grant approves everything. A narrowed grant approves a command whose
// first word plus following words equal a prefix (`kubectl` matches `kubectl
// get pods`, `kubectl get` matches `kubectl get pods` and not `kubectl delete`),
// and denies any command carrying a metacharacter that could chain a second
// one. Deny is the safe direction.
function commandAllowed(shell, commandText) {
  if (!shell) return false;
  if (shell.all) return true;
  const cmd = String(commandText || '').trim();
  if (!cmd || SMUGGLE.test(cmd)) return false;
  const words = cmd.split(/\s+/);
  return shell.prefixes.some((prefix) => {
    const pw = prefix.split(/\s+/);
    if (pw.length > words.length) return false;
    return pw.every((w, i) => w === words[i]);
  });
}

// decide answers one permission request from the translated grant.
// Returns { allow: boolean, why: string } — the caller turns it into the SDK's
// result kind and logs the denials with the pattern that failed.
function decide(grant, request) {
  const kind = request && request.kind;
  switch (kind) {
    case 'shell': {
      const ok = commandAllowed(grant.shell, request.fullCommandText);
      return { allow: ok, why: ok ? 'shell grant' : `no shell grant matches: ${String(request.fullCommandText || '').slice(0, 160)}` };
    }
    case 'write': {
      const ok = grant.builtins.includes('Edit') || grant.builtins.includes('Write');
      return { allow: ok, why: ok ? 'Edit/Write granted' : `write to ${request.fileName} without Edit or Write` };
    }
    case 'read': {
      const ok = grant.builtins.includes('Read') || grant.builtins.includes('Grep') || grant.builtins.includes('Glob');
      return { allow: ok, why: ok ? 'Read granted' : `read of ${request.path} without Read/Grep/Glob` };
    }
    case 'mcp': {
      const wire = `${request.serverName}-${request.toolName}`;
      const ok = grant.mcpTools.has(wire);
      return { allow: ok, why: ok ? 'mcp tool granted' : `mcp tool ${wire} is not in the allowlist` };
    }
    default:
      // url, memory, custom-tool, hook, extension management, factory: none
      // has an agent-ops pattern, so none is ever granted.
      return { allow: false, why: `no agent-ops pattern grants ${kind || 'an unknown request kind'}` };
  }
}

module.exports = { BUILTIN, parseShellPattern, parseMcpPattern, translate, commandAllowed, decide };
