## 1. Low-risk single-finding files (design: risk-ordered, cheapest first)

- [ ] 1.1 `internal/activity/log.go` — extract the flagged function's
      control-flow body into a named top-level helper. Verify: the file's
      own package tests plus `go build ./... && go vet ./...` (default,
      `conformance`, `e2e` tags) and `go test ./...` all green; record which
      function and helper name.
- [ ] 1.2 `internal/mcpcompile/compile.go` — same. Verify: same as 1.1, plus
      `internal/mcpcompile`'s own test package if one exists.
- [ ] 1.3 `internal/dispatch/dispatch.go` — same. Verify: same as 1.1, plus
      `internal/dispatch`'s own test package.
- [ ] 1.4 `internal/runtimepod/podspec.go` — 2 findings, extracted together
      in one pass since they're in the same file. Verify: same as 1.1, plus
      `internal/runtimepod`'s own test package (pod-spec rendering tests
      exist — assert the rendered spec is byte-identical before/after for a
      representative case).

## 2. Multi-finding files (design: one task per file)

- [ ] 2.1 `internal/controller/pipeline_controller.go` — 1 finding.
      Verify: same shape as 1.1, plus the pipeline reconciler's envtest
      suite.
- [ ] 2.2 `internal/chat/ops.go`, `internal/chat/delivery.go`,
      `internal/chat/pipelines.go` — 4 findings across these three files.
      Verify: same shape as 1.1, plus `internal/chat`'s own test package,
      run individually per file to isolate which extraction (if any) needs
      a fix before moving to the next.
- [ ] 2.3 `internal/httpapi/server.go`, `signals.go`, `channelread.go`,
      `status.go` — 9 findings across these four files. Verify: same shape,
      plus `internal/httpapi`'s own test package (including the adapter
      contract tests) and the conformance suite, since this package serves
      `/channel/*` and `/signal/*`.
- [ ] 2.4 `internal/controller/conversation_controller.go` — 7 findings, one
      at complexity 61. May take more than one extraction pass per finding
      per design.md - Risks; if a function still reads high after a
      reasonable split, record that honestly rather than forcing an
      artificial one-shot fix. Verify: the conversation reconciler's full
      envtest suite (the largest in the module) plus `-tags e2e` against a
      real cluster if a lane touches the changed logic, per
      change-tests.md's own e2e criterion (a reconciler write is exactly
      what envtest can decide — confirm nothing here also needs a cluster
      lane before ticking task 5.1 as not-applicable).

## 3. Re-verification (design: carries sonar-ratings-baseline task 2.2)

- [ ] 3.1 After every file above lands, re-run the finding count for
      `manager` against a live SonarCloud analysis (`sonar-findings-baseline.py`
      or the `get_component_measures`/`search_sonar_issues_in_projects` MCP
      reads `sonar-ratings-baseline` used) and record whether `go:S3776`
      reads zero for `manager`, or exactly what remains and why — same
      honesty convention as the parent change's own tasks.md. Verify: the
      recorded count is read from a live analysis of the actual commit that
      landed, not assumed from the local fix list.

## 4. Unit tests

- [ ] 4.1 `go test ./...` (envtest included) green for the whole
      `platform/manager` module after all extractions land, not just
      per-file. Verify: full run, not the per-task partial runs above.

## 5. E2E tests

- [ ] 5.1 State per file above whether its reconciler/dispatch/httpapi
      change is decided by a cluster (RBAC, informer liveness, a real pod
      lifecycle) and needs a `-tags e2e` lane, or whether envtest already
      covers it — do not default this to "not applicable" without checking
      each file, since `conversation_controller.go` and `httpapi` both sit
      on paths this repo's own e2e pack exercises.

## 6. Documentation

### 6.1 Reference docs

- [ ] 6.1.1 Confirm no reference doc (`docs/concepts.md`, `docs/contracts.md`,
      `docs/cr-reference.md`) names one of the 26 extracted functions by
      name or describes its internal shape; if one does, update it. If none
      do (expected, per proposal.md - Impact), tick this stating so.

### 6.2 Adopter site

- [ ] 6.2.1 None of the site's pages describe internal function shape, so
      nothing there is made untrue by this change. No edit needed.
