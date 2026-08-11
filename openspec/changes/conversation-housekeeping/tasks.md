## 1. Autoclose in the manager

- [ ] 1.1 Add autoclose config to the reconciler (enabled flag defaulting to false, idle window, first-pass jitter) and wire it from env in `cmd/manager/main.go` as `CONVERSATION_RETENTION_ENABLED` / `CONVERSATION_RETENTION_AGE`
- [ ] 1.2 Implement the `finished` predicate in `internal/controller/conversation_controller.go` — phase `Idle`, no pending inputs, no inflight run, no runtime pod — reusing the existing `needsWorker` helpers rather than restating them
- [ ] 1.3 Add `delivered to every bound channel` to the same predicate, over `status.runs[].delivered[]`, treating a conversation with no bound channels as trivially delivered; it is an eligibility rule for every close, not a special case of one setting
- [ ] 1.4 Resolve LAST ACTIVITY as the window's origin — most recent run or input, falling back to creation only when the conversation never ran — and confirm it agrees with the `ageSeconds` the console already computes
- [ ] 1.5 Close on expiry via `chat.Router.CloseConversation` (never a bare `Delete`), passing a reason that names the automatic close and the elapsed window, so the farewell, ownerRef GC and the close-topics finalizer are the ones `/close` already uses
- [ ] 1.6 Schedule expiry by requeue rather than by sweep: a finished conversation requeues for the moment it becomes eligible; add jitter so an install whose conversations are all eligible at startup does not close them in one instant
- [ ] 1.7 Tests: disabled closes nothing; the window is respected; the window is measured from last activity, so a recently-active old conversation survives; a working/queued/inflight conversation survives its window; an undelivered run holds its conversation open; the close posts a farewell naming the window, archives threads and GCs dependents

## 2. The reclaiming workload

- [ ] 2.1 Create the `housekeeping/` module — dependency-free Go, in-cluster API over `net/http`, same technique as `signal-k8s-events`/`console`
- [ ] 2.2 Implement conversation listing (read-only; no API writes anywhere in the module) and session-id collection from `status.sessionId`
- [ ] 2.3 Implement workspace orphan reclamation in the mandated order — scan directory entries FIRST, list conversations SECOND, delete only entries absent from the later listing
- [ ] 2.4 Implement session-transcript reclamation — unreferenced session id AND file older than the grace period, both required
- [ ] 2.5 Add the per-run deletion bound and dry-run mode, reporting what was skipped and how much remains
- [ ] 2.6 Add the Dockerfile and a `go vet`/`go test` pass consistent with the other satellite modules
- [ ] 2.7 Tests: a conversation created mid-scan is never reclaimed; a deleted conversation's directory is; a transcript from a run in flight is kept; an old unreferenced transcript is removed; dry run deletes nothing; the bound is honored

## 3. Self-exclusion

- [ ] 3.1 Add `agentops-housekeeping-` to the name-prefix rule in `signal-k8s-events/selfexclude.go`
- [ ] 3.2 Test that a Warning event for a housekeeping pod is dropped by the prefix rule with a cold cache

## 4. Chart

- [ ] 4.1 Add the `housekeeping` values block (enabled, schedule, image, dryRun, maxDeletions, workspace/sessions toggles, session grace, resources, nodeSelector) — every component off by default
- [ ] 4.2 Add autoclose values (`retention.enabled: false`, `retention.age`) wired to the manager's env, with the consequence documented AT the setting rather than in a release note: closing a conversation removes the only durable copy of its result, so the window is "how long do I want to read this", not "how long until it is tidy"
- [ ] 4.3 Render the CronJob, its ServiceAccount and a read-only Role; name the workload `agentops-housekeeping` so the self-exclusion prefix applies
- [ ] 4.4 Mount both claims at their ROOT, and only when the corresponding persistence block is enabled
- [ ] 4.5 Add the render guard: fail if the housekeeping identity resolves to the runtime ServiceAccount (mirror the k8s-bundle MCP server guard)
- [ ] 4.6 Bump the chart version and add the `CHANGELOG.md` entry

## 5. Documentation

- [ ] 5.1 Document autoclose in `docs/concepts.md`, next to conversation lifecycle: the flag, the idle window, that the window is idle time and not lifetime, and that an automatic close announces itself
- [ ] 5.2 Document what reclaims what, and why the manager cannot, in the restart-resilience/storage section of `docs/concepts.md`
- [ ] 5.3 Record in `CLAUDE.md` the three rules that are easy to get wrong: scan-before-list for directories, delivery as an eligibility rule for every close, and that there is ONE close implementation — `/close`, the console batch and autoclose all route through `CloseConversation`

## 6. Verification

- [ ] 6.1 `go build ./... && go vet ./...` at the root and in every satellite module, including the new one
- [ ] 6.2 Full test run with `KUBEBUILDER_ASSETS` (unit + envtest)
- [ ] 6.3 Integration test: autoclose closes an idle finished conversation and leaves a working one, through a real API server
- [ ] 6.4 Chart render tests: everything off by default, the runtime-SA guard fires, claims mount only when persistence is on
- [ ] 6.5 Live smoke on a cluster with persistence enabled: create a conversation, delete it, confirm the job reclaims exactly its directory and nothing else, first with dry run
- [ ] 6.6 Live smoke for autoclose with a short window: confirm the farewell names the window, the topic is archived after it, and the result stayed readable in the console for the length of the window
