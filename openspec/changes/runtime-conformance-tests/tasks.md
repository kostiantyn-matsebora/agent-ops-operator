## 1. `FakeManager` serves the work contract — testing-specialist

- [ ] 1.1 Add `GET /work` and `POST /work/done` handlers to
      `platform/manager/test/conformance/fakemanager.go`, mirroring
      `handleOps`/`handleDone`. Verify with a direct HTTP client in a new
      `fakemanager_test.go` case: a queued unit is returned once, a second
      poll while inflight gets nothing.
- [ ] 1.2 Add `QueueWork`, `WorkRequests()` and `Completions()`-style
      accessors (naming TBD at review, matching `QueueOp`/`OpsRequests`'s
      shape) plus a `RejectWorkDone(code int)` toggle, mirroring
      `RejectInbound`. Verify the same way, by direct HTTP calls against
      the running `FakeManager` in a unit test.

## 2. The fake Ollama double — testing-specialist

- [ ] 2.1 Add an in-package `httptest.Server` double for the `OLLAMA_URL`
      endpoint shape `runtimes/ollama/work.go` and `ollama.go` call,
      returning the design's default scripted completion. Verify by
      pointing a bare `http.Client` at it in a unit test and reading back
      the fixed completion.
- [ ] 2.2 Add the `slow` and `unparsable` directives from `design.md`'s
      vocabulary table, keyed on the first word of the prompt the double
      receives. Verify each directive's behavior in the same unit test
      file, one case per directive.

## 3. Runtime conformance assertions — testing-specialist

`API-3` is not assigned a task below. It obligates whoever answers
`GET /work` — the manager, not the runtime under test — so it joins the
`API-6`/`MGR-n` exclusion `design.md` and the delta spec both state, and
stays owned by envtest/system.

- [ ] 3.1 New `platform/manager/test/conformance/runtime_test.go`
      (`//go:build conformance`). Build `runtimes/ollama` with the
      existing `build()` helper, start it against `FakeManager` and the
      fake Ollama double, and assert `API-1` (long-poll, `204` is not an
      error), `LC-1` (one inflight unit — a second poll while Working gets
      nothing) and `RT-2` (the subprocess's captured stdout is non-empty
      once a run completes).
- [ ] 3.2 Assert `API-2` (exactly one of `promptFile`/`promptText` in the
      served unit), `API-7` (a fact the runtime cannot determine is
      omitted, never zero) and `API-9` (neither `turns[]` nor
      `toolCalls[]` reported when the run made neither a model nor a tool
      call) against the completion report `FakeManager` records.
- [ ] 3.3 Assert `API-4` (`turns[]`/`toolCalls[]` trimmed to the documented
      bounds before posting) and `API-8` (no tool input or output
      reported) by driving the `calls`-shaped case through the fake Ollama
      double and reading the posted report.
- [ ] 3.4 Assert `API-5` — `runtimeContextId`/`brief` are latest-wins, a
      second run that omits them leaves the stored value untouched (read
      back through `FakeManager`'s own state, exactly as
      `signal_test.go`'s cursor assertion does).
- [ ] 3.5 Assert `LC-2` (exit after `RUNTIME_IDLE_TTL_M` minutes with no
      work, set short for the test) and `LC-3` (`/exit` refused while a
      run is in progress — this suite scripts that through the manager
      side the way `channel_test.go` does for its own lifecycle calls).
- [ ] 3.6 Assert `RT-1` (clones `REPO_URL`/`REPO_REF` when set, against a
      local bare repository fixture) and `RT-3`/`3.1`–`3.4` (allowlist
      composition: `merge` unions with position kept, `overwrite` takes
      `allowedTools` alone, an empty allowlist stays empty, an absent or
      unreadable agent definition contributes nothing).
- [ ] 3.7 Assert `SEC-1`–`SEC-3` (no secret value appears in anything the
      runtime logs or reports) and `COMPAT-1` (an unknown work-unit field
      is ignored, not rejected) by adding an undocumented field to a served
      unit and confirming the run still succeeds.

## 4. Unit tests

- [ ] 4.1 `cd platform/manager && go test ./...` passes, covering the
      `FakeManager` additions and the fake Ollama double's own unit tests
      from tasks 1 and 2, run in this worktree.
- [ ] 4.2 `cd platform/manager && go test -tags conformance -count=1
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
