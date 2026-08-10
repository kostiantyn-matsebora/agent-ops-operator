## 1. Retention in the manager

- [ ] 1.1 Add retention config to the reconciler (`mode`: `off` | `age` | `immediate`, window, first-pass jitter) and wire it from env in `cmd/manager/main.go`
- [ ] 1.2 Implement the `finished` predicate in `internal/controller/conversation_controller.go` — phase `Idle`, no pending inputs, no inflight run, no runtime pod — reusing the existing `needsWorker` helpers rather than restating them
- [ ] 1.3 Implement the `delivered to every bound channel` predicate over `status.runs[].delivered[]`, treating a conversation with no bound channels as trivially delivered
- [ ] 1.4 Delete on expiry through the existing path (no second deletion route), so ownerRef GC and the close-topics finalizer both apply
- [ ] 1.5 Schedule expiry by requeue rather than by sweep: a finished conversation requeues for the moment it becomes eligible; add jitter so an install whose conversations are all eligible at startup does not expire them in one instant
- [ ] 1.6 Tests: `off` deletes nothing; `age` respects the window; a working/queued/inflight conversation survives its window; `immediate` waits for delivery; an undelivered run retains its conversation; deletion archives threads and GCs dependents

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
- [ ] 4.2 Add retention values (`retention.mode`, `retention.age`) with the reply-dead consequence of `immediate` documented AT the setting, not in a release note
- [ ] 4.3 Render the CronJob, its ServiceAccount and a read-only Role; name the workload `agentops-housekeeping` so the self-exclusion prefix applies
- [ ] 4.4 Mount both claims at their ROOT, and only when the corresponding persistence block is enabled
- [ ] 4.5 Add the render guard: fail if the housekeeping identity resolves to the runtime ServiceAccount (mirror the k8s-bundle MCP server guard)
- [ ] 4.6 Bump the chart version and add the `CHANGELOG.md` entry

## 5. Documentation

- [ ] 5.1 Document retention and the three modes in `docs/concepts.md`, next to conversation lifecycle
- [ ] 5.2 Document what reclaims what, and why the manager cannot, in the restart-resilience/storage section of `docs/concepts.md`
- [ ] 5.3 Record in `CLAUDE.md` the two rules that are easy to get wrong: scan-before-list for directories, and `immediate` requiring delivery

## 6. Verification

- [ ] 6.1 `go build ./... && go vet ./...` at the root and in every satellite module, including the new one
- [ ] 6.2 Full test run with `KUBEBUILDER_ASSETS` (unit + envtest)
- [ ] 6.3 Integration test: retention deletes a finished conversation and leaves a working one, through a real API server
- [ ] 6.4 Chart render tests: everything off by default, the runtime-SA guard fires, claims mount only when persistence is on
- [ ] 6.5 Live smoke on a cluster with persistence enabled: create a conversation, delete it, confirm the job reclaims exactly its directory and nothing else, first with dry run
- [ ] 6.6 Live smoke for `immediate`: confirm the reply lands on the bound thread before the conversation disappears
