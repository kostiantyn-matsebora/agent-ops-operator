// cd runtimes/copilot && node --test
'use strict';

const test = require('node:test');
const assert = require('node:assert');
const fs = require('fs');
const os = require('os');
const path = require('path');

const {
  MODE_MERGE, MODE_OVERWRITE,
  parseFrontmatterTools, agentDeclaredTools, composeAllowedTools, definitionPath,
  safeJoin, sanitizeLog, resolveBin,
} = require('./tools');

// mkRepo builds a throwaway checkout containing the given agent definitions,
// at the path THIS vendor reads.
function mkRepo(defs = {}) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'agentops-copilot-tools-'));
  const agents = path.join(dir, '.github', 'agents');
  fs.mkdirSync(agents, { recursive: true });
  for (const [name, body] of Object.entries(defs)) {
    fs.writeFileSync(path.join(agents, `${name}.agent.md`), body);
  }
  return dir;
}

// ---- reading the definition ------------------------------------------------

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

test('a definition with no tools: key declares nothing — the vendor default of "everything" never applies', () => {
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

test('unclosed frontmatter is an error, reported not thrown', () => {
  const got = parseFrontmatterTools('---\ntools: Read\nbody without closing\n');
  assert.ok(got.error);
  assert.strictEqual(got.tools, undefined);
});

test('an unclosed flow list is an error', () => {
  assert.ok(parseFrontmatterTools('---\ntools: [Read, Grep\n---\n').error);
});

test('a single-quoted flow item is unquoted too, not just double-quoted', () => {
  // Every earlier flow-list test used double quotes; unquote's single-quote
  // arm (`t[0] === "'" && t.endsWith("'")`) had never run.
  assert.deepStrictEqual(
    parseFrontmatterTools("---\ntools: [Read, 'Bash(git *)']\n---\n").tools,
    ['Read', 'Bash(git *)'],
  );
});

test('leading blank lines before the opening --- are skipped', () => {
  assert.deepStrictEqual(
    parseFrontmatterTools('\n\n---\ntools: Read\n---\n').tools,
    ['Read'],
  );
});

test('a blank line inside a block list does not end it', () => {
  assert.deepStrictEqual(
    parseFrontmatterTools('---\ntools:\n  - Read\n\n  - Grep\n---\n').tools,
    ['Read', 'Grep'],
  );
});

// ---- the path is this vendor's ----------------------------------------------

test('the definition is read from .github/agents/<agent>.agent.md', () => {
  assert.strictEqual(definitionPath('/ws', 'k8s'), path.join('/ws', '.github', 'agents', 'k8s.agent.md'));
});

test('a declared list is read from the checkout', () => {
  const repo = mkRepo({ k8s: '---\nname: k8s\ntools: Read, Bash(kubectl:*)\n---\n' });
  assert.deepStrictEqual(agentDeclaredTools(repo, 'k8s'), ['Read', 'Bash(kubectl:*)']);
});

test('a definition at the claude path is NOT read by this runtime', () => {
  const repo = mkRepo({});
  const claude = path.join(repo, '.claude', 'agents');
  fs.mkdirSync(claude, { recursive: true });
  fs.writeFileSync(path.join(claude, 'k8s.md'), '---\ntools: Bash\n---\n');
  assert.deepStrictEqual(agentDeclaredTools(repo, 'k8s'), []);
});

test('no definition contributes nothing', () => {
  assert.deepStrictEqual(agentDeclaredTools(mkRepo({}), 'missing'), []);
  assert.deepStrictEqual(agentDeclaredTools('', 'x'), []);
  assert.deepStrictEqual(agentDeclaredTools(mkRepo({}), ''), []);
});

test('an agent name that escapes the workspace contributes nothing, and logs why', () => {
  const dir = mkRepo({});
  const logged = [];
  const got = agentDeclaredTools(dir, '../../../etc/passwd', (m) => logged.push(m));
  assert.deepStrictEqual(got, []);
  assert.match(logged[0], /escapes the workspace/);
  assert.strictEqual(definitionPath(dir, '../../../etc/passwd'), null);
});

// ---- safeJoin ----------------------------------------------------------------

test('safeJoin joins ordinary segments', () => {
  const dir = mkRepo({});
  assert.strictEqual(safeJoin(dir, '.github', 'agents', 'x.agent.md'), path.join(dir, '.github', 'agents', 'x.agent.md'));
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

test('resolveBin falls back to reading process.env.PATH when no pathEnv is passed', () => {
  // Every other resolveBin test supplies pathEnv explicitly, so the default
  // parameter `process.env.PATH || ''` has never been evaluated.
  assert.strictEqual(resolveBin('agentops-surely-nowhere-on-path'), 'agentops-surely-nowhere-on-path');
});

test('resolveBin copes with no PATH in the environment at all', () => {
  // The above only exercises the "process.env.PATH is set" half of that
  // default; a runtime that somehow inherits no PATH must still fail closed
  // to the bare name rather than throwing on a split of undefined.
  const saved = process.env.PATH;
  delete process.env.PATH;
  try {
    assert.strictEqual(resolveBin('agentops-surely-nowhere-either'), 'agentops-surely-nowhere-either');
  } finally {
    process.env.PATH = saved;
  }
});

test('resolveBin skips an empty PATH segment (a leading or doubled separator)', () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'agentops-bin-'));
  const bin = path.join(dir, 'agentops-fake-bin-2');
  fs.writeFileSync(bin, '#!/bin/sh\n');
  fs.chmodSync(bin, 0o755);
  assert.strictEqual(resolveBin('agentops-fake-bin-2', `${path.delimiter}${dir}`), bin);
});

test('unparseable frontmatter with no log callback given uses the default no-op silently', () => {
  // Every other agentDeclaredTools test supplies its own log callback; the
  // default `log = () => {}` parameter had never been invoked.
  const repo = mkRepo({ bad: '---\ntools: [Read\nnever closed\n' });
  assert.deepStrictEqual(agentDeclaredTools(repo, 'bad'), []);
});

test('unparseable frontmatter is logged and contributes nothing', () => {
  const repo = mkRepo({ bad: '---\ntools: [Read\nnever closed\n' });
  const logs = [];
  assert.deepStrictEqual(agentDeclaredTools(repo, 'bad', (m) => logs.push(m)), []);
  assert.strictEqual(logs.length, 1);
  assert.match(logs[0], /bad\.agent\.md/);
});

// ---- composition ------------------------------------------------------------

test('merge unions, agent first, deduped', () => {
  assert.deepStrictEqual(
    composeAllowedTools(['Read', 'Grep'], 'Grep,Bash', MODE_MERGE),
    ['Read', 'Grep', 'Bash'],
  );
});

test('overwrite passes the wiring alone', () => {
  assert.deepStrictEqual(composeAllowedTools(['Read'], 'Bash', MODE_OVERWRITE), ['Bash']);
});

test('an unknown or absent mode reads as merge', () => {
  assert.deepStrictEqual(composeAllowedTools(['Read'], 'Bash', undefined), ['Read', 'Bash']);
  assert.deepStrictEqual(composeAllowedTools(['Read'], 'Bash', 'whatever'), ['Read', 'Bash']);
});

test('nothing composed stays nothing', () => {
  assert.deepStrictEqual(composeAllowedTools([], '', MODE_MERGE), []);
  assert.deepStrictEqual(composeAllowedTools(['Read'], '', MODE_OVERWRITE), []);
});

test('wiringTools already an array is used as-is, not re-split', () => {
  // Every other composition test passes wiringTools as a comma string; the
  // caller may also hand over an already-split array (e.g. relaying another
  // component's parsed list), and Array.isArray(wiringTools) had never been true.
  assert.deepStrictEqual(composeAllowedTools(['Read'], ['Bash', 'Edit'], MODE_MERGE), ['Read', 'Bash', 'Edit']);
});

test('agentTools passed as a comma string is split, not just arrays', () => {
  // The reverse gap: every earlier test passed agentTools as an array, so
  // Array.isArray(agentTools) had never been false.
  assert.deepStrictEqual(composeAllowedTools('Read,Grep', 'Bash', MODE_MERGE), ['Read', 'Grep', 'Bash']);
});
