## 1. Spike the transport — backend-developer

- [x] 1.1 Against the test-only `nats-server` the state backend already vendors, show that a publish returns only once JetStream has stored it and that a repeated id is suppressed.
- [x] 1.2 Show that a claim made through one connection completes through another and that a lapsed lease returns the task (design D-3, check 2).
- [x] 1.3 Show delayed redelivery with a bounded attempt count ending in a dead record.
- [x] 1.4 Build the candidate library's required `nats.go` version into the manager module and run the state backend's conformance suite and bench unchanged against it.
- [x] 1.5 Record the verdict in the task, numbers included. Choose the library under the transport or `nats.go` KV directly, per design D-3. Delete the spike code.

**Verdict: `nats.go` KV directly, not Watermill-as-transport.** The candidate
(`github.com/ThreeDotsLabs/watermill-nats/v2` v2.2.0, JetStream module)
cleared checks 1-3 against an in-process `nats-server` v2.10.26. Check 4
failed as stated, and the weight it adds exceeds what it saves once D-2
already owns the lease.

| Check | Measured |
|---|---|
| 1.1 publish ack-on-store, dedup | First publish returned in 212µs, stream already at 1 message. A repeated command id raised no error, stream stayed at 1 message |
| 1.2 cross-connection claim, lease lapse | Connection A claimed the task, never acked. Connection B, same durable consumer, received the redelivery at +1.40s against a 700ms lease, acked it. `NumAckPending` reached 0 |
| 1.3 delayed bounded retry to dead | Three Naks at +0ms/+151ms/+302ms. The library's own `NakDelay`/`MaxRetryDelay` termed the message on the third. No further redelivery. `NumAckPending`/`NumPending` both 0 |
| 1.4 `nats.go` 1.52.0 vs the state suite | Broke `scriptedKV.Create` in `internal/state/nats/failure_paths_test.go` (`jetstream.KeyValue.Create` gained a variadic `opts` param). Patched, the suite passed (8.04s vs 8.12s baseline). Bench stayed inside n=3 sample noise: Conversations 931µs→728µs, Runs 2.11ms→2.32ms |

Why the fallback, past check 4's own failure:

- The `jetstream` package still calls itself beta and tells production callers
  to use its ack-based `pkg/nats` subscriber instead.
- `Publisher.Publish` sends to `configureStream(topic).Name`, not the topic
  argument. A stream name can't hold the dots the queue inventory's own names
  (`in.signal`, `out.<type>`) use.
- Its lease is the message ack, the shape D-2 rejects for cross-replica
  completion. The ledger's CAS-over-KV claim and watch feed still have to be
  hand-written against `nats.go` regardless of the library.
- Adopting it forces `nats.go` from 1.39.1 to 1.52.0 for a thin wrapper
  around `nats.go`'s own JetStream client, for a dependency tree task 3
  does not otherwise need.

`go.mod`/`go.sum` end this task as they started. The `nats` backend (task 3)
builds directly on `nats.go` JetStream KV, its already-assumed fallback
shape. The spike (a throwaway module outside the repo) is deleted.

## 2. Contract, `memory` backend, conformance suite — backend-developer

- [x] 2.1 Define the contract in `platform/manager/internal/commandbus/`: submit, claim with lease, settle, fail with a code, task and dead-letter reads and feeds, per-queue statistics.
- [x] 2.2 Define the closed queue inventory and its declarations: key, attempts, backoff, TTL, dedup window, park cool-off, per-invocation in-flight cap. A kind may override its queue's attempts and backoff.
- [x] 2.3 Define the error-code vocabulary and derive retryability from the code. Record an unknown code as `internal`.
- [x] 2.4 Write the `memory` backend.
- [x] 2.5 Write the conformance suite and bench, walked once per queue in the inventory, failing and naming a queue left unwired.
- [x] 2.6 Conformance covers: durable accept, idempotent submit, dedup across every state, park then release, FIFO with a retry keeping its place, serialization per key under parallel consumers, lease completion through a second handle, lease-lapse redelivery, bounded retry into dead, TTL expiry counting as settled, a gap-free resumable feed, dead-letter listing, per-queue statistics.

## 3. `nats` backend — backend-developer

- [x] 3.1 Implement the ledger over JetStream KV buckets (`tasks`, `ids`, `dead`), each created with the configured replica count and a prefix of its own.
- [x] 3.2 Claim by compare-and-swap on the task revision, with the claim-time ordering rule over the ordered feed (design D-2).
- [x] 3.3 A watch-fed local mirror for pending and settled ids, so reconcile never reads the server.
- [x] 3.4 Sweeps: parked-id release, TTL expiry, ledger and dead-letter retention caps.
- [x] 3.5 Pass the conformance suite and bench against the in-process server.
- [x] 3.6 Extend the static import test to name `internal/state/nats` and `commandbus/nats` as the only importers of `nats.go`.

## 4. Manager wiring — backend-developer

- [x] 4.1 Select the backend from `COMMANDBUS_BACKEND`, reading `COMMANDBUS_NATS_URL`, `COMMANDBUS_NATS_USER` and `COMMANDBUS_NATS_PASSWORD`. Never read `NATS_*`.
- [x] 4.2 Run the consumers on every replica, one pool per queue, with a configurable concurrency.
- [x] 4.3 Report not ready while the bus is unreachable.
- [x] 4.4 Refuse `memory` with more than one replica.

## 5. Accept and process — backend-developer

- [ ] 5.1 Build the accept step: authentication, scope, shape, unknown channel or source, signal Wired and filter reasons, the receipt, the move stamp, the thread-to-conversation lookup, then mint `invocationId`, enqueue, reply `202 {invocationId, taskIds}` additively.
  - DONE for `reopen`, `delete`, `reset-context` and `/channel/inbound`.
    The thread lookup, self-input and closed-thread refusals run at accept with no task.
  - DONE for `/coordinate/close` and `/coordinate/escalate`.
    The move stamp runs at accept, on a successful `Submit` (see 5.2 and 5.5).
    The reply stays `200` for these two, per the proposal. Every other converted verb replies `202`.
  - NOT done for `/signal/inbound`.
    There is no Wired, filter-reason or receipt change. Publish stays synchronous (see 5.6's note).
- [x] 5.2 Move `close` (console single and bulk, chat, coordinated, the idle timer, a member's `escalate`, budget-exceeded), `exit`, `reopen`, `delete` and `reset-context` into handlers. Keep immediate refusals that need only a cheap read, such as `delete` of a conversation not Closed.
  DONE: chat `/close`/`/exit` (via the inbound-message task), the idle timer, budget-exceeded (both call sites — the reconciler's deadline edge and `handleWorkDone`'s maxTurns edge), `reopen`, `delete`, `reset-context`, and now `coordinated` close (`/coordinate/close` → `VerbCloseCoordinated`/`taskCloseCoordinated`) and a member's `escalate` (`/coordinate/escalate` → `VerbEscalate`/`taskEscalate`). "Console bulk" is confirmed to be N individual chat `/close` messages (no bulk HTTP endpoint exists), so it is already covered by the chat path. `/coordinate/close`'s own widened-scope check (one-hop membership, the Coordinator-owner sibling-root widening, `isHumanInitiated`) is NOT a cheap read in the "immediate refusal" sense this task names — it stays deferred into `taskCloseCoordinated`, which re-derives it fresh via the unchanged `Router.CloseCoordinated`. Only target-defaults-to-caller (a string default) stays at accept.
- [ ] 5.3 Enqueue a cascade at accept, members before the parent on the root's key. The ancestor re-checks its descendants at run time and closes any open one or fails `precondition_failed`.
  - DONE for plain auto-close (idle timer), budget-exceeded (both call sites) and `/coordinate/close`.
    One shared builder serves all three plus delete (5.10): `convtask.CloseMemberRequests`, `internal/httpapi`'s `closeCascadeRequests` and the controller's `submitCloseCascade`.
    Each enqueues one `CascadeMember` task per live descendant, deepest-first, ahead of the verb's own final request.
    The final request is the unchanged recursive Router call (`CloseConversation`, `AutoCloseConversation`, `CloseBudgetExceeded`, `CloseCoordinated`).
    Its fresh re-listing of live `causedBy` members is the run-time re-check, and closes any straggler invoked after accept.
    That matches `closeWithCascade`'s own second sweep: close it, never `precondition_failed`.
    Covered by `TestHandleCoordinateCloseAncestorReChecksAMemberInvokedAfterAccept`.
  - PARTIAL on one entry point: a plain chat `/close` (and the console's N individual closes).
    It is found by parsing text inside `chat.Router.HandleMessage`, already inside a single "conv" task with no `commandbus.Bus` to reach.
    Decomposing it would need `chat` to import `commandbus`, reversing this change's dependency direction.
    The alternative is `httpapi` re-parsing command text ahead of `HandleMessage`, forking logic `taskInboundMessage`'s doc comment argues against.
    Left as the documented simplification, see `closecascade.go`'s doc comment.
- [x] 5.4 Confirm where `CloseBudgetExceeded` is called relative to `recordRun`. Enqueue it as a `close` task after the run is recorded, or record why it must stay inline.
  Confirmed: in `internal/httpapi/server.go`'s `handleWorkDone`, the call sits AFTER `s.recordRun` (and after the phase-setting `state.Mutate`) — reading the lines between them shows no reordering needed. Now enqueued as a `conv`/`close`/`budget-exceeded` task there (`Server.closeBudgetExceeded`), and likewise at the reconciler's own deadline-edge call site (`ConversationReconciler.closeBudgetExceeded`) — both fall back to calling the Router inline when no Bus is wired.
- [ ] 5.5 Split `escalate`: a root patch task, and for a member a cascading `close` then `append-result`. Derive the append's dedup id from the task, not the clock.
  PARTIAL. The dedup-id bug (`chat.Router.Escalate`'s member-branch append id embedded `time.Now().UnixNano()`, so a redelivery appended the result twice) is FIXED. The id is the stable `"escalate:"+conv.Name`. Covered by `TestEscalateMemberRedeliveryAppendsTheResultOnlyOnce`.
  `/coordinate/escalate` is now ACCEPT-THEN-PROCESS (`VerbEscalate`/`taskEscalate`, 18 existing `coordinate_test.go` cases updated to drain before asserting the effect), closing the gap this note used to carry. NOT done: the literal two-task split this task names for a member's own branch (a cascading close task, THEN a separate append-result task). `taskEscalate` instead calls `Router.Escalate` UNCHANGED — one task performing cascade-close-then-append together, same as it always has, now just running later, UNLIKE `/coordinate/close`'s own member decomposition (5.3's note) — this split was out of scope for this pass, a deliberate choice named rather than guessed at.
- [ ] 5.6 Process inbound chat and inbound signals on `conv` and `in.signal`. Accept the optional `Idempotency-Key` and derive a key from channel, thread, text and sender when it is absent.
  DONE for inbound CHAT (`/channel/inbound` → one `conv` task, `Idempotency-Key` honoured, else a derived hash of channel/thread/text/sender — `inboundMessageCommandID`). NOT done for inbound SIGNALS: `handleSignalInbound`'s `answerCommands`/`publish` stay synchronous. `in.signal` carries no producer or consumer in this change. See the hand-back report for why (the ~15 existing `publish_test.go` tests assert synchronous storage, and this is the highest-traffic ingestion path — deserves its own pass).
- [x] 5.7 Make `delete` of an absent conversation succeed.
  `handleConversationDelete` answers `202` with no task for an already-absent target. `taskDelete` treats `state.ErrNotFound` as success for every cascade member too.
- [x] 5.8 Add `GET /state/tasks`, `/state/tasks/{id}`, `/state/tasks/stream` (resumable, ordered by revision) and `GET /state/dead-letters`, read-only, on the existing `/state/*` token scheme.
  `/state/tasks/{id}` (`Bus.GetTask`), `/state/tasks/stream` (`Bus.Feed`, same SSE shape as the conversation/signal streams) and `/state/dead-letters` (`Bus.DeadLetters`) are direct. `GET /state/tasks` is NOT a flat item list — `commandbus.Bus` (sections 1-4) exposes no such method, and `Feed`'s own "zero resume starts from now" rule means it cannot replay one either. This is a genuine interface gap, named in `statetasks.go`'s own doc comment and in the hand-back report rather than invented around. `GET /state/tasks` instead serves the per-(queue,kind) `Stats` the contract DOES expose.
- [x] 5.9 Verify each endpoint against `docs/contracts/channel-adapter.md` and `docs/contracts/runtime.md`. Neither is edited. A needed edit is a stop that asks the user.
  Verified for `reopen`/`delete`/`reset-context`/`/channel/inbound`: the contract states a request body and behavioural requirements (`MGR-7`, `MGR-8`, `IN-1`..`IN-4`) for all four, every one preserved exactly, but names NO response status code or body shape for any of them — adding `invocationId`/`taskIds` and moving `reset-context` from `200` to `202` is additive, not a conflict. Neither contract file touched. NOT verified for `/signal/inbound` or `/coordinate/*`, since neither was changed (5.5/5.6).
- [x] 5.10 Enqueue a delete cascade over already-closed descendants deepest-first, one task per conversation, on the root's key, ending with the root.
  `deleteCascadeRequests`/`deleteCascadeOrder` in `internal/httpapi/server.go`.

**Harness regression, fixed.** 5.1/5.2/5.5's own notes describe tests
"updated to drain before asserting". `internal/integration/suite_test.go`
never actually gained a `Bus`, though. Every converted endpoint panicked
on a nil `Bus.Submit` and aborted the whole envtest binary.

Fix: wire an in-process `commandbus/memory` bus into
`apiServerWithActivity()`. Add `drainConvTasks` (`suite_test.go`) — the
`conv` queue's own analogue of `postSignal`'s `consumeNewSignals`. The full
suite now passes (`go test ./internal/integration/...`).

## 6. `OpQueue` migration — backend-developer

- [x] 6.1 Re-implement `OpQueue` over the bus as `out.<channelType>`, keyed by conversation and channel, keeping `Enqueue*`, `Claim`, `Complete`, `Settled`, `Pending` and `Stats`.
- [x] 6.2 Keep the dedup rule: a stable id is suppressed while pending, retrying, parked or settled. A failure never suppresses its own recovery.
- [x] 6.3 Keep `Complete`'s handlers synchronous before the `200`. A failing handler releases the id.
- [x] 6.4 Give ephemeral sends their short TTL and `delete-conversation` a TTL of the finalizer's grace, never parked.
- [x] 6.5 Run in-process providers as a consumer over `out.<type>`.
- [x] 6.6 Keep `/api/queues` working from the per-queue statistics.

**Claim stays non-blocking over a blocking `Bus.Claim`.**
`handleChannelOps` already owns the long poll. `OpQueue.Claim` gives
`Bus.Claim` a short, non-zero deadline and reads `context.DeadlineExceeded`
as "nothing claimable".

Not literally zero: the nats backend's own mirror-readiness wait ties a
zero deadline against an already-ready channel. Go's `select` breaks that
tie at random. Proven against both backends
(`TestClaimOnAnEmptyQueueReturnsPromptly`/
`TestClaimWithAShortDeadlineStillFindsAnEligibleTask` in
`internal/commandbus/memory` and `internal/commandbus/nats`).

**`commandbus.QueueStats` gained five fields**: `OldestPendingID`,
`OldestPendingKey`, `OldestRunningAge`, `OldestRunningID`,
`OldestRunningKey`. `Stats()`'s own identity fields need an op's identity,
not only a count, and the bus carried none. Both backends and the
conformance suite extended together (`testPerQueueStatistics`).

**The claimed-op to bus-task bridge is a small local index**:
`OpQueue.claimed`, keyed by the PUBLISHED op id, holding the lease token
and the attempt/maxAttempts pair `resultingStateAfterFail` needs. Claim
populates it. Complete consumes it.

**Complete's old release-on-failure hack is retired.** The bus's own
attempts and backoff (design D-5) give a failed stable op its next chance
now. Complete calls `Fail`.

`CodeUnavailable` is for an adapter's own reported error.
`CodeInternal` is for the manager's own write failing despite the
adapter's success. Either way the bus decides retry vs dead.

One narrow exception, found and fixed along the way.
`out.deleteConversation`'s own `DedupWindow` is declared `0`
(deliberately, conformance-tested). That offers the bus no suppression at
all the instant after a success.

`finalizeClose` (`internal/controller`) re-enqueues every pass regardless.
`OpQueue.enqueue` now checks its own local "settled" mirror (success only,
never Dead) before ever asking the bus.

This restores the old queue's "stays completed" behaviour for this one
op, without touching the already-examined inventory value.

**In-process providers run via a real `consumer.Pool`**, one per
`Registry`-registered channel type, started from `OpQueue.Run`.

This is a new `manager.RunnableFunc`, registered unconditionally from
`cmd/manager` — it also carries the watch-fed Pending/Settled/Stats
mirror, needed regardless of whether any type is registered.

`Registry` gained `Types()` to list what to start pools for.

No channel type is registered in production today (`platform/console` is
an external adapter). This path is exercised by tests only, and the
mechanism is real and unchanged by that.

**A real, now-twice-found commandbus gap, left open.** `Bus` has no "list
every queue in use" call.

`OpQueue.Stats()` works around it with its own `channelTypes` set,
populated from this process's own enqueues and claims, and from
`watchMirror`'s Feed consumption of every `out.*` queue it observes.

That is sufficient for every test and for `/api/queues`. A fresh replica
with no local history will not list a channel type until its own Feed
watch, or a local enqueue or claim, has seen one.

Left to a future change if the gap matters beyond this.

## 7. Leader proxy and claim placeholder — backend-developer

- [x] 7.1 Exempt `/channel/ops`, `/channel/ops/{id}/done`, `/state/tasks*` and `/state/dead-letters` from the leader proxy. Accept endpoints still proxy.
- [x] 7.2 Stop writing `status.threads[].claim` and ignore a value found there. The five-minute lease is owned by the bus, and `claimStalenessSeconds` is inert.
- [x] 7.3 Add the retired-vocabulary entry for `status.threads[].claim` in this change's apply, when the field stops being written. Doing it at proposal time would fail the guard on the published `durable-chat-ops-broker` spec, which still describes the field until archive. Run `retired-vocabulary-guard.py`.

## 8. Chart — deployment-engineer

- [x] 8.1 Decide whether the existing `nats` subchart can be instantiated a second time under an alias. If it cannot, give the bus its own template.
  **Verdict: cannot — own template.** `chart/charts/nats/templates/_helpers.tpl`
  names every object it renders with a hardcoded literal, 9 occurrences across
  5 definitions, none parameterized by anything the caller's `dict` passes in:
  `agentops-nats` (StatefulSet + both Services' selector/labels + PDB +
  NetworkPolicy, repeated), `agentops-nats-headless`, `agentops-nats-client`,
  `agentops-nats-credentials`, and `agentops-nats`'s own
  `app.kubernetes.io/name` label used as BOTH the pod selector and the
  NetworkPolicy's identity. A second instantiation under an alias would
  collide on every one of them — two StatefulSets, two Services of each kind,
  one PDB and one credential Secret all claiming the same name. Retrofitting
  a name parameter through every one of those 9 sites is real surface-area
  risk on an already-shipped, already-tested bundle, for a benefit (sharing
  template code between two near-identical-but-not-identical-forever
  resources) D-13 does not ask for. Gave the bus its own template instead:
  `chart/charts/commandbus/templates/_helpers.tpl` (a renamed sibling,
  `commandbus.statefulset`/`.services`/`.pdb`/`.credential`/`.networkpolicy`,
  objects named `agentops-commandbus` and friends throughout), rendered from
  `chart/templates/commandbus.yaml` mirroring `templates/nats.yaml`'s own
  structure and doc comment.
- [ ] 8.2 Add `commandBus.backend`, `commandBus.nats.url`, `commandBus.nats.auth.*`, per-queue declarations and the dead-letter retention caps as values.
  - DONE for `commandBus.backend`, `commandBus.nats.url`, `.enabled`, `.auth.*`, `.replicas`, `.image`, `.persistence`, `.probes` and `.podDisruptionBudget`.
  - DONE for `commandBus.consumerConcurrency`, wiring `COMMANDBUS_CONSUMER_CONCURRENCY`.
    `commandBusConsumerConcurrency()` in `cmd/manager/commandbus.go` reads it.
    Both `OpQueue.Run`'s pool and `httpapi`'s registration in `main.go` call it.
  - NOT DONE for the per-queue declarations: attempts, backoff, TTL, dedup window, park cool-off, in-flight cap.
  - NOT DONE for the retention caps: sweep interval, ledger retention, dead-letter max age and count.
  - The cause is a Go-side gap, not a chart oversight.
    The binary reads only five command bus variables: `COMMANDBUS_BACKEND`, `COMMANDBUS_NATS_URL`, `COMMANDBUS_NATS_USER`, `COMMANDBUS_NATS_PASSWORD` and `COMMANDBUS_CONSUMER_CONCURRENCY`.
    The `Option`s in `internal/commandbus/nats/bus.go` exist, but `commandBusNatsOptions` is `nil` in production.
    Every bucket therefore opens at the hardcoded defaults whatever the chart says.
  - A chart value with nothing on the other end would be an inert knob, which this task refuses.
    The gap is named in the comment under `commandBus:` in `chart/values.yaml`.
    A follow-up must add the env vars. That is Go-side work, out of this lane.
- [x] 8.3 Render a bundle by default, with a PodDisruptionBudget and a chart-generated credential resolved in the usual order (explicit, existing Secret, generation on install only).
- [x] 8.4 Add the NetworkPolicy rule admitting only the manager to the bus server.
- [x] 8.5 Inject the connection variables into the manager Deployment.
- [x] 8.6 Extend the guard: exactly one of a rendered bundle or a URL, a URL needs its own credential, `memory` with more than one replica fails, every contradiction names the value.
  `agentops.commandBusGuard` (`chart/templates/_helpers.tpl`), included from
  both `templates/commandbus.yaml` and `templates/deployment.yaml` (the
  render needs it even when the whole bundle template is otherwise
  skippable). `commandBus.nats.enabled` follows `commandBus.nats.url`'s
  presence exactly as `nats.enabled` follows `stateStore.backend` — hand-set
  it to catch a contradiction, leave it alone and a url opts out of the
  bundle on its own with no error. Verified against every permutation in
  `platform/manager/internal/integration/charttemplate_test.go`'s new
  `TestCommandBus*` cases (see hand-back report for the exact failure
  strings).

## 9. End-to-end tests — testing-specialist

**WRITTEN, COMPILES, NOT RUN — no cluster in this environment.** Same
constraint section 8's own chart dispatch recorded for 9/11: this
environment has no k3d/docker-backed cluster `platform/manager/test/
system/` can install against. Every flow below is added to
`platform/manager/test/system/async_e2e_test.go`, calls `fullTier(t)`
after `requireEnv(t)`, and is verified by:

```sh
cd platform/manager
go vet -tags system ./test/system/...                        # clean
go test -tags system -c -o /tmp/system.test ./test/system/    # builds
go build ./... && go vet ./...                                # clean
KUBEBUILDER_ASSETS=~/.envtest/k8s/1.31.0-linux-amd64 go test ./... -count=1   # all ok, incl. internal/integration
```

None of this is a pass of the flows themselves — that needs the real
k3d cluster, images and chart `system-full.yml` provisions, which this
role did not run. Left for whoever dispatches the cluster tier.

- [ ] 9.1 Add the end-to-end flows to the full tier of the system pack, through the real HTTP surface with the task stream observed.
  Six flows added (9.2-9.7 below), each opening `/state/tasks/stream`
  (or polling `/state/tasks/{id}`/`GET /state/dead-letters`) rather than
  inferring success from the eventual Kubernetes-object side effect
  alone, the way `lanes_test.go`'s existing lanes do.
- [ ] 9.2 Flow: signal, conversation, close, delete, with every task reaching `succeeded` in order.
  `TestAsyncLifecycleSignalCloseDeleteTasksSucceedInOrder`. Named in its
  own doc comment: `/signal/inbound`'s own accept is NOT a task (task
  5.6's deferral, still synchronous), so the two tasks observed in order
  on one stream connection are the chat `/close` and the delete.
- [ ] 9.3 Flow: a bulk close of N conversations as N tasks sharing one invocation, each settling independently.
  `TestAsyncBulkCloseEachConversationGetsItsOwnInvocation`.
  **FINDING: this task's own premise does not hold.** Production code's
  bulk close (`platform/console/convapi.go`'s `handleBulkClose`) is, by
  its own doc comment, "a FAN-OUT of `/close`, deliberately, and not a
  manager-side batch verb" — one independent `POST /channel/inbound` per
  target, each minting its OWN `invocationId` at `accept.go`'s
  `acceptAndEnqueue`. The only place this change shares one
  `invocationId` across more than one `SubmitRequest` is a CASCADE
  (design D-7, task 9.4) — one root's own `causedBy` subtree, never N
  conversations a person multi-selected. The test instead proves the
  true, weaker statement — each of N targets gets its own distinct
  `invocationId`, settling independently — and fails loudly (naming the
  contradiction) if a future change ever makes a plain bulk close share
  one, so the assumption stays checked rather than stale.
- [ ] 9.4 Flow: a cascade delete of a coordination, members before the parent.
  `TestAsyncCascadeDeleteSettlesMembersBeforeParent`. A genuinely
  TWO-level `causedBy` chain (root -> member1 -> member2, member1 itself
  invoking member2 — `handleCoordinateInvoke`'s own doc comment confirms
  a plain member caller, not only a root, may invoke), going beyond
  `coordinator_test.go`'s existing one-level
  `TestCoordinatorCloseAndDeleteCascadeThroughConsole` (which proves
  eventual full deletion only). This flow additionally asserts the
  task-stream ARRIVAL ORDER of the three delete tasks' own `succeeded`
  events matches `deleteCascadeRequests`/`convtask.CascadeOrder`'s
  deepest-first, root-last order exactly.
- [ ] 9.5 Flow: an inbound chat message delivered through the Telegram fake Bot API lane and through the console channel.
  `TestAsyncInboundChatThroughTelegramFakeAndConsole`. Telegram half: a
  reply fed through the real fake Bot API carrying
  `is_topic_message`+`message_thread_id` on an already-bound topic, which
  `gateway-telegram`/`channel-telegram` forward to `/channel/inbound` —
  observed on the task stream (no HTTP response reaches this test for
  that call). Console half: posted directly against the manager's own
  `/channel/inbound`, the same call the console's backend makes
  internally, which is what makes the raw `202`/`invocationId`/`taskIds`
  shape observable directly, ahead of the reply.
- [ ] 9.6 Flow: a manager restart with work in flight loses nothing and duplicates nothing.
  `TestAsyncManagerRestartLosesNothingDuplicatesNothing`.
  **DECISION on the claim's strength:** this pack's command bus always
  renders at the chart default `commandBus.backend: nats`
  (`chart/values.yaml`). `install.go` never overrides it —
  `SYSTEM_STATE_BACKEND` pins the DIFFERENT conversation/signal state
  store, not the command bus. So the ledger and lease (design D-2) live
  in the bus's own NATS server, never the manager process. The test
  therefore asserts the STRONGER of the two claims the task text allows:
  the SAME task UID the old manager minted is the one that later
  settles — not merely the same eventual effect under a new one — and
  that exactly one more outbound call (the farewell) lands on the bound
  Telegram thread, no duplicate.
- [ ] 9.7 Flow: a failing channel surface backs off, then leaves a dead-letter record, and a later re-derive delivers once it recovers.
  `TestAsyncChannelFailureBacksOffDeadLettersThenRecovers`, against the
  real fake Bot API's `/control/fail` toggle on `createForumTopic`
  (ensure-topic, `KindOutStable`: 8 attempts, 5s-5m jittered backoff).
  **DECISION on duration and recovery path:** waits out the real backoff
  to a real `dead` state and a real `GET /state/dead-letters` listing
  (bounded 15 minutes, past the ~10m25s deterministic worst case) rather
  than asserting only the first couple of retries, since reaching dead
  IS this flow's own subject. Recovery is proven through a close+reopen
  once the surface is healthy again — `EnqueueEnsureTopic` mints a FRESH
  `CommandID` per reopen count, so the reopen's own re-derive is a
  genuinely unsuppressed submission — rather than waiting out the
  natural 5-minute park cool-off for the SAME `CommandID` to unsuppress:
  that cool-off's own timing is already pinned at the unit tier by the
  commandbus conformance suite (task 2.6, "park then release"), so
  re-proving it here too would cost every full-tier run several more
  real minutes for a fact this lane does not uniquely decide.

## 10. Unit tests

Audited across the whole repo, not just re-run. Everything else audited
was already real, already passing, and left alone. Three gaps closed:

- An `ErrorCode.Retryable()`/`ParseErrorCode` table test (10.4).
- A `TestCommandBusCredentialPrecedence` test mirroring the state-store
  bundle's own (10.5).
- A test-coverage regression found along the way. Section 7's own commit
  deleted `TestActivityProxiesToLeaderWhenReachable` as collateral damage
  alongside the `/channel/ops` test it meant to retire. Restored verbatim
  in `internal/integration/activity_leaderproxy_test.go`.

- [x] 10.1 `go test ./...` in `platform/manager`, including the conformance suite and bench against `memory` and against the in-process `nats-server`.
- [x] 10.2 Handler tests for every verb in the inventory against the `memory` backend: accept replies, accept-time refusals, idempotent redelivery, the escalate decomposition, the cascade re-check.
- [x] 10.3 `OpQueue` facade tests: the dedup rule after a failure, synchronous completion handlers, ephemeral TTL, in-process providers.
- [x] 10.4 The static import test, the error-code derivation table test and the leader-proxy exemption test.
- [x] 10.5 The chart's render and guard tests for every permutation: default, URL, `memory`, each contradiction, credential precedence, `--is-upgrade`.
- [x] 10.6 `go test` in every other module still passes, and `python3 .github/scripts/publication-guard.py` is clean.

## 11. System tests

**WRITTEN, COMPILES, NOT RUN — no cluster in this environment.** Same
constraint section 9's own end-to-end flows recorded. Every lane below is
added to `platform/manager/test/system/async_system_test.go`, calls
`fullTier(t)` after `requireEnv(t)`, and is verified by:

```sh
cd platform/manager
go vet -tags system ./test/system/...                        # clean
go test -tags system -c -o /tmp/system.test ./test/system/    # builds
go build ./... && go vet ./...                                # clean
KUBEBUILDER_ASSETS=~/.envtest/k8s/1.31.0-linux-amd64 go test ./... -count=1   # all ok, incl. internal/integration
```

None of this is a pass of the lanes themselves — that needs the real k3d
cluster, images and chart `system-full.yml` provisions, which this role
did not run. Left for whoever dispatches the cluster tier (11.7).

- [ ] 11.1 Add lanes to `platform/manager/test/system/`, run against THIS working copy's chart and images, not the default branch's.
  **PACK-WIDE GUARANTEE, not a per-lane assertion.** `install.go`'s own
  `Install` always renders `filepath.Join(repoRoot(), "chart")` — this
  checkout's own working tree, never a fetched ref — and `valuesYAML()`
  always pins `image: agentops-manager tag: e2e`, the tag `images.go`'s
  `BuildAndImport` just built from this commit and `Import`ed directly
  into the node (never pulled). The one named exception:
  `commandBus.nats.image` (`chart/values.yaml`) is a pinned THIRD-PARTY
  image (`nats:2.10-alpine` by digest) genuinely unrelated to "this
  working copy," unlike `agentops-manager`, which carries every line
  sections 1-8 wrote.
- [ ] 11.2 Lane: the bundle comes up, the credential is projected by the kubelet, and the NetworkPolicy admits only the manager.
  `TestAsyncCommandBusBundleUpCredentialProjectedNetworkPolicyEnforced`,
  three `t.Run` steps:

  | Step | Asserts |
  |---|---|
  | bundle up | the StatefulSet at its chart default of 3 ready replicas |
  | credential | `COMMANDBUS_NATS_USER`/`PASSWORD` on the manager pod as `valueFrom.secretKeyRef` against `agentops-commandbus-credentials`, never a literal. The manager's own Available status stands in for "the kubelet-resolved connection already succeeded" — its image is distroless, with no shell to exec a probe from |
  | NetworkPolicy | the FIRST live enforcement lane in this package (the existing `internal/integration/charttemplate_networkpolicy_test.go` is a chart-render test only). Enables `global.agentops.networkPolicy.enabled`, execs `wget` from a non-manager pod (the state store's own nats bundle) against the bus's client port, and asserts the real CNI (k3s's bundled network-policy controller, never disabled by `EnsureCluster`) drops it, while the manager itself stays Available through its own admitted rule |
- [ ] 11.3 Lane: a claim on one manager replica completes on another, and a lapsed lease is redelivered.
  `TestAsyncClaimOnOneReplicaCompletesOnAnother`. **DOCUMENTED DECISION**
  (the same kind of call 9.7 already makes): nothing this pack exposes
  names which replica's own goroutine holds a task's claim —
  `consumer.Pool`'s own `ConsumerID` defaults to the queue's name, never
  `POD_NAME`, so `/state/tasks`' `lease.holder` reads identically on
  every replica, and `taskClose`/`taskDelete` are too fast to reliably
  catch strictly between "claimed" and "settled" from outside the
  process without a double this task's scope has no mandate to extend.
  So the lane pins what it CAN control deterministically: scales to
  exactly one replica first (the sole possible claimant), adds a live
  sibling, then force-deletes the original mid-accept and waits past the
  lease's own 5-minute worst case (8 minutes) for the SAME task UID to
  settle exactly once, with the accept endpoint never going down (a
  sibling stays Ready throughout, unlike 9.6's full-process restart).
- [ ] 11.4 Lane: ordering holds under parallel consumers across two replicas.
  `TestAsyncOrderingHoldsAcrossTwoReplicasUnderParallelConsumers`. Two
  live manager replicas, five inbound messages submitted in rapid
  succession against ONE conversation (one Key), observed on the task
  stream to settle in submission order — design D-2's own claim-time
  rule ("the next is not claimable until the previous has settled"),
  watched directly rather than assumed.
- [ ] 11.5 Lane: a manager restart and a bus-server rolling restart lose no accepted task.
  `TestAsyncManagerRestartAndBusServerRestartLoseNoAcceptedTask`, two
  `t.Run` halves. Manager half reuses 9.6's own
  `restartManagerSchedulable`/`postInboundDuringRestart` mechanism
  (the same fact, asked for again at the system tier by design D-14's
  own table). Bus-server half rolls the bundle's own 3 StatefulSet pods
  one at a time (captured ordinals, never re-listed mid-roll), each
  interleaved with a newly-accepted task, asserting every one still
  succeeds — no claim-timing race to document here, since the ledger
  itself is a 3-way replicated JetStream store, same shape as
  `state_nats_test.go`'s own
  `TestNatsServerPodDeletionMidConversationCompletes`.
- [ ] 11.6 Lane: backoff into a dead letter against a surface that really fails.
  `TestAsyncBackoffIntoADeadLetterAgainstARealFailure`. Section 9.7
  already proves this shape against `test/fakebotapi`'s own cooperating
  `/control/fail` toggle. This lane instead repoints `telegram.apiBase`
  (a plain chart value) at a cluster-internal host nothing answers, so
  every outbound call fails with a genuine DNS/connection error the
  kubelet's own network stack produces — the thing only a real cluster
  decides, per design D-14's own table — through the same backoff,
  dead-letter and `/state/dead-letters` listing checks 9.7 already uses.
- [ ] 11.7 Run the pack and record the verdict. A cluster-less remote session records this as not performed and dispatches the cluster tier instead.
  **NOT PERFORMED — no cluster in this environment.** Left for whoever
  opens the pull request to dispatch, per `.github/routines/
  implement-issue.md`'s own step 6:
  ```sh
  gh workflow run system-smoke.yml --ref change/async-command-bus
  ```

## 12. Documentation

Written against the ACTUAL shipped state of sections 1-11, not the design
doc's aspirational target.

The named deviations are stated plainly in `contracts.md`,
`configuration.md`, `concepts.md`, the guide and the changelog, never
glossed over:

- `/signal/inbound` stays synchronous.
- A plain `/close` and a caused `escalate` stay wholesale inside one task.
- Per-queue tuning has no chart value.
- `claimStalenessSeconds` is inert.

Two already-stale spots unrelated to this task's own prose were fixed along
the way. `contracts.md`'s "`/channel/ops` still requires the leader proxy"
line and `invariants.md`'s matching row were both wrong once section 7
landed. `concepts.md`'s state-restart table still described
`status.threads[].claim` as live.

### 12.1 Reference docs

- [x] 12.1.1 `docs/concepts.md`: where work and state live, and what a reply means.
  Added the command bus ledger as a third declared bounded/lossy state home,
  retired the `claim` sub-bullet and table row, added the accepted-work
  table row, and a pointer from "Ending a conversation" to the new
  accept/process split.
- [x] 12.1.2 `docs/contracts.md`: the accept semantics of each endpoint, `/state/tasks` and `/state/dead-letters`.
  New "Accept and process" section with the full per-endpoint table, the
  new `/state/tasks*`/`/state/dead-letters` rows, and the leader-proxy
  exemption fix above.
- [x] 12.1.3 `docs/configuration.md`: `commandBus.*`.
  New "Command bus" subsection mirroring "State store"'s own shape, plus
  fixes to the now-stale `replicas` and `claimStalenessSeconds` rows under
  Capacity.
- [x] 12.1.4 `docs/security.md`: the new trust boundary and its control, then re-run `threat-model.py`.
  New "Command bus isolation" subsection, folded into crossing 7 (same
  shape as the state store's own) rather than minting a new numbered
  crossing. Diagram regenerated and screenshotted to confirm the relabeled
  "state & bus servers" node still fits its box.
- [x] 12.1.5 `docs/testing.md`: the bus conformance suite, the new lanes and the end-to-end flows.
  Two new sections mirroring the state store's own "conformance suite" /
  "system lanes" pair, naming both test files from sections 9 and 11 and
  stating plainly that neither has run against a real cluster yet.
- [x] 12.1.6 `docs/CHANGELOG.md`: the semantic change and upgrade notes, newest first.
  Added to the existing `[Unreleased]` entry's `Added`/`Changed`/`Upgrade`
  sections — this change ships alongside others already accumulated there,
  not as its own version.
- [x] 12.1.7 A `docs/guides/` page for the bundle.
  `docs/guides/command-bus.md`, new (no existing guide was bundle-shaped to
  mirror — every other guide page is about a CRD kind). Added to
  `_data/nav.yml`. Its own `next` points at the configuration reference,
  not into the guide sequence, since this is an operational topic rather
  than a learning-order step.
- [x] 12.1.8 `.claude/rules/structure.md`, `invariants.md`, `gotchas.md`, `terminology.md`.
  Added `internal/commandbus/` to `structure.md`'s package table. Fixed
  `invariants.md`'s stale leader-proxy row and added two new invariant rows
  (the ledger as a third state home, accept-means-accepted). Added five new
  rows to `gotchas.md` (the `selfSeq` mirror race, the Watermill spike
  finding, the dot-named-queue restriction, the nil-Bus harness panic, the
  coordinate-close authorization-ordering bug). **`terminology.md` left
  untouched, deliberately**: its table is retirements-with-replacements,
  and task/invocation/ledger/dead-letter are new vocabulary, not a renamed
  old term — they belong in the files that define them, which they already
  are.
- [x] 12.1.9 Re-run `python3 .github/scripts/docs-generate.py`, because chart values changed.
  Clean both before and after — the new guide's `renders bundle=commandbus`
  block generated correctly on the first pass.

### 12.2 Adopter site

- [x] 12.2.1 `docs/installation.md`: the new workload and volumes, and uninstall residue.
  Added the bus bundle to the same "next release" paragraph describing the
  state-store bundle's own arrival, a storage-class row, and the bundle's
  volumes to the `helm uninstall` residue paragraph.
- [x] 12.2.2 `docs/getting-started.md`, `docs/introduction.md` and the landing page, wherever they say a close or delete returns when the work is done.
  Searched all three directly. None describes a close/delete/reset-context
  call's reply as meaning the work finished — nothing to change.
- [x] 12.2.3 `README.md`: the component list, within its line budget.
  README's component list never goes past top-level directories
  (`platform/manager/` etc.). No new top-level directory was added —
  nothing to change.
- [x] 12.2.4 The console UI is not touched, so `npm run screenshots` and `npm run demo` do not apply.
  Confirmed with `git diff --stat` from this change's own branch point —
  zero files under `platform/console/ui/` touched by any of this change's
  own commits.
