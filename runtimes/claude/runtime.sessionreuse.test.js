// cd runtime-claude && node --test
//
// openspec/changes/claude-runtime-sdk/tasks.md 9.4 — the configuration
// fingerprint (design.md D4) and the open/reuse/re-open decision it drives.
//
// The pure fingerprint functions are tested directly first, then the live
// path through executeUnit()/openOrReuseSession() with a SCRIPTED fake
// query() substituted via __setQueryForTest — the real
// `@anthropic-ai/claude-agent-sdk` package is never require()'d by this
// file (9.6: no network access needed or possible here).
'use strict';

const test = require('node:test');
const assert = require('node:assert');

const {
  computeFingerprint, changedFingerprintFields, fingerprintsEqual, FINGERPRINT_FIELDS, normalizedMaxTurns,
  __setQueryForTest, __getLiveSessionForTest, __resetLiveSessionForTest,
  openOrReuseSession, executeUnit, WORKSPACE,
} = require('./runtime');

// ---- the fingerprint, field by field ---------------------------------------

// The allowlist ITSELF is not a field (it lives in the canUseTool callback,
// D3). The built-in set it OFFERS is (coordinator-move-per-turn D-F): the
// SDK's `tools` option is set once per query(), so it is session-affecting.
test('FINGERPRINT_FIELDS is exactly the six session-affecting fields: the allowlist is not one, the built-ins it offers is', () => {
  assert.deepStrictEqual([...FINGERPRINT_FIELDS].sort(), ['agent', 'cwd', 'maxTurns', 'mcpServers', 'systemPrompt', 'offeredTools'].sort());
});

test('computeFingerprint reads systemPrompt, maxTurns and agent from the unit, and mcpServers/cwd independently', () => {
  const fp = computeFingerprint({ systemPrompt: 'be nice', maxTurns: 12, agent: 'k8s-ops' }, { srv: {} });
  assert.strictEqual(fp.systemPrompt, 'be nice');
  assert.deepStrictEqual(fp.mcpServers, { srv: {} });
  assert.strictEqual(fp.maxTurns, 12);
  assert.strictEqual(fp.cwd, WORKSPACE);
  assert.strictEqual(fp.agent, 'k8s-ops');
  assert.deepStrictEqual(fp.offeredTools, [], 'no composed allowlist offers no built-in');
});

test('computeFingerprint derives offeredTools from the composed allowlist: built-in base names only, MCP grants excluded', () => {
  const fp = computeFingerprint({}, {}, ['mcp__aops__*', 'Read', 'Bash(git *)']);
  assert.deepStrictEqual(fp.offeredTools, ['Bash', 'Read'], 'sorted, so the same set in another order is one fingerprint');
});

test('computeFingerprint is order-blind: the same built-ins in another allowlist order are one fingerprint', () => {
  const a = computeFingerprint({}, {}, ['Read', 'Bash', 'mcp__aops__*']);
  const b = computeFingerprint({}, {}, ['mcp__aops__*', 'Bash', 'Read']);
  assert.deepStrictEqual(a, b);
});

test('computeFingerprint normalizes an absent maxTurns to the same floor normalizedMaxTurns applies', () => {
  const fp = computeFingerprint({}, {});
  assert.strictEqual(fp.maxTurns, normalizedMaxTurns({}));
  assert.strictEqual(fp.systemPrompt, '', 'an absent systemPrompt reads as empty, not undefined');
  assert.strictEqual(fp.agent, '', 'an absent agent reads as empty, not undefined');
});

test('changedFingerprintFields names exactly the one field that changed, for each field in turn', () => {
  const base = { systemPrompt: 'a', mcpServers: { x: {} }, maxTurns: 10, cwd: '/w', agent: 'ag', offeredTools: ['Read'] };
  const changes = [
    ['systemPrompt', 'b'],
    ['mcpServers', { y: {} }],
    ['maxTurns', 20],
    ['cwd', '/other'],
    ['agent', 'different-agent'],
    ['offeredTools', ['Read', 'Bash']],
  ];
  for (const [field, changedValue] of changes) {
    const next = { ...base, [field]: changedValue };
    assert.deepStrictEqual(changedFingerprintFields(base, next), [field], `expected only ${field} to register as changed`);
    assert.strictEqual(fingerprintsEqual(base, next), false);
  }
});

test('fingerprintsEqual is true, and changedFingerprintFields empty, for two identical fingerprints', () => {
  const fp = computeFingerprint({ systemPrompt: 'a', agent: 'x' }, { a: {} });
  assert.strictEqual(fingerprintsEqual(fp, { ...fp }), true);
  assert.deepStrictEqual(changedFingerprintFields(fp, { ...fp }), []);
});

// The decision D4 exists to protect: the tool allowlist is composed per
// unit and lives in the canUseTool callback (D3), not in the fingerprint —
// computeFingerprint never reads unit.allowedTools/unit.toolsMode at all,
// only the COMPOSED list's offered built-ins. An allowlist change that
// offers the same built-ins (a scope, an MCP grant) is still no change.
test('an allowlist change offering the same built-ins does not register as a fingerprint change', () => {
  const mcpServers = { srv: {} };
  const unit = { systemPrompt: 'be nice', agent: 'k8s-ops', allowedTools: 'Read', toolsMode: 'merge' };
  const fp1 = computeFingerprint(unit, mcpServers, ['Read', 'Bash(git *)', 'mcp__aops__*']);
  const fp2 = computeFingerprint({ ...unit, allowedTools: 'Read,Bash,Edit', toolsMode: 'overwrite' }, mcpServers, ['Read', 'Bash(kubectl *)', 'mcp__kubernetes__*']);
  assert.deepStrictEqual(changedFingerprintFields(fp1, fp2), []);
  assert.strictEqual(fingerprintsEqual(fp1, fp2), true);
});

// D-F: the `tools` option is query()-wide, so a different offered set is a
// different session.
test('an allowlist change that offers a different built-in set registers as an offeredTools change', () => {
  const fp1 = computeFingerprint({}, {}, ['Read']);
  const fp2 = computeFingerprint({}, {}, ['Read', 'Bash']);
  assert.deepStrictEqual(changedFingerprintFields(fp1, fp2), ['offeredTools']);
  assert.strictEqual(fingerprintsEqual(fp1, fp2), false);
});

// ---- a scripted fake query(), modelled on the SDK's typed messages --------

// makeFakeQuery returns a substitute for query() that never touches the
// real SDK or network. `sessionsScript[n]` is the flat event queue for the
// (n+1)th time the runtime calls query() — i.e. one open or one re-open —
// letting a test pre-load exactly how many vendor "sessions" it expects to
// see opened, and what each one says.
function makeFakeQuery(sessionsScript) {
  const sessions = [];
  function fakeQuery({ options } = {}) {
    const session = { events: (sessionsScript[sessions.length] || []).slice(), closed: false, interrupted: false, options };
    sessions.push(session);
    return {
      async next() {
        if (!session.events.length) return { value: undefined, done: true };
        return { value: session.events.shift(), done: false };
      },
      async close() { session.closed = true; },
      async interrupt() { session.interrupted = true; },
    };
  }
  fakeQuery.sessions = sessions;
  return fakeQuery;
}

function resultEvent(sessionId, text, extra = {}) {
  return { type: 'result', subtype: 'success', num_turns: 1, duration_ms: 5, result: text, is_error: false, session_id: sessionId, ...extra };
}

// Each test gets a clean module-level liveSession — node's test runner does
// NOT isolate module-level `let` state across tests in the same file.
function freshSession(t) {
  __resetLiveSessionForTest();
  t.after(() => __resetLiveSessionForTest());
}

function withConsoleLog(t) {
  const lines = [];
  const real = console.log;
  console.log = (...args) => { lines.push(args.join(' ')); };
  t.after(() => { console.log = real; });
  return lines;
}

// ---- open / reuse / re-open, driven directly through openOrReuseSession ---

test('openOrReuseSession logs open, reuse and re-open as three distinct lines', (t) => {
  freshSession(t);
  const fakeQuery = makeFakeQuery([[], []]);
  __setQueryForTest(fakeQuery);
  const logged = withConsoleLog(t);

  const fpA = computeFingerprint({ systemPrompt: 'a' }, {});
  const fpB = computeFingerprint({ systemPrompt: 'b' }, {});
  openOrReuseSession({ runId: 'r1' }, fpA, ['Read'], {});
  openOrReuseSession({ runId: 'r2' }, fpA, ['Read'], {}); // identical fingerprint -> reuse
  openOrReuseSession({ runId: 'r3' }, fpB, ['Read'], {}); // changed fingerprint -> re-open

  assert.strictEqual(fakeQuery.sessions.length, 2, 'open + re-open is two vendor sessions; reuse opens none');
  assert.ok(logged.some((l) => l.includes('[runtime] session open')), `expected an open log, got: ${logged.join(' | ')}`);
  assert.ok(logged.some((l) => l.includes('[runtime] session reuse')), `expected a reuse log, got: ${logged.join(' | ')}`);
  assert.ok(
    logged.some((l) => l.includes('[runtime] session re-open') && l.includes('systemPrompt')),
    `expected a re-open log naming systemPrompt, got: ${logged.join(' | ')}`,
  );
});

// ---- the full round trip through executeUnit -------------------------------

test('two units with an identical fingerprint reuse the open session, opening the vendor session exactly once', async (t) => {
  freshSession(t);
  const fakeQuery = makeFakeQuery([[resultEvent('sess-1', 'first answer'), resultEvent('sess-1', 'second answer')]]);
  __setQueryForTest(fakeQuery);

  const base = { promptText: 'hi', systemPrompt: 'be nice' };
  const out1 = await executeUnit({ ...base, runId: 'r1' });
  assert.strictEqual(out1.status, 'succeeded');
  assert.strictEqual(out1.sessionId, 'sess-1');
  assert.strictEqual(fakeQuery.sessions.length, 1, 'the first unit opens exactly one vendor session');

  const out2 = await executeUnit({ ...base, runId: 'r2' });
  assert.strictEqual(out2.status, 'succeeded');
  assert.match(out2.result, /second answer/);
  assert.strictEqual(fakeQuery.sessions.length, 1, 'an identical fingerprint must reuse the open session, not open a second one');
  assert.strictEqual(fakeQuery.sessions[0].closed, false, 'a reused session must not be closed');
  assert.ok(__getLiveSessionForTest(), 'the live session stays open across the two reused units');
});

test('a changed fingerprint on the second unit closes the session and opens a second one, logging the field that changed', async (t) => {
  freshSession(t);
  const fakeQuery = makeFakeQuery([[resultEvent('sess-A', 'first')], [resultEvent('sess-B', 'second')]]);
  __setQueryForTest(fakeQuery);
  const logged = withConsoleLog(t);

  const out1 = await executeUnit({ runId: 'r1', promptText: 'hi', systemPrompt: 'be nice' });
  assert.strictEqual(out1.sessionId, 'sess-A');
  assert.strictEqual(fakeQuery.sessions.length, 1);

  const out2 = await executeUnit({ runId: 'r2', promptText: 'hi', systemPrompt: 'be stern' }); // systemPrompt changed
  assert.strictEqual(out2.sessionId, 'sess-B');
  assert.strictEqual(fakeQuery.sessions.length, 2, 'a changed fingerprint must open a SECOND vendor session');
  assert.strictEqual(fakeQuery.sessions[0].closed, true, 'the superseded session must be closed before the replacement opens');
  assert.strictEqual(fakeQuery.sessions[1].closed, false);
  assert.ok(
    logged.some((l) => l.includes('[runtime] session re-open') && l.includes('systemPrompt')),
    `expected the re-open logged naming systemPrompt, got: ${logged.join(' | ')}`,
  );
});

test('an allowlist change offering the same built-ins reuses the open session, the live callback seeing the new list', async (t) => {
  freshSession(t);
  const fakeQuery = makeFakeQuery([[resultEvent('sess-1', 'first'), resultEvent('sess-1', 'second')]]);
  __setQueryForTest(fakeQuery);

  await executeUnit({ runId: 'r1', promptText: 'hi', allowedTools: 'Read,Bash(git *),mcp__aops__*', toolsMode: 'overwrite' });
  assert.strictEqual(fakeQuery.sessions.length, 1);

  const out2 = await executeUnit({ runId: 'r2', promptText: 'hi', allowedTools: 'Read,Bash(kubectl *),mcp__kubernetes__*', toolsMode: 'overwrite' });
  assert.strictEqual(fakeQuery.sessions.length, 1, 'the same offered built-ins must not force a re-open — the allowlist itself is not fingerprinted');
  assert.strictEqual(out2.sessionId, 'sess-1');
  // the live callback must see the NEW allowlist on the reused session (D4)
  assert.deepStrictEqual(__getLiveSessionForTest().allowedHolder.current, ['Read', 'Bash(kubectl *)', 'mcp__kubernetes__*']);
});

// coordinator-move-per-turn 6.2 (design D-F): the offered built-in set is a
// query()-wide option, so a unit whose allowlist offers a different set
// re-opens the session, and the fresh query() is handed the new set.
test('a unit whose allowlist offers a different built-in set re-opens the session, logging offeredTools as the changed field', async (t) => {
  freshSession(t);
  const fakeQuery = makeFakeQuery([[resultEvent('sess-A', 'first')], [resultEvent('sess-B', 'second')]]);
  __setQueryForTest(fakeQuery);
  const logged = withConsoleLog(t);

  const out1 = await executeUnit({ runId: 'r1', promptText: 'hi', allowedTools: 'mcp__aops__*', toolsMode: 'overwrite' });
  assert.strictEqual(out1.sessionId, 'sess-A');
  assert.strictEqual(fakeQuery.sessions.length, 1);
  assert.deepStrictEqual(fakeQuery.sessions[0].options.tools, [], 'an aops-only allowlist offers no built-in');

  const out2 = await executeUnit({ runId: 'r2', promptText: 'hi', allowedTools: 'Read,Grep', toolsMode: 'overwrite' });
  assert.strictEqual(out2.sessionId, 'sess-B');
  assert.strictEqual(fakeQuery.sessions.length, 2, 'a different offered built-in set must open a SECOND vendor session');
  assert.strictEqual(fakeQuery.sessions[0].closed, true, 'the superseded session must be closed before the replacement opens');
  assert.deepStrictEqual(fakeQuery.sessions[1].options.tools, ['Read', 'Grep'], 'the fresh query() is offered the new set');
  assert.ok(
    logged.some((l) => l.includes('[runtime] session re-open') && l.includes('offeredTools')),
    `expected the re-open logged naming offeredTools, got: ${logged.join(' | ')}`,
  );
});
