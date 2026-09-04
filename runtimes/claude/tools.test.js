// cd runtime-claude && node --test
'use strict';

const test = require('node:test');
const assert = require('node:assert');
const fs = require('fs');
const os = require('os');
const path = require('path');

const {
  MODE_MERGE, MODE_OVERWRITE,
  parseFrontmatterTools, agentDeclaredTools, composeAllowedTools,
  safeJoin, sanitizeLog, resolveBin, buildClaudeArgs,
} = require('./tools');

// mkRepo builds a throwaway checkout containing the given agent definitions.
function mkRepo(defs = {}) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'agentops-tools-'));
  const agents = path.join(dir, '.claude', 'agents');
  fs.mkdirSync(agents, { recursive: true });
  for (const [name, body] of Object.entries(defs)) {
    fs.writeFileSync(path.join(agents, `${name}.md`), body);
  }
  return dir;
}

// ---- 3.1 reading the definition ---------------------------------------------

test('inline comma list is read', () => {
  assert.deepStrictEqual(
    parseFrontmatterTools('---\nname: a\ntools: Read, Grep\n---\nbody\n').tools,
    ['Read', 'Grep'],
  );
});

test('flow list is read', () => {
  assert.deepStrictEqual(
    parseFrontmatterTools('---\ntools: [Read, "Bash(git *)"]\n---\n').tools,
    ['Read', 'Bash(git *)'],
  );
});

test('block list is read', () => {
  assert.deepStrictEqual(
    parseFrontmatterTools('---\nname: a\ntools:\n  - Read\n  - Grep\ndescription: x\n---\n').tools,
    ['Read', 'Grep'],
  );
});

test('a definition with no tools: key declares nothing', () => {
  assert.deepStrictEqual(parseFrontmatterTools('---\nname: a\n---\nbody\n').tools, []);
});

test('a definition with no frontmatter declares nothing, and is not an error', () => {
  const got = parseFrontmatterTools('# Just prose\n\nno frontmatter here\n');
  assert.deepStrictEqual(got.tools, []);
  assert.strictEqual(got.error, undefined);
});

test('an indented tools: belongs to another key and is not read', () => {
  assert.deepStrictEqual(parseFrontmatterTools('---\nnested:\n  tools: Bash\n---\n').tools, []);
});

test('unterminated frontmatter is an error, not a crash', () => {
  const got = parseFrontmatterTools('---\ntools: Read\nnever closed\n');
  assert.ok(got.error, 'expected an error reason');
  assert.strictEqual(got.tools, undefined);
});

test('unclosed flow list is an error', () => {
  assert.ok(parseFrontmatterTools('---\ntools: [Read, Grep\n---\n').error);
});

test('a missing definition file contributes nothing', () => {
  const dir = mkRepo({});
  assert.deepStrictEqual(agentDeclaredTools(dir, 'nobody'), []);
});

test('an agent name that escapes the workspace contributes nothing, and logs why', () => {
  const dir = mkRepo({});
  const logged = [];
  const got = agentDeclaredTools(dir, '../../../etc/passwd', (m) => logged.push(m));
  assert.deepStrictEqual(got, []);
  assert.match(logged[0], /escapes the workspace/);
});

// ---- safeJoin ----------------------------------------------------------------

test('safeJoin joins ordinary segments', () => {
  const dir = mkRepo({});
  assert.strictEqual(safeJoin(dir, '.claude', 'agents', 'x.md'), path.join(dir, '.claude', 'agents', 'x.md'));
});

test('safeJoin refuses a segment that escapes base via ..', () => {
  const dir = mkRepo({});
  assert.strictEqual(safeJoin(dir, '..', '..', 'etc', 'passwd'), null);
  assert.strictEqual(safeJoin(dir, '../../../../etc/passwd'), null);
});

test('safeJoin refuses an absolute segment that resolves outside base', () => {
  const dir = mkRepo({});
  assert.strictEqual(safeJoin(dir, '/etc/passwd'), null);
});

test('safeJoin allows base itself, with no segments', () => {
  const dir = mkRepo({});
  assert.strictEqual(safeJoin(dir), dir);
});

// ---- sanitizeLog --------------------------------------------------------------

test('sanitizeLog passes ordinary text through unchanged', () => {
  assert.strictEqual(sanitizeLog('run-42'), 'run-42');
});

test('sanitizeLog strips CR/LF so a value cannot forge a second log line', () => {
  assert.strictEqual(sanitizeLog('id\n[runtime] FAKE line\r\n'), 'id_[runtime] FAKE line__');
});

test('sanitizeLog stringifies a non-string value', () => {
  assert.strictEqual(sanitizeLog(42), '42');
});

// ---- resolveBin ----------------------------------------------------------------

test('resolveBin finds an executable on PATH and returns its absolute path', () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'agentops-bin-'));
  const bin = path.join(dir, 'agentops-fake-bin');
  fs.writeFileSync(bin, '#!/bin/sh\n');
  fs.chmodSync(bin, 0o755);
  assert.strictEqual(resolveBin('agentops-fake-bin', dir), bin);
});

test('resolveBin falls back to the bare name when nothing on PATH matches', () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'agentops-bin-'));
  assert.strictEqual(resolveBin('agentops-nowhere', dir), 'agentops-nowhere');
});

test('resolveBin skips a non-executable file with the right name', () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'agentops-bin-'));
  fs.writeFileSync(path.join(dir, 'agentops-not-exec'), '#!/bin/sh\n');
  assert.strictEqual(resolveBin('agentops-not-exec', dir), 'agentops-not-exec');
});

test('no agent name and no workspace contribute nothing', () => {
  assert.deepStrictEqual(agentDeclaredTools('', 'x'), []);
  assert.deepStrictEqual(agentDeclaredTools(mkRepo({}), ''), []);
});

test('unparseable frontmatter is logged and contributes nothing, never throws', () => {
  const dir = mkRepo({ broken: '---\ntools: Read\nthis file never closes its frontmatter\n' });
  const logged = [];
  const got = agentDeclaredTools(dir, 'broken', (m) => logged.push(m));
  assert.deepStrictEqual(got, []);
  assert.strictEqual(logged.length, 1, 'the reason must reach the pod log');
  assert.match(logged[0], /broken\.md/);
});

test('a declared list reaches the caller', () => {
  const dir = mkRepo({ 'k8s-engineer': '---\nname: k8s-engineer\ntools: Read, Grep\n---\nrole\n' });
  assert.deepStrictEqual(agentDeclaredTools(dir, 'k8s-engineer'), ['Read', 'Grep']);
});

// ---- 3.2 composition ---------------------------------------------------------

test('merge extends what the agent declares, agent keeping position', () => {
  assert.deepStrictEqual(
    composeAllowedTools(['Read', 'Grep'], 'Bash', MODE_MERGE),
    ['Read', 'Grep', 'Bash'],
  );
});

test('merge dedups across the two halves', () => {
  assert.deepStrictEqual(
    composeAllowedTools(['Read', 'Grep'], 'Grep,Bash,Read', MODE_MERGE),
    ['Read', 'Grep', 'Bash'],
  );
});

test('overwrite replaces the agent declaration entirely', () => {
  assert.deepStrictEqual(
    composeAllowedTools(['Read', 'Grep'], 'Bash', MODE_OVERWRITE),
    ['Bash'],
  );
});

test('merge with nothing declared is the wiring alone', () => {
  assert.deepStrictEqual(composeAllowedTools([], 'Bash,Read', MODE_MERGE), ['Bash', 'Read']);
});

test('merge with no wiring tools is the agent declaration alone', () => {
  assert.deepStrictEqual(composeAllowedTools(['Read'], '', MODE_MERGE), ['Read']);
});

// An absent mode is what an older manager, or an object stored before the
// field existed, sends. It must never read as overwrite.
test('an absent or unknown mode composes as merge', () => {
  assert.deepStrictEqual(composeAllowedTools(['Read'], 'Bash', undefined), ['Read', 'Bash']);
  assert.deepStrictEqual(composeAllowedTools(['Read'], 'Bash', ''), ['Read', 'Bash']);
  assert.deepStrictEqual(composeAllowedTools(['Read'], 'Bash', 'nonsense'), ['Read', 'Bash']);
});

// ---- 3.3 empty means empty ---------------------------------------------------

test('nothing declared anywhere yields an empty allowlist, not a substituted Read', () => {
  assert.deepStrictEqual(composeAllowedTools([], '', MODE_MERGE), []);
  assert.deepStrictEqual(composeAllowedTools([], '', MODE_OVERWRITE), []);
});

test('overwrite with an empty wiring grants nothing even when the agent declared tools', () => {
  assert.deepStrictEqual(composeAllowedTools(['Read', 'Bash'], '', MODE_OVERWRITE), []);
});

test('blank entries in the wiring list are dropped, not passed as empty tools', () => {
  assert.deepStrictEqual(composeAllowedTools([], ' Read , , Bash ', MODE_MERGE), ['Read', 'Bash']);
});

// The end-to-end shape the runtime passes to the CLI.
test('the composed allowlist joins to a comma-separated argument', () => {
  const dir = mkRepo({ probe: '---\ntools: Read, Glob\n---\n' });
  const declared = agentDeclaredTools(dir, 'probe');
  assert.strictEqual(
    composeAllowedTools(declared, 'Bash', MODE_MERGE).join(','),
    'Read,Glob,Bash',
  );
  assert.strictEqual(
    composeAllowedTools(declared, 'Bash', MODE_OVERWRITE).join(','),
    'Bash',
  );
});

// ---- buildClaudeArgs (jssecurity:S6350) ----------------------------------------

test('buildClaudeArgs puts the prompt last, after a -- separator', () => {
  const args = buildClaudeArgs({ allowed: ['Read'], mcpConfig: '/etc/mcp.json', prompt: 'hello' });
  assert.deepStrictEqual(args.slice(-2), ['--', 'hello']);
});

test('buildClaudeArgs is unaffected by a prompt beginning with -', () => {
  const args = buildClaudeArgs({ allowed: [], mcpConfig: '/etc/mcp.json', prompt: '--dangerously-skip-permissions' });
  const sepIndex = args.indexOf('--');
  assert.strictEqual(args[sepIndex + 1], '--dangerously-skip-permissions');
  assert.strictEqual(args.length, sepIndex + 2, 'the prompt is the LAST argv element');
});

test('buildClaudeArgs joins a resume id with = rather than a separate token', () => {
  const args = buildClaudeArgs({ contextId: 'sess-123', allowed: [], mcpConfig: '/x', prompt: 'p' });
  assert.strictEqual(args[0], '--resume=sess-123');
});

test('buildClaudeArgs = -joins a resume id even when it begins with -', () => {
  const args = buildClaudeArgs({ contextId: '--fake-session', allowed: [], mcpConfig: '/x', prompt: 'p' });
  assert.strictEqual(args[0], '--resume=--fake-session');
});

test('buildClaudeArgs omits --resume with no context id', () => {
  const args = buildClaudeArgs({ allowed: [], mcpConfig: '/x', prompt: 'p' });
  assert.ok(!args.some((a) => a.startsWith('--resume')));
});

test('buildClaudeArgs appends the system prompt as its own required-argument flag, unmodified', () => {
  const args = buildClaudeArgs({ systemPrompt: '--looks-like-a-flag', allowed: [], mcpConfig: '/x', prompt: 'p' });
  const i = args.indexOf('--append-system-prompt');
  assert.notStrictEqual(i, -1);
  assert.strictEqual(args[i + 1], '--looks-like-a-flag');
});

test('buildClaudeArgs omits --append-system-prompt with none given', () => {
  const args = buildClaudeArgs({ allowed: [], mcpConfig: '/x', prompt: 'p' });
  assert.ok(!args.includes('--append-system-prompt'));
});

test('buildClaudeArgs carries the composed allowlist, the mcp config path, and defaults max-turns to 60', () => {
  const args = buildClaudeArgs({ allowed: ['Read', 'Bash'], mcpConfig: '/etc/agentops/mcp.json', prompt: 'p' });
  assert.strictEqual(args[args.indexOf('--allowedTools') + 1], 'Read,Bash');
  assert.strictEqual(args[args.indexOf('--mcp-config') + 1], '/etc/agentops/mcp.json');
  assert.strictEqual(args[args.indexOf('--max-turns') + 1], '60');
});

test('buildClaudeArgs honours an explicit max-turns', () => {
  const args = buildClaudeArgs({ allowed: [], mcpConfig: '/x', maxTurns: 12, prompt: 'p' });
  assert.strictEqual(args[args.indexOf('--max-turns') + 1], '12');
});

// The three fixed flags a pod's own contract depends on, none of which vary
// by call site — a hang (no --permission-mode dontAsk, the interactive
// default prompts for a person who isn't there), a parse the manager can't
// read (no --output-format stream-json), or a raw mcp.json edited outside
// the referenced config file being silently honoured (no --strict-mcp-config)
// are each a real failure mode this pins against a future edit dropping one.
test('buildClaudeArgs always carries permission-mode, output-format and strict-mcp-config', () => {
  const args = buildClaudeArgs({ allowed: [], mcpConfig: '/x', prompt: 'p' });
  assert.strictEqual(args[args.indexOf('--permission-mode') + 1], 'dontAsk');
  assert.strictEqual(args[args.indexOf('--output-format') + 1], 'stream-json');
  assert.ok(args.includes('--strict-mcp-config'));
});
