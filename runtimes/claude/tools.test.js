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
