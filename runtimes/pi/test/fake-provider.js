// A minimal OpenAI-compatible chat-completions SSE fake, for driving a real
// pi session in a test without a real model vendor.
//
// Pi's OpenAI-compatible client always requests `stream: true` (confirmed
// live, design.md's spike S7) — a fake returning one plain JSON body is
// rejected into a silent retry ending in `stopReason: "error"`. This answers
// Server-Sent-Events chunks matching the `chat.completion.chunk` streaming
// shape instead.
'use strict';

const http = require('http');

function startFakeProvider({ reply = 'hello from the fake', toolCall = null, toolCallOnce = false } = {}) {
  const requests = [];
  const server = http.createServer((req, res) => {
    let body = '';
    req.on('data', (c) => { body += c; });
    req.on('end', () => {
      let parsed = {};
      try { parsed = JSON.parse(body); } catch { /* logged via requests below */ }
      requests.push(parsed);
      res.writeHead(200, { 'Content-Type': 'text/event-stream' });
      const id = 'chatcmpl-fake';
      const base = { id, object: 'chat.completion.chunk', created: 0, model: 'fake-model' };
      const send = (delta, finish_reason = null, extra = {}) => {
        const chunk = { ...base, choices: [{ index: 0, delta, finish_reason }], ...extra };
        res.write(`data: ${JSON.stringify(chunk)}\n\n`);
      };
      send({ role: 'assistant', content: '' });
      const useToolCall = toolCall && (!toolCallOnce || requests.length === 1);
      if (useToolCall) {
        send({ tool_calls: [{ index: 0, id: toolCall.id || 'call_1', type: 'function', function: { name: toolCall.name, arguments: '' } }] });
        send({ tool_calls: [{ index: 0, function: { arguments: JSON.stringify(toolCall.args || {}) } }] });
        send({}, 'tool_calls');
      } else {
        send({ content: reply });
        send({}, 'stop', { usage: { prompt_tokens: 39, completion_tokens: 7, total_tokens: 46 } });
      }
      res.write('data: [DONE]\n\n');
      res.end();
    });
  });
  return new Promise((resolve) => {
    server.listen(0, '127.0.0.1', () => {
      const port = server.address().port;
      resolve({ server, port, requests, close: () => new Promise((r) => server.close(r)) });
    });
  });
}

module.exports = { startFakeProvider };
