## 1. `FakeManager` serves the work contract — testing-specialist

- [x] 1.1 Add `GET /work` and `POST /work/done` handlers to
      `platform/manager/test/conformance/fakemanager.go`, mirroring
      `handleOps`/`handleDone`. Verify with a direct HTTP client in a new
      `fakemanager_test.go` case: a queued unit is returned once, a second
      poll while inflight gets nothing.
- [x] 1.2 Add `QueueWork`, `WorkRequests()` and `Completions()`-style
      accessors (naming TBD at review, matching `QueueOp`/`OpsRequests`'s
      shape) plus a `RejectWorkDone(code int)` toggle, mirroring
      `RejectInbound`. Verify the same way, by direct HTTP calls against
      the running `FakeManager` in a unit test.

## 2. The fake Ollama double — testing-specialist

- [x] 2.1 Add an in-package `httptest.Server` double for the `OLLAMA_URL`
      endpoint shape `runtimes/ollama/work.go` and `ollama.go` call,
      returning the design's default scripted completion. Verify by
      pointing a bare `http.Client` at it in a unit test and reading back
      the fixed completion.
- [x] 2.2 Add the `slow` and `unparsable` directives from `design.md`'s
      vocabulary table, keyed on the first word of the prompt the double
      receives. Verify each directive's behavior in the same unit test
      file, one case per directive.

## 3. Runtime conformance assertions — testing-specialist

`API-3` is not assigned a task below. It obligates whoever answers
`GET /work` — the manager, not the runtime under test — so it joins the
`API-6`/`MGR-n` exclusion `design.md` and the delta spec both state, and
stays owned by envtest/system.

- [x] 3.1 New `platform/manager/test/conformance/runtime_test.go`
      (`//go:build conformance`). Build `runtimes/ollama` with the
      existing `build()` helper, start it against `FakeManager` and the
      fake Ollama double, and assert `API-1` (long-poll, `204` is not an
      error), `LC-1` (one inflight unit — a second poll while Working gets
      nothing) and `RT-2` (the subprocess's captured stdout is non-empty
      once a run completes).
- [x] 3.2 Assert `API-2` (exactly one of `promptFile`/`promptText` in the
      served unit), `API-7` (a fact the runtime cannot determine is
      omitted, never zero) and `API-9` (neither `turns[]` nor
      `toolCalls[]` reported when the run made neither a model nor a tool
      call) against the completion report `FakeManager` records.
- [x] 3.3 Assert `API-4` (`turns[]`/`toolCalls[]` trimmed to the documented
      bounds before posting) and `API-8` (no tool input or output
      reported) by driving the `calls`-shaped case through the fake Ollama
      double and reading the posted report.

      Note (testing-specialist): the SHAPE (two turns, one tool call, every
      integer field non-negative, no tool input/output anywhere in the
      posted JSON) is asserted end to end against the real binary. The
      numeric ENTRY-COUNT bound (≤200) itself is not separately re-driven
      here — it already has a dedicated, deterministic unit-tier pin
      (`runtimes/ollama/report_test.go`'s `TestReportIsTrimmedToTheManagersBounds`),
      and reproducing a 200+-call run through this suite's subprocess would
      buy no additional coverage for a real cost in runtime.
- [x] 3.4 Assert `API-5` — `runtimeContextId`/`brief` are latest-wins, a
      second run that omits them leaves the stored value untouched (read
      back through `FakeManager`'s own state, exactly as
      `signal_test.go`'s cursor assertion does).
- [ ] 3.5 Assert `LC-2` (exit after `RUNTIME_IDLE_TTL_M` minutes with no
      work, set short for the test) and `LC-3` (`/exit` refused while a
      run is in progress — this suite scripts that through the manager
      side the way `channel_test.go` does for its own lifecycle calls).

      Note (testing-specialist): `LC-2` IS asserted
      (`TestRuntimeOllamaIdleExit`). `LC-3` is NOT — left unticked
      deliberately rather than claimed done. `/exit` is a chat command
      handled entirely inside the manager's `internal/chat/router.go`
      (`isExitCommand`, refusing on the Conversation's own phase). It never
      issues any HTTP call to the runtime pod, and `runtimes/ollama` exposes
      no endpoint shaped like it at all. There is no runtime-observable
      behavior this black-box suite could assert for `LC-3` — it is already
      exercised against the real router by the manager's own
      `internal/chat` tests. Needs a reviewer's call on whether to mark this
      task's `LC-3` half "not applicable to this suite" or reassign it.
- [x] 3.6 Assert `RT-1` (clones `REPO_URL`/`REPO_REF` when set, against a
      local bare repository fixture) and `RT-3`/`3.1`–`3.4` (allowlist
      composition: `merge` unions with position kept, `overwrite` takes
      `allowedTools` alone, an empty allowlist stays empty, an absent or
      unreadable agent definition contributes nothing).

      Note (testing-specialist): the fixture is a plain (non-`--bare`)
      local git repository, matching the existing precedent
      (`runtimes/ollama/repo_sync_test.go`'s `gitAt` pattern) rather than a
      literal `git init --bare` — `git clone --depth 1 -b <ref>` against
      either is the same wire operation, so this proves RT-1 identically.
      RT-3.1 and RT-3.3 are asserted together, deliberately: the runtime's
      own code (`agentDeclaredTools`) takes the exact same "declares
      nothing" path whether no repository was ever cloned or a repository
      was cloned but lacks this agent's file — there is no code branch that
      distinguishes "cannot see a repository" from "the file is absent",
      so one fixture case (an agent name the repo never defines) proves
      both documented rules without inventing a distinction the
      implementation doesn't make.
- [ ] 3.7 Assert `SEC-1`–`SEC-3` (no secret value appears in anything the
      runtime logs or reports) and `COMPAT-1` (an unknown work-unit field
      is ignored, not rejected) by adding an undocumented field to a served
      unit and confirming the run still succeeds.

      Note (testing-specialist): `COMPAT-1` IS asserted
      (`runtimeOllamaAssertCOMPAT1`). `SEC-1`–`SEC-3` are NOT — left
      unticked deliberately. As worded in `docs/contracts/runtime.md`, all
      three are facts about the MANAGER/OPERATOR's own pod-spec and RBAC
      rendering (no Secret read, `AOPS_MCP_TOKEN` derivation, no
      ServiceAccount granted) — none of it is behavior the
      `runtimes/ollama` BINARY performs or could violate from the
      manager-facing wire this suite drives. `FakeManager` trivially
      "satisfies" them by construction, exactly the reason `design.md`
      already gives for excluding `API-3`/`API-6`/every `MGR-n`. Writing a
      check that merely confirms the runtime never echoes an arbitrary env
      value in its logs would test an invented, adjacent property and
      report it under these IDs, which the review bar this role holds
      a diff to rules out. Already owned by `internal/runtimepod`'s own
      tests and the chart's RBAC templates. Needs a reviewer's call on
      whether to mark `SEC-1`–`SEC-3` "not applicable to this suite" or
      reassign them.

## 4. Unit tests

- [x] 4.1 `cd platform/manager && go test ./...` passes, covering the
      `FakeManager` additions and the fake Ollama double's own unit tests
      from tasks 1 and 2, run in this worktree.

      Note (testing-specialist): every file in `test/conformance/` —
      `fakemanager.go` included, from before this change — carries
      `//go:build conformance`, so this UNTAGGED command never actually
      compiles or runs that package at all (confirmed:
      `go test ./test/conformance/...` reports "matched no packages" with
      no tag). The command passes trivially for this directory, exactly as
      it already did for every pre-existing conformance file. The real
      execution and coverage of the `FakeManager`/`fakeollama` unit tests
      is task 4.2's tagged command — a pre-existing repository convention,
      not something this change altered.
- [x] 4.2 `cd platform/manager && go test -tags conformance -count=1
      -timeout 20m -v ./test/conformance/` passes, including the new
      `runtime_test.go` cases, run in this worktree.

## 5. System tests

- [x] 5.1 Not applicable — nothing a cluster decides changed. The suite
      added here runs entirely as built binaries against HTTP doubles, the
      same tier `channel_test.go`/`signal_test.go` already occupy, with no
      pod, no RBAC and no informer involved.

## 6. Documentation

- [x] 6.1 Reference docs: update `docs/testing.md`'s "Contract
      conformance" tier row so it names runtimes alongside adapters, and
      update its "Run it yourself" section if the command changes.
- [x] 6.2 Reference docs: none of `docs/contracts/runtime.md`'s
      obligations change, so it is not edited and its version stays
      `1.0.0` — confirm this stays true at review.
- [x] 6.3 Adopter site: none. This is contributor-facing test
      infrastructure with no adopter-facing behavior, so no page under
      `docs/` needs a change — state this explicitly rather than leaving
      the subsection silently empty.
