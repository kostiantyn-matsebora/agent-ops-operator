// MCP configuration translation: the manager's mcp.json → the SDK's mcpServers.
//
// internal/mcpcompile writes `{"mcpServers": {<name>: {type?, url?, command?,
// args?, headers?, env?}}}` and claude-code reads it directly. The Copilot SDK
// takes the same facts as a typed record, so this maps field for field:
// `{command,args,env}` → stdio, `{type:http,url,headers}` → http.
//
// ONE THING DOES NOT CARRY. The manager writes a secret-backed value as a
// `${ENV}` placeholder and relies on claude-code expanding it from the pod's
// environment. The SDK takes literal strings, so THIS module expands them —
// in-process, never logged — and a placeholder with no value FAILS THAT
// SERVER's registration with a logged reason, rather than reaching an MCP
// server as the literal text `${TOKEN}`.

'use strict';

const fs = require('fs');

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

// translateServers turns the parsed mcp.json into the SDK record.
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
      servers[name] = { type, url: s.url, ...(Object.keys(headers.value).length ? { headers: headers.value } : {}) };
      continue;
    }
    if (!s.command) { failed.push({ name, reason: 'stdio server without command' }); continue; }
    const envMap = expandMap(s.env, env);
    if (envMap.missing) { failed.push({ name, reason: `unresolved placeholder in env (${envMap.missing.join(', ')})` }); continue; }
    servers[name] = {
      type: 'stdio',
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

module.exports = { expand, translateServers, loadMcpServers };
