// The MCP extension (design.md's D3): the manager's mcp.json → connected MCP
// servers → `mcp__<server>__<tool>` custom tools on a pi session.
//
// Pi core has no MCP of its own, so this is the ONE extension the image
// ships, and it is the CLIENT half only:
//
//   MCP_CONFIG ──▶ parse ──▶ expand ${ENV} ──▶ connect (listed servers only)
//                                                   │
//                                      tools/list ──┘
//                                           │
//                       name = mcp__<server>__<tool>
//                                           │
//                 register only if the composed allowlist permits it
//
// The TRANSLATION half (`expand`/`expandMap`/`translateServers`/
// `loadMcpServers`) is ported field for field from `runtimes/copilot/mcp.js`
// — same shape, same failure reasons, a reviewer reads one shape twice. What
// differs from copilot is everything AFTER translation: copilot hands its
// translated record straight to the Copilot SDK's own `mcpServers` session
// option and the vendor connects it; pi's SDK has no such option, so this
// file connects each server itself, over `@earendil-works/pi-mcp` (the same
// vendor scope pi-coding-agent itself depends on, pinned here explicitly
// rather than left to transitive resolution) — never a third-party MCP
// package, which D3 rejected for putting an unreviewed dependency between
// the manager's compiled config and the agent.
//
// ONE THING DOES NOT CARRY, same as copilot's: the manager writes a
// secret-backed value as a `${ENV}` placeholder, and THIS module expands it
// — in-process, never logged — right before that server's connection is
// opened. A placeholder with no value fails THAT server's registration with
// a logged reason naming the variable, never its value, rather than reaching
// an MCP server as the literal text `${TOKEN}` or leaking the real one.
//
// `type: "sse"` is the manager's own name for an HTTP MCP server
// (`internal/mcpcompile`), not the legacy two-endpoint SSE transport —
// connected here the same way as `type: "http"`, over pi-mcp's
// `StreamableHttpTransport`, the only HTTP MCP transport the pinned client
// ships.

'use strict';

const fs = require('fs');
const { parseMcpPattern } = require('./vocabulary');
const { sanitizeLog } = require('./tools');

const PLACEHOLDER = /\$\{([A-Za-z_][A-Za-z0-9_]*)\}/g;

// expand replaces every `${VAR}` in s from env. Returns { value } or
// { missing: [names] } — the caller decides what a missing one costs.
function expand(s, env) {
  const missing = [];
  const value = String(s).replace(PLACEHOLDER, (_, name) => {
    if (Object.prototype.hasOwnProperty.call(env, name) && env[name] !== undefined) return env[name];
    missing.push(name);
    return '';
  });
  return missing.length ? { missing } : { value };
}

function expandMap(m, env) {
  const out = {};
  const missing = [];
  for (const [k, v] of Object.entries(m || {})) {
    const r = expand(v, env);
    if (r.missing) missing.push(...r.missing.map((n) => `${k}: \${${n}}`));
    else out[k] = r.value;
  }
  return missing.length ? { missing } : { value: out };
}

// translateServers turns the parsed mcp.json into this runtime's own record,
// keyed by server name. `tools: ['*']` is carried for parity with copilot's
// shape (the SDK-facing field there, read by nothing of pi's) — this
// runtime's own allowlist decision (task 3.3) is made later, against the
// composed allowlist, not against this field.
// Returns { servers: Record<name, config>, failed: [{name, reason}] }.
function translateServers(parsed, env = process.env) {
  const servers = {};
  const failed = [];
  const src = (parsed && parsed.mcpServers) || {};
  for (const [name, s] of Object.entries(src)) {
    if (!s || typeof s !== 'object') { failed.push({ name, reason: 'not an object' }); continue; }
    const type = String(s.type || (s.url ? 'http' : 'stdio')).toLowerCase();
    if (type === 'http' || type === 'sse') {
      if (!s.url) { failed.push({ name, reason: 'http server without url' }); continue; }
      const headers = expandMap(s.headers, env);
      if (headers.missing) { failed.push({ name, reason: `unresolved placeholder in headers (${headers.missing.join(', ')})` }); continue; }
      servers[name] = { type, url: s.url, tools: ['*'], ...(Object.keys(headers.value).length ? { headers: headers.value } : {}) };
      continue;
    }
    if (!s.command) { failed.push({ name, reason: 'stdio server without command' }); continue; }
    const envMap = expandMap(s.env, env);
    if (envMap.missing) { failed.push({ name, reason: `unresolved placeholder in env (${envMap.missing.join(', ')})` }); continue; }
    servers[name] = {
      type: 'stdio',
      tools: ['*'],
      command: s.command,
      args: Array.isArray(s.args) ? s.args.map(String) : [],
      ...(Object.keys(envMap.value).length ? { env: envMap.value } : {}),
    };
  }
  return { servers, failed };
}

// loadMcpServers reads the file the manager mounted. An absent file is an
// empty record — a conversation with no MCPConfig bound has none — and an
// unreadable one is reported as a single failure so the run says so.
function loadMcpServers(file, env = process.env) {
  let text;
  try {
    text = fs.readFileSync(file, 'utf8');
  } catch (e) {
    if (e && e.code === 'ENOENT') return { servers: {}, failed: [] };
    return { servers: {}, failed: [{ name: file, reason: `read: ${e.message}` }] };
  }
  let parsed;
  try {
    parsed = JSON.parse(text);
  } catch (e) {
    return { servers: {}, failed: [{ name: file, reason: `parse: ${e.message}` }] };
  }
  return translateServers(parsed, env);
}

// ---- connect + register (pi-specific — pi core has no MCP of its own) -----

let mcpSdk = null;

// Loaded lazily, same reasoning as runtime.js's getSdk(): `@earendil-works/
// pi-mcp` is ESM-only, so a test requiring this module for its translation
// functions alone never pays that import cost.
async function getMcpSdk() {
  if (!mcpSdk) mcpSdk = await import('@earendil-works/pi-mcp');
  return mcpSdk;
}

// allowlistFrom turns the composed allowlist's raw `mcp__...` patterns
// (vocabulary.translate's `mcpPatterns`) into a (server, tool) => boolean
// predicate. A `mcp__<server>__*` pattern permits every tool of THAT server
// and no other's — the per-server wildcard never leaks across servers.
function allowlistFrom(mcpPatterns) {
  const exact = new Set();
  const wildcard = new Set();
  for (const raw of mcpPatterns || []) {
    const parsed = parseMcpPattern(String(raw).trim());
    if (!parsed) continue;
    if (parsed.tool === '*') wildcard.add(parsed.server);
    else exact.add(`${parsed.server}\u0000${parsed.tool}`);
  }
  return (server, tool) => wildcard.has(server) || exact.has(`${server}\u0000${tool}`);
}

// connectServer opens a transport for one translated server config and
// returns a connected McpClient. stdio is spawned as a child of this
// process, same residual risk the claude/copilot runtimes already carry
// (design.md's Risks); an HTTP(S) server stays behind the egress proxy.
async function connectServer(name, cfg) {
  const mcp = await getMcpSdk();
  const transport = cfg.type === 'stdio'
    ? new mcp.StdioTransport({ command: cfg.command, args: cfg.args || [], env: cfg.env, inheritEnv: true })
    : new mcp.StreamableHttpTransport({ url: cfg.url, headers: cfg.headers });
  const client = new mcp.McpClient({ name: 'agentops-runtime-pi', version: '0.1.0' });
  await client.connect(transport);
  return client;
}

// registerServerTools connects to one server, lists its tools, and registers
// every one the allowlist permits as `mcp__<server>__<tool>`. Never throws —
// a connect failure or a tools/list failure is logged and leaves this server
// with no tools registered, exactly like a translation failure (task 3.1):
// one bad server must never cost every other one its tools. Returns the
// connected client on success, so the caller can close it on session
// shutdown, or undefined when this server never connected.
//
// `handle` is the caller's own `{ toolNames, clients }` (mutated via
// `push`, never reassigned):
//   - `toolNames` is every `mcp__<server>__<tool>` name actually registered
//     on `pi` — `pi.registerTool()` alone puts a tool into the session's
//     registry, but `createAgentSession`'s own `tools:` option is a
//     SEPARATE, explicit activation list (confirmed live: an explicit
//     `tools` array becomes the session's `initialActiveToolNames`
//     outright, bypassing any tool's own default-active behaviour) — so
//     the caller that builds that option needs this list back, not just
//     pi's say-so that registration succeeded.
//   - `clients` is every connected `McpClient` this server contributed, so
//     the caller can close it when ITS OWN session ends (see
//     registerMcpTools below — `pi.on('session_shutdown', ...)` is not
//     enough on its own).
async function registerServerTools(pi, name, cfg, allow, log, handle) {
  let client;
  try {
    client = await connectServer(name, cfg);
  } catch (e) {
    log(`[mcp] server "${sanitizeLog(name)}" could not be connected: ${sanitizeLog(e.message)}`);
    return undefined;
  }
  let tools;
  try {
    tools = await client.listTools();
  } catch (e) {
    log(`[mcp] server "${sanitizeLog(name)}" tools/list failed: ${sanitizeLog(e.message)}`);
    try { await client.close(); } catch { /* best effort */ }
    return undefined;
  }
  const mcp = await getMcpSdk();
  let count = 0;
  for (const tool of tools) {
    if (!allow(name, tool.name)) continue;
    const toolName = `mcp__${name}__${tool.name}`;
    count++;
    handle.toolNames.push(toolName);
    pi.registerTool({
      name: toolName,
      label: `${name}/${tool.name}`,
      description: tool.description && tool.description.trim() ? tool.description.trim() : `MCP tool ${tool.name} from server ${name}`,
      // An object schema, which is what pi's own TypeBox-typed `parameters`
      // is at runtime — same normalization pi's own (unused by this
      // runtime) built-in MCP extension applies to a server's raw schema.
      parameters: mcp.normalizeMcpInputSchema(tool.inputSchema || {}),
      async execute(_toolCallId, params, signal) {
        const result = await client.callTool(tool.name, params || {}, { signal });
        return {
          content: mcp.toLlmContent(result),
          details: { server: name, tool: tool.name },
          ...(result.isError ? { isError: true } : {}),
        };
      },
    });
  }
  log(`[mcp] server "${sanitizeLog(name)}" connected — ${tools.length} tool(s) listed, ${count} registered`);
  return client;
}

// registerMcpTools is mcp.js's one entry point: read MCP_CONFIG, connect to
// every server it lists (never any other), and register each tool the
// composed allowlist permits. A missing or unreadable MCP_CONFIG is no
// servers at all (task 3.1) — never an error, never a blocked run. Intended
// as a pi `ExtensionFactory`: `(pi) => registerMcpTools(pi, MCP_CONFIG,
// grant.mcpPatterns)`, joining the tool_call-permission factory
// buildResourceLoader already registers.
//
// Every connected client is ALSO closed on `session_shutdown` — the
// extension lifecycle event pi fires before a session-replacement flow
// (resume, fork, switchSession) tears the old session down — as a second,
// defensive path. It is NOT the primary one: this runtime's own
// `attempt()` (runtime.js) calls the bare `AgentSession.dispose()` between
// units, which — confirmed live — never emits `session_shutdown` at all
// (only the SDK's own higher-level session-replacement path does). So
// `attempt()` closes `handle.clients` itself once a run ends, and this
// listener exists only for a pi-driven lifecycle event this runtime does
// not currently trigger.
//
// `handle`, when the caller passes its own `{ toolNames, clients }`
// (defaulted fresh per call otherwise), is mutated with every
// `mcp__<server>__<tool>` name actually registered and every connected
// `McpClient` — the caller is runtime.js's `buildResourceLoader`, which
// `await`s this factory (through `loader.reload()`) to completion before
// `openSession`/`attempt()` read either list, so both are fully populated
// by the time they are.
async function registerMcpTools(pi, file, mcpPatterns, env = process.env, log = console.log, handle = { toolNames: [], clients: [] }) {
  let servers, failed;
  try {
    ({ servers, failed } = loadMcpServers(file, env));
  } catch (e) {
    // loadMcpServers itself never throws today, but an extension factory
    // must never take the session down with it — defensive, not reachable.
    log(`[mcp] ${sanitizeLog(e.message)}`);
    return handle;
  }
  for (const f of failed) log(`[mcp] server "${sanitizeLog(f.name)}" not registered: ${sanitizeLog(f.reason)}`);
  const allow = allowlistFrom(mcpPatterns);
  for (const [name, cfg] of Object.entries(servers)) {
    const client = await registerServerTools(pi, name, cfg, allow, log, handle);
    if (client) handle.clients.push(client);
  }
  if (handle.clients.length && typeof pi.on === 'function') {
    pi.on('session_shutdown', async () => {
      await Promise.all(handle.clients.map((c) => c.close().catch(() => {})));
    });
  }
  return handle;
}

module.exports = {
  expand, expandMap, translateServers, loadMcpServers,
  allowlistFrom, connectServer, registerServerTools, registerMcpTools, getMcpSdk,
};
