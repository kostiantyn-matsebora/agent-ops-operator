// Vocabulary translation: agent-ops tool patterns → pi's built-in tool names.
//
// MCPToolset patterns are opaque and claude-flavoured cluster-wide (`Read`,
// `Bash(kubectl:*)`, `mcp__kubernetes__pods_list`). A Pipeline binds the same
// toolsets whichever runtime serves it, so the vendor difference is absorbed
// HERE, at the one boundary where vendor knowledge already lives.
//
// Confirmed live against SDK 1.5.2 (design.md's spike S1): pi offers a
// built-in's name as its own tool_call `toolName` — no availability/
// permission split like copilot's, no per-call narrowing like claude's
// `--allowedTools` prefixes. Enabled WHOLE or NOT AT ALL, the same posture
// `runtimes/ollama`'s Go-side Gate holds: a scoped `Bash(<prefix>:*)` pattern
// cannot be enforced (pi hands the whole command to its own bash tool with
// no prefix hook), so it enables nothing and is logged, exactly as a
// narrowing pattern is `ignored` there.
//
// THE MAPPING (design D2), pinned by the spike against a live session's
// `getActiveToolNames()`:
const BUILTIN = Object.freeze({
  Read: 'read',
  Write: 'write',
  Edit: 'edit',
  Bash: 'bash',
  Grep: 'grep',
  Glob: 'find',
});

// parseShellPattern reads `Bash(<prefix>:*)` / `Bash(<prefix> *)` and returns
// the scoped prefix it names — never honoured (no per-invocation hook to
// enforce it against), only extracted so the caller can log a specific
// reason rather than a bare "unmapped".
function parseShellPattern(p) {
  const m = /^Bash\((.*)\)$/.exec(p);
  if (!m) return null;
  return m[1].trim();
}

// parseMcpPattern reads `mcp__<server>__<tool>` and returns {server, tool},
// with tool '*' for the per-server wildcard, or null for a non-MCP pattern.
// Section 3 (the MCP extension) is the only consumer of mcpServerNames below
// — this module decides none of it, it only recognises the shape so an MCP
// pattern is never misread as "unmapped".
function parseMcpPattern(p) {
  if (!p.startsWith('mcp__')) return null;
  const rest = p.slice(5);
  if (rest.startsWith('_')) return null;
  const sep = rest.indexOf('__');
  if (sep < 1) return null;
  const tool = rest.slice(sep + 2);
  if (!tool) return null;
  return { server: rest.slice(0, sep), tool };
}

// translate maps a composed allowlist into what this session grants.
//
// Returns:
//   builtinNames  — pi's own tool names to grant, in first-seen order
//   mcpPatterns   — the raw `mcp__...` patterns granted, for section 3 to
//                   resolve against the servers it actually connects
//   scoped        — `Bash(...)` patterns seen (logged, granting nothing)
//   unmapped      — patterns nothing here understands (logged, withheld)
function translate(patterns) {
  const builtinNames = [];
  const seen = new Set();
  const add = (name) => { if (!seen.has(name)) { seen.add(name); builtinNames.push(name); } };
  const mcpPatterns = [];
  const scoped = [];
  const unmapped = [];

  for (const raw of patterns || []) {
    const p = String(raw).trim();
    if (!p) continue;

    if (Object.prototype.hasOwnProperty.call(BUILTIN, p)) {
      add(BUILTIN[p]);
      continue;
    }

    const prefix = parseShellPattern(p);
    if (prefix !== null) { scoped.push(p); continue; }

    if (parseMcpPattern(p)) { mcpPatterns.push(p); continue; }

    unmapped.push(p);
  }
  return { builtinNames, mcpPatterns, scoped, unmapped };
}

// checkInventory logs every granted built-in name the session did not
// actually register. A wire name is a vendor fact that moves; a wrong one
// must surface on every run, not just the first.
function checkInventory(builtinNames, registeredNames) {
  const registered = new Set(registeredNames || []);
  const gaps = [];
  for (const name of builtinNames || []) {
    if (!registered.has(name)) gaps.push(`mapped built-in "${name}" is not registered by this session — the mapping is stale`);
  }
  return gaps;
}

// decide answers one `tool_call` event (task 2.7) from a flat list of
// granted names. Exact match only — pi's built-ins carry no scoping of
// their own (no per-command prefix, no per-path narrowing), so "granted"
// and "not granted" are the only two outcomes. An `mcp__<server>__<tool>`
// custom tool (section 3) is granted the same way, by its own registered
// name — the caller (runtime.js's `buildResourceLoader`) is the one that
// folds `mcpHandle.toolNames` in alongside the built-ins before calling
// this, since this function itself knows nothing about MCP. Deny by
// default — an unrecognised toolName is refused, never offered an
// interactive prompt.
function decide(builtinNames, toolName) {
  const granted = new Set(builtinNames || []);
  const allow = granted.has(toolName);
  return { allow, why: allow ? `${toolName} granted` : `${toolName || '(unnamed tool)'} is not in the composed allowlist` };
}

module.exports = { BUILTIN, parseShellPattern, parseMcpPattern, translate, checkInventory, decide };
