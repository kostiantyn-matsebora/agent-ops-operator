// A minimal MCP stdio server, for driving a real `StdioTransport`/`McpClient`
// connection in a test without a real MCP server.
//
// Newline-delimited JSON-RPC on stdout, matching `@earendil-works/pi-mcp`'s
// `StdioTransport`/`JsonRpcLineReader` framing (no Content-Length headers).
// Tools are configurable through `FAKE_MCP_TOOLS` (a JSON array of MCP
// `Tool` objects) so one script serves every test's server shape.
'use strict';

const readline = require('readline');

const TOOLS = JSON.parse(
  process.env.FAKE_MCP_TOOLS ||
    '[{"name":"echo","description":"echoes its message","inputSchema":{"type":"object","properties":{"message":{"type":"string"}}}}]'
);

function send(msg) {
  process.stdout.write(`${JSON.stringify(msg)}\n`);
}

function callTool(req) {
  const params = req.params || {};
  const name = params.name;
  const args = params.arguments || {};
  if (name === 'echo') {
    send({ jsonrpc: '2.0', id: req.id, result: { content: [{ type: 'text', text: String(args.message || '') }] } });
    return;
  }
  if (name === 'env-marker') {
    // Reports only WHETHER a named env var is set, never its value — proof this server received a
    // secret-backed env var without that secret ever flowing back through a tool result, which the
    // runtime would otherwise log or carry into the transcript.
    const present = Boolean(process.env[args.name]);
    send({ jsonrpc: '2.0', id: req.id, result: { content: [{ type: 'text', text: present ? 'present' : 'absent' }] } });
    return;
  }
  send({ jsonrpc: '2.0', id: req.id, error: { code: -32601, message: `unknown tool ${name}` } });
}

const rl = readline.createInterface({ input: process.stdin, terminal: false });
rl.on('line', (line) => {
  if (!line.trim()) return;
  let req;
  try {
    req = JSON.parse(line);
  } catch {
    return;
  }
  switch (req.method) {
    case 'initialize':
      send({
        jsonrpc: '2.0',
        id: req.id,
        result: {
          protocolVersion: (req.params && req.params.protocolVersion) || '2025-06-18',
          capabilities: { tools: {} },
          serverInfo: { name: 'agentops-fake-mcp-server', version: '0.0.1' },
        },
      });
      return;
    case 'notifications/initialized':
      return; // a notification carries no id and gets no response
    case 'tools/list':
      send({ jsonrpc: '2.0', id: req.id, result: { tools: TOOLS } });
      return;
    case 'tools/call':
      callTool(req);
      return;
    default:
      send({ jsonrpc: '2.0', id: req.id, error: { code: -32601, message: `unknown method ${req.method}` } });
  }
});
rl.on('close', () => process.exit(0));
