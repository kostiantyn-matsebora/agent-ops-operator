## 1. The API: a phase and a timestamp

- [ ] 1.1 Add phase `Closed` and `status.closedAt` to `api/v1alpha1` — status only; closing is something the manager DOES, never something a client asserts, so no spec field is added
- [ ] 1.2 Regenerate deepcopy and CRDs (`controller-gen object` + `crd`), and check the CRD YAML into `chart/files/crds/`

## 2. Closing stops deleting

- [ ] 2.1 Change the ONE close implementation's final step from `Delete` to a status write: phase `Closed`, `status.closedAt` stamped — every originator (`/close`, console batch, the timer) keeps calling it unchanged
- [ ] 2.2 Move the `close-topic` enqueue from the deletion finalizer to the close TRANSITION, one op per bound thread, with a "topics archived" marker on the conversation so the ops become re-derivable after a restart instead of staying the one non-derivable kind
- [ ] 2.3 Keep the `agentops.dev/close-topics` finalizer as the guard for a direct `kubectl delete` of a conversation nobody closed; deleting an already-`Closed` conversation finds its threads archived and releases immediately
- [ ] 2.4 Tear down the runtime pod and the MCP ConfigMap at the close, and release capacity so a `Pending` conversation is admitted — unchanged in effect, moved to the transition
- [ ] 2.5 Tests: `/close` leaves a `Closed` conversation rather than deleting it; topics are archived at the close; a direct `kubectl delete` of a never-closed conversation still archives through the finalizer; deleting a `Closed` one does not archive twice

## 3. A closed conversation is inert

- [ ] 3.1 Skip `Closed` in dispatch — no work units, ever
- [ ] 3.2 Skip `Closed` in the admission gate: not counted active (it has no pod, which already holds) and not a member of the FIFO waiting set, so it can neither consume capacity nor starve a `Pending` conversation
- [ ] 3.3 Skip `Closed` in conversation REUSE in `internal/ingest` — a matching signature opens a NEW conversation; this is the rule that makes closing mean anything
- [ ] 3.4 Skip `Closed` wherever a Pipeline resolves conversations, so a closed conversation is not somewhere work can land
- [ ] 3.5 Tests: a signal matching a closed conversation's signature opens a new one; a closed conversation receives no dispatch; a closed conversation does not occupy a capacity slot

## 4. Reopening

- [ ] 4.1 Implement reopen: phase → `Idle`, `status.closedAt` cleared, materialized refs left EXACTLY as they are — no re-resolution, or a Pipeline edit re-wires an existing conversation
- [ ] 4.2 Validate the refs at reopen and fail naming the missing one; never partially reopen and never silently drop a binding
- [ ] 4.3 Add the optional previous-thread-id hint to `ensure-topic` and enqueue one per bound channel on reopen; update `status.threads[]` from whatever the adapter returns, same as today
- [ ] 4.4 Honour the hint in `channel-telegram` by un-archiving the topic and returning the same thread id; confirm an adapter that ignores the hint stays correct
- [ ] 4.5 Tests: reopen restores wiring and runs unchanged; reopen with a deleted profile fails naming it; the hint honoured returns the same thread and ignored returns a new one; a two-channel reopen records one continued and one fresh thread

## 5. The two timers

- [ ] 5.1 Add both config blocks to the reconciler and wire them from env in `cmd/manager/main.go`: `CONVERSATION_AUTOCLOSE_ENABLED` / `CONVERSATION_AUTOCLOSE_IDLE_AGE`, `CONVERSATION_AUTODELETE_ENABLED` / `CONVERSATION_AUTODELETE_CLOSED_AGE` — both flags default false, and enabling one must not enable the other
- [ ] 5.2 Implement the `finished` predicate — phase `Idle`, no pending inputs, no inflight run, no runtime pod — reusing the existing `needsWorker` helpers rather than restating them
- [ ] 5.3 Add `delivered to every bound channel` to that predicate, over `status.runs[].delivered[]`, treating a conversation with no bound channels as trivially delivered; it gates CLOSING only, because it is about a message reaching a thread
- [ ] 5.4 Resolve LAST ACTIVITY as the close window's origin — most recent run or input, falling back to creation only when the conversation never ran — and confirm it agrees with the `ageSeconds` the console already computes
- [ ] 5.5 Close on expiry through the ONE close implementation, passing a reason that names the automatic close, the elapsed window AND that the conversation can be reopened
- [ ] 5.6 Delete on expiry of `closedAge` measured from `status.closedAt`, applying to `Closed` conversations only; a reopen clears the timestamp and therefore the clock
- [ ] 5.7 Schedule both by requeue rather than by sweep, with jitter on each timer's first pass so an install whose conversations are all eligible at startup does not act on them in one instant
- [ ] 5.8 Tests: both disabled do nothing; each window respected; the close window measured from last activity so a recently-active old conversation survives; working/queued/inflight survives its window; an undelivered run holds a conversation open; a never-closed conversation is never auto-deleted however idle; reopening before the delete window prevents the delete

## 6. Delete and reopen as manager verbs

- [ ] 6.1 Add manager-side delete and reopen verbs reachable over the console's existing authenticated, gated, attributed write path — the console gains NO Kubernetes write
- [ ] 6.2 Bound reach by the BINDING: a surface may act on a conversation whose `spec.channelRefs` names its channel, read from the conversation and never taken from the request
- [ ] 6.3 Refuse delete on anything not already `Closed`, with a reason naming the missing step — never close-then-delete in one call
- [ ] 6.4 Tests: a surface with no binding is refused with a reason naming the binding; a live conversation named for deletion is refused, not closed; an unattributed or read-only request is refused

## 7. Console

- [ ] 7.1 Present `Closed` as a state rather than an absence: closed rows listed, labelled, and distinguished from a conversation held by its finalizer
- [ ] 7.2 Add per-row reopen; no bulk reopen — a batch would re-materialise threads on surfaces nobody is watching
- [ ] 7.3 Add bulk delete beside bulk close, same 50-name bound, same explicit selection, same per-item outcomes (`deleted` / `skipped` / `failed`) and totals; a non-closed name is `skipped` with "close it first"
- [ ] 7.4 Confirmation for delete names the count AND that the conversation's recorded results and volume state both go, and that it cannot be undone
- [ ] 7.5 Tests: closed rows render and offer reopen; a mixed delete batch reports per-item outcomes; a live conversation in a delete batch is skipped; the bound is server-enforced

## 8. The reclaiming workload

- [ ] 8.1 Create the `housekeeping/` module — dependency-free Go, in-cluster API over `net/http`, same technique as `signal-k8s-events`/`console`
- [ ] 8.2 Implement conversation listing (read-only; no API writes anywhere in the module) and context-handle collection — the listing MUST be phase-blind, or a closed conversation's state is reclaimed out from under a reopen
- [ ] 8.3 Implement workspace orphan reclamation in the mandated order — scan directory entries FIRST, list conversations SECOND, delete only entries absent from the later listing
- [ ] 8.4 Implement session-transcript reclamation — unreferenced session id AND file older than the grace period, both required
- [ ] 8.5 Add the per-run deletion bound and dry-run mode, reporting what was skipped and how much remains
- [ ] 8.6 Add the Dockerfile and a `go vet`/`go test` pass consistent with the other satellite modules
- [ ] 8.7 Tests: a conversation created mid-scan is never reclaimed; a deleted conversation's directory is; **a CLOSED conversation's directory and transcripts survive a run**; a transcript from a run in flight is kept; an old unreferenced transcript is removed; dry run deletes nothing; the bound is honored

## 9. Self-exclusion

- [ ] 9.1 Add `agentops-housekeeping-` to the name-prefix rule in `signal-k8s-events/selfexclude.go`
- [ ] 9.2 Test that a Warning event for a housekeeping pod is dropped by the prefix rule with a cold cache

## 10. Chart

- [ ] 10.1 Add both retention blocks wired to the manager's env — `retention.autoclose.{enabled,idleAge}` and `retention.autodelete.{enabled,closedAge}`, both flags false
- [ ] 10.2 Document the consequence AT the delete setting rather than in a release note: deleting removes the only durable copy of the result, so the window is "how long do I want to be able to read this", not "how long until it is tidy"
- [ ] 10.3 Note at the same setting that enabling autodelete without the housekeeping job reclaims the API half and leaves the disk — correct with persistence off, a silent leak with it on
- [ ] 10.4 Add the `housekeeping` values block (enabled, schedule, image, dryRun, maxDeletions, workspace/sessions toggles, session grace, resources, nodeSelector) — every component off by default
- [ ] 10.5 Render the CronJob, its ServiceAccount and a read-only Role; name the workload `agentops-housekeeping` so the self-exclusion prefix applies
- [ ] 10.6 Mount both claims at their ROOT, and only when the corresponding persistence block is enabled
- [ ] 10.7 Add the render guard: fail if the housekeeping identity resolves to the runtime ServiceAccount (mirror the k8s-bundle MCP server guard)
- [ ] 10.8 Bump the chart version and add the `CHANGELOG.md` entry — closing no longer deletes is a BEHAVIOURAL BREAK: after upgrade, closed conversations remain as `Closed` rows, and the old semantics are autodelete with a short `closedAge`

## 11. Documentation

- [ ] 11.1 Document the two-stage lifecycle in `docs/concepts.md` next to conversation lifecycle: the two flags, the two windows and what each is measured from, what `Closed` means exhaustively, and that reopening restores wiring without re-resolving it
- [ ] 11.2 Document what reclaims what, and why the manager cannot, in the restart-resilience/storage section of `docs/concepts.md`
- [ ] 11.3 Document the `ensure-topic` hint and the two console-reachable verbs in `docs/contracts.md`, including that ignoring the hint is a valid implementation
- [ ] 11.4 Document `Closed` rows, reopen and bulk delete in `docs/console.md`, and that the console still holds no Kubernetes write path
- [ ] 11.5 Record in `CLAUDE.md` the rules that are easy to get wrong: closing sets a phase and deletion is a second verb; a closed conversation is excluded from reuse and from every pipeline; the reclaiming job's listing is PHASE-BLIND on purpose; reopen never re-resolves refs; delete/reopen reach is the BINDING, which is what the retired no-remote-close-verb rule was actually protecting
- [ ] 11.6 Retire the "`close-topic` is the ONE op not derivable from CR state" clause in `CLAUDE.md` and `docs/concepts.md`, replacing it with why it WAS the exception (it was enqueued while the object was disappearing) and why it no longer is (the object survives the close, so an unarchived thread is still readable) — leaving the clause standing would send the next reader looking for an exception the code no longer has

## 12. Verification

- [ ] 12.1 `go build ./... && go vet ./...` at the root and in every satellite module, including the new one
- [ ] 12.2 Full test run with `KUBEBUILDER_ASSETS` (unit + envtest)
- [ ] 12.3 Integration test through a real API server: autoclose closes an idle finished conversation and leaves a working one; the closed one is skipped by reuse; reopen restores it; autodelete removes it only after `closedAge` from the close
- [ ] 12.4 Chart render tests: both flags off by default, the runtime-SA guard fires, claims mount only when persistence is on
- [ ] 12.5 Live smoke on a cluster with persistence enabled: close a conversation, confirm its directory SURVIVES a dry-run and a real housekeeping run, reopen it and confirm the agent resumes with its workspace
- [ ] 12.6 Live smoke for the full lifecycle with short windows: the farewell names the window and says it can be reopened, the topic is archived, the conversation stays readable in the console, and only after `closedAge` do the object and then the directory go
