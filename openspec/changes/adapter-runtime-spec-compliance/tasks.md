# Tasks — adapter-runtime-spec-compliance

Finding ids (C/O/P/A/B/S) are the audit's, restated per task. Specs carry
the behaviour, design.md the approach.

## 1. Runtime continuity and report truthfulness — backend-developer

- [x] 1.1 claude: an unreadable session store is an outage — readdir errors
  propagate to the `unavailable` path instead of reading as "not found"
  (C1). Verify: `node --test` case feeding a readdir error asserts
  `continuity: unavailable` and no loss verdict.
- [x] 1.2 claude: a failed resume never reports `continued` — the retry
  path reports what happened, and a retry that finds no session fails as
  lost, not continued (C2, C3). Verify: `node --test` cases per outcome.
- [x] 1.3 ollama: a context-store error reports `unavailable`, never
  `continued` with the same handle (O1). Verify: `go test` case asserting
  the report against a failing store.
- [x] 1.4 copilot: any resume failure triggers the re-check, unreadable
  counts as present, and a failed retry is never reported `continued`
  (P2, P3). Verify: `node --test` cases for non-"session not found"
  failures and the failed-retry report.
- [x] 1.5 all three: `POST /work/done` stops retrying on a definitive 4xx
  (not 408/429) — logged with status and body, polling resumes. Transient
  failures keep bounded retry. Verify: unit test per runtime against a fake
  manager returning 400.
- [x] 1.6 all three: `exitCode` is omitted when no process ran (C6, O6,
  P6). Verify: unit tests assert the field is absent on prompt-read and
  spawn-error reports.

## 2. Runtime tooling, workspace and output fixes — backend-developer

- [x] 2.1 copilot: extract `<brief>` from the result into the report's
  `brief` field, like claude and ollama (P1). Verify: `node --test` asserts
  the field and that the block leaves `result`.
- [x] 2.2 ollama: parsed `mcpServer.Headers` are sent on the HTTP MCP
  transport (O2). Verify: `go test` against a fake server asserting the
  auth header arrives.
- [x] 2.3 ollama built-ins: Grep refuses a symlink escaping the workspace
  (O3), Bash captured output is bounded in memory (O4), a mid-string `*`
  pattern is logged as uninterpretable rather than "unavailable" (O5), an
  unset `${VAR}` in MCP config fails that server instead of expanding empty
  (O8), and the prompt-file/agent-name joins go through a safeJoin (O7).
  Verify: `go test` case per behaviour.
- [x] 2.4 all three: a failed clone fails the run with a reason, and
  `git fetch`/`reset` exit codes are checked (C5, O9, P7). Verify: unit
  test per runtime against a failing git invocation.
- [x] 2.5 claude logs every granted entry no vendor tool or connected MCP
  server provides (C4). copilot checks inventory per work unit, `mcp:`
  targets included (P4). Verify: `node --test` cases in both runtimes.

## 3. Telegram adapter contract fixes — backend-developer

- [x] 3.1 one retry budget per OPERATION — every call's wait, retry and
  HTTP timeout charges one total minted from `reclaimAfterSeconds` (A2).
  Verify: `go test` drives a multi-call op into the bound and asserts
  failure before the reclaim interval.
- [x] 3.2 dial errors, timeouts and 5xx are retried inside the budget (A1),
  close-topic riding the same rule with a completed op never re-attempted
  (A9). Verify: `go test` with injected transient failures.
- [x] 3.3 pacing charges every outbound Bot API call, `setMyCommands`
  included (A3). Verify: `go test` asserts token spend per call.
- [x] 3.4 `SendDocument` goes through the shared relay response path —
  `retry_after` honoured, a gateway rejection surfaced with status and
  body, general-topic fallback applied (A4). Verify: `go test` cases per
  response shape.
- [x] 3.5 a choice whose `callback_data` cannot fit degrades to text,
  never dropped (A5), and a topic `callback_query` is parsed and dispatched
  so selecting a choice sends the original message inbound (A6). Verify:
  `go test` render and dispatch cases.
- [x] 3.6 fold-order chunking — title and named sections lead chunk one
  when they fit, folded content never precedes them (A7), and the stale
  "no parsing happens here" comments go. Verify: `go test` chunking cases
  including the overflow scenario.
- [x] 3.7 `DeleteTopic` tolerates thread-not-found on redelivery (A10),
  and escaping is fixed — href-safe URLs, caption truncation before HTML
  rendering, rune-safe command descriptions (A11). Verify: `go test` case
  per edge.

## 4. Console adapter fixes — backend-developer

- [x] 4.1 the `channel:<name>` pseudo-thread is served by the transcript
  API as the channel's general feed (B1, server half). Verify: `go test`
  reads back a thread-less send.
- [x] 4.2 `delete-conversation` and `close-topic` without a thread id
  complete with an error instead of silent success (B3). Verify: `go test`
  asserts the error result.
- [x] 4.3 `ensure-topic` always answers `console-<uid>` — a cache miss
  resolves the UID or errors for redelivery, and the name-based fallback is
  deleted (B5). Verify: `go test` cache-miss case.
- [x] 4.4 vocabulary revision is read from the long-poll's
  `X-Agentops-Vocabulary-Revision` header and drives the refetch (B6).
  Verify: `go test` asserts a header change triggers it.
- [x] 4.5 the input-id parser anchors on the published shape by position
  (name segments cannot contain the separator) and cites the contract
  (B4). Verify: `go test` with an input id whose free segment contains the
  separator.

## 5. Console UI fixes — frontend-developer

- [x] 5.1 the SPA renders the channel general feed so thread-less sends
  and their choices are visible (B1, UI half). Verify: vitest case plus a
  dev-server screenshot of the feed, per the visual check.
- [x] 5.2 each message's choices render with that message, only the latest
  set interactive, superseded ones degrading to a visible list (B2).
  Verify: vitest case plus a screenshot showing an older message's choices.

## 6. Chart gate and docs tooling — deployment-engineer

- [x] 6.1 the telegram `configSchema` sets `additionalProperties: false`
  so a misspelled key reports on `ConfigValid` (A8). Verify: chart render
  test asserts the schema rejects a near-miss key.
- [x] 6.2 `docs-generate.py` renders the work-unit and report schema
  tables from the manager's Go structs under a declared marker in
  `docs/contracts/runtime.md`. Verify: the script suite covers the new
  generator and `--check` fails when a struct field moves.

## 7. Conformance suite additions — testing-specialist

- [x] 7.1 a send without `threadId` must end user-visible or complete with
  an error (pins B1). Verify: `go test -tags conformance` green for both
  adapters.
- [x] 7.2 `delete-conversation` acts or completes with an error, never a
  silent no-op (pins B3). Verify: `go test -tags conformance` green for
  both adapters.
- [x] 7.3 one operation's total in-process retry time stays below
  `reclaimAfterSeconds` under injected 429/5xx (pins A1, A2). Verify:
  `go test -tags conformance` green for both adapters.

## 8. Unit tests

- [x] 8.1 `go test ./...` green in every touched Go module —
  `platform/manager`, `runtimes/ollama`, `channels/telegram`,
  `platform/console`.
- [x] 8.2 `node --test` green in `runtimes/claude` and `runtimes/copilot`.
- [x] 8.3 console UI vitest suite green (`npm test` in
  `platform/console/ui`).
- [x] 8.4 chart render tests green, the new configSchema assertion
  included.
- [x] 8.5 the docs-script suite green for the `docs-generate.py` generator.
- [x] 8.6 `go test -tags conformance -count=1 ./test/conformance/` green
  with the three new assertions, run against this working copy's adapter
  binaries.

## 9. System tests

- [x] 9.1 extend the system pack's storage-outage coverage to assert the
  real claude runtime reports `unavailable` (never `continued`) through a
  failed resume, and run the pack (`go test -tags system`) against this
  working copy's tree — not the default branch's images.

## 10. Documentation

Reference docs:

- [x] 10.1 `docs/contracts/channel-adapter.md`: per-operation retry-budget
  wording, the published input id shape, ADA-11.1 citing
  `sharedOutputFormat` instead of the retired profile field (S2), and the
  version moved per its own versioning rule. Moved to 1.3.0. **S2 note:**
  `sharedOutputFormat` never existed in shipped code — the field has
  always been `AgentProfile.spec.outputFormat`, which ADA-11.1 already
  cited correctly. The stale name in
  `openspec/specs/adapter-rendered-messages/spec.md` is corrected in this
  change.
- [x] 10.2 `docs/contracts/runtime.md`: numbered conformance checklist,
  the generated schema tables under their marker, the startup-crash
  ruling, the `/work/done` definitive-rejection rows, and the version
  moved. Moved to 2.0.0 (BREAKING) — the maintainer's call on the review
  thread: API-10/11/12 constrain a `/work/done` 4xx response 1.0.0 left
  unconstrained, so a runtime conformant with 1.0.0 can stop conforming.
- [x] 10.3 the three runtime pages' spec-version claims match the pages'
  actual conformance after the fixes.
- [x] 10.4 `docs/CHANGELOG.md` entry for the adopter-observable behaviour
  fixes.
- [x] 10.5 record `telegramAdapter.enabled` in
  `.claude/rules/retired-vocabulary.md` and
  `.github/retired-vocabulary.json`, and run the guard clean. (Recorded in
  `.github/retired-vocabulary.json`, which is where every term actually
  lives — `retired-vocabulary.md` carries no per-term table to add to.)
- [x] 10.6 `python3 .github/scripts/docs-generate.py` rerun and `--check`
  clean.
- [x] 10.7 console UI changed, so rerun `npm run screenshots` AND
  `npm run demo` in `platform/console/ui`. **`npm run screenshots` done**
  (9 PNGs regenerated). **`npm run demo` done** (run on a workstation with
  docker, after the remote session that opened this PR had none) —
  `docs/assets/video/console-demo-{light,dark}.mp4` and their posters
  regenerated.

Adopter site:

- [x] 10.8 checked the landing page, introduction, getting-started,
  installation and guides for claims the fixes make untrue — "none
  found": existing mentions of continuity/thread ids/etc. are illustrative
  wire-format examples, not behavior claims, and the telegram/console
  pages already use the live `telegram.enabled` key.
