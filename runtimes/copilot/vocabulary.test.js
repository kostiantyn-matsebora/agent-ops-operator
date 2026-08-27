// cd runtimes/copilot && node --test
'use strict';

const test = require('node:test');
const assert = require('node:assert');

const { translate, commandAllowed, decide, parseShellPattern, parseMcpPattern } = require('./vocabulary');

// ---- the mapping table, every row -------------------------------------------

test('each built-in maps to its Copilot wire name', () => {
  const g = translate(['Read', 'Grep', 'Glob', 'Edit', 'Write']);
  assert.deepStrictEqual(g.available, ['builtin:view', 'builtin:grep', 'builtin:glob', 'builtin:edit', 'builtin:create']);
  assert.deepStrictEqual(g.unmapped, []);
  assert.strictEqual(g.shell, null);
});

test('Bash makes the shell available and approves every command', () => {
  const g = translate(['Bash']);
  assert.deepStrictEqual(g.available, ['builtin:bash']);
  assert.deepStrictEqual(g.shell, { all: true });
});

test('Bash(kubectl:*) makes the shell available and scopes it', () => {
  const g = translate(['Bash(kubectl:*)']);
  assert.deepStrictEqual(g.available, ['builtin:bash']);
  assert.deepStrictEqual(g.shell, { prefixes: ['kubectl'] });
});

test('an MCP tool maps to mcp:<server>-<tool>', () => {
  const g = translate(['mcp__kubernetes__pods_list']);
  assert.deepStrictEqual(g.available, ['mcp:kubernetes-pods_list']);
  assert.ok(g.mcpTools.has('kubernetes-pods_list'));
});

test('order is kept and duplicates collapse', () => {
  const g = translate(['Bash', 'Read', 'Bash(git:*)', 'Read']);
  assert.deepStrictEqual(g.available, ['builtin:bash', 'builtin:view']);
  assert.deepStrictEqual(g.shell, { all: true }); // bare Bash wins over the narrowing
});

// ---- unmapped denies, never passes through, never drops silently ------------

test('an unmapped pattern is returned, not passed through and not dropped', () => {
  const g = translate(['WebFetch', 'Task', 'view']);
  assert.deepStrictEqual(g.available, []);
  assert.deepStrictEqual(g.unmapped, ['WebFetch', 'Task', 'view']);
});

test('a Copilot wire name is not accepted as an agent-ops pattern', () => {
  // `bash` is Copilot's name; agent-ops says `Bash`. Accepting the vendor's
  // spelling would let a toolset written for one vendor mean something here.
  assert.deepStrictEqual(translate(['bash']).unmapped, ['bash']);
});

test('a malformed shell pattern is unmapped', () => {
  const g = translate(['Bash(', 'Bash(kubectl; rm:*)']);
  assert.deepStrictEqual(g.available, []);
  assert.deepStrictEqual(g.unmapped, ['Bash(', 'Bash(kubectl; rm:*)']);
});

// ---- per-server wildcard is refused ------------------------------------------

test('mcp__<server>__* is refused and never widened to mcp:*', () => {
  const g = translate(['mcp__kubernetes__*']);
  assert.deepStrictEqual(g.available, []);
  assert.deepStrictEqual(g.refused, ['mcp__kubernetes__*']);
  assert.deepStrictEqual(g.unmapped, []);
});

test('any wildcard inside an MCP pattern is refused', () => {
  assert.deepStrictEqual(translate(['mcp__kubernetes__pods_*']).refused, ['mcp__kubernetes__pods_*']);
  assert.deepStrictEqual(translate(['mcp__*__list']).refused, ['mcp__*__list']);
});

test('parsers', () => {
  assert.strictEqual(parseShellPattern('Bash(kubectl:*)'), 'kubectl');
  assert.strictEqual(parseShellPattern('Bash(kubectl get:*)'), 'kubectl get');
  assert.strictEqual(parseShellPattern('Bash(git *)'), 'git');
  assert.strictEqual(parseShellPattern('Bash(ls)'), 'ls');
  assert.strictEqual(parseShellPattern('Read'), null);
  assert.deepStrictEqual(parseMcpPattern('mcp__a__b'), { server: 'a', tool: 'b' });
  assert.deepStrictEqual(parseMcpPattern('mcp__github__list_issues'), { server: 'github', tool: 'list_issues' });
  assert.strictEqual(parseMcpPattern('Read'), null);
});

// ---- sub-command matching ----------------------------------------------------

test('a bare grant approves anything', () => {
  assert.strictEqual(commandAllowed({ all: true }, 'rm -rf /; echo x'), true);
});

test('a narrowed grant approves the prefix and denies the rest', () => {
  const s = { prefixes: ['kubectl'] };
  assert.strictEqual(commandAllowed(s, 'kubectl get pods -A'), true);
  assert.strictEqual(commandAllowed(s, '  kubectl   logs x'), true);
  assert.strictEqual(commandAllowed(s, 'kubectlx get'), false);
  assert.strictEqual(commandAllowed(s, 'helm list'), false);
  assert.strictEqual(commandAllowed(s, ''), false);
  assert.strictEqual(commandAllowed(null, 'kubectl get'), false);
});

test('a two-word prefix scopes to the sub-command', () => {
  const s = { prefixes: ['kubectl get'] };
  assert.strictEqual(commandAllowed(s, 'kubectl get pods'), true);
  assert.strictEqual(commandAllowed(s, 'kubectl delete pod x'), false);
  assert.strictEqual(commandAllowed(s, 'kubectl'), false);
});

test('metacharacters that could smuggle a second command are denied under a narrowed grant', () => {
  const s = { prefixes: ['kubectl'] };
  for (const cmd of [
    'kubectl get pods; rm -rf /',
    'kubectl get pods && helm uninstall x',
    'kubectl get pods | sh',
    'kubectl get pods `id`',
    'kubectl get $(cat secret)',
    'kubectl get pods > /etc/passwd',
    'kubectl get pods < x',
    'kubectl get pods\nhelm list',
  ]) {
    assert.strictEqual(commandAllowed(s, cmd), false, cmd);
  }
});

// ---- decisions per request kind ----------------------------------------------

test('shell requests are decided by the shell grant', () => {
  const g = translate(['Bash(kubectl:*)']);
  assert.strictEqual(decide(g, { kind: 'shell', fullCommandText: 'kubectl get ns' }).allow, true);
  const d = decide(g, { kind: 'shell', fullCommandText: 'helm list' });
  assert.strictEqual(d.allow, false);
  assert.match(d.why, /helm list/);
});

test('write requests need Edit or Write', () => {
  assert.strictEqual(decide(translate(['Edit']), { kind: 'write', fileName: 'a' }).allow, true);
  assert.strictEqual(decide(translate(['Write']), { kind: 'write', fileName: 'a' }).allow, true);
  assert.strictEqual(decide(translate(['Read']), { kind: 'write', fileName: 'a' }).allow, false);
});

test('read requests need an observation tool', () => {
  assert.strictEqual(decide(translate(['Grep']), { kind: 'read', path: '/x' }).allow, true);
  assert.strictEqual(decide(translate(['Bash']), { kind: 'read', path: '/x' }).allow, false);
});

test('mcp requests are decided by the exact wire name', () => {
  const g = translate(['mcp__kubernetes__pods_list']);
  assert.strictEqual(decide(g, { kind: 'mcp', serverName: 'kubernetes', toolName: 'pods_list' }).allow, true);
  assert.strictEqual(decide(g, { kind: 'mcp', serverName: 'kubernetes', toolName: 'pods_delete' }).allow, false);
  assert.strictEqual(decide(g, { kind: 'mcp', serverName: 'github', toolName: 'pods_list' }).allow, false);
});

test('a request kind no pattern can grant is denied', () => {
  const g = translate(['Bash', 'Read', 'Edit', 'Write', 'mcp__a__b']);
  for (const kind of ['url', 'memory', 'custom-tool', 'hook', 'factory', undefined]) {
    assert.strictEqual(decide(g, { kind }).allow, false, String(kind));
  }
});

test('nothing granted denies everything', () => {
  const g = translate([]);
  assert.deepStrictEqual(g.available, []);
  for (const r of [{ kind: 'shell', fullCommandText: 'ls' }, { kind: 'write', fileName: 'a' }, { kind: 'read', path: 'a' }, { kind: 'mcp', serverName: 'a', toolName: 'b' }]) {
    assert.strictEqual(decide(g, r).allow, false);
  }
});
