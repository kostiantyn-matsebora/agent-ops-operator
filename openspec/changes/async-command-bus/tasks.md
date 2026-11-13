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
  (`in.signal`, `out.<channel>`) use.
- Its lease is the message ack, the shape D-2 rejects for cross-replica
  completion. The log's conditional claim and change feed still have to be
  hand-written against `nats.go` regardless of the library.
- Adopting it forces `nats.go` from 1.39.1 to 1.52.0 for a thin wrapper
  around `nats.go`'s own JetStream client, for a dependency tree task 3
  does not otherwise need.

`go.mod`/`go.sum` end this task as they started. The `nats` backend (task 3)
builds directly on `nats.go` JetStream, its already-assumed fallback
shape. The spike (a throwaway module outside the repo) is deleted.

## 2. Contract, `memory` backend, conformance suite — backend-developer

- [x] 2.1 Define the contract in `platform/manager/internal/commandbus/`: submit, claim with lease, settle, fail with a code, task and dead-letter reads and feeds, per-queue statistics.
- [x] 2.2 Define the closed queue inventory and its declarations: key, attempts, backoff, TTL, dedup window, park cool-off, per-invocation in-flight cap. A kind may override its queue's attempts and backoff.
- [x] 2.3 Define the error-code vocabulary and derive retryability from the code. Record an unknown code as `internal`.
- [x] 2.4 Write the `memory` backend.
- [x] 2.5 Write the conformance suite and bench, walked once per queue in the inventory, failing and naming a queue left unwired.
- [x] 2.6 Conformance covers: durable accept, idempotent submit, dedup across every state, park then release, FIFO with a retry keeping its place, serialization per key under parallel consumers, lease completion through a second handle, lease-lapse redelivery, bounded retry into dead, TTL expiry counting as settled, a gap-free resumable feed, dead-letter listing, per-queue statistics.

## 3. `nats` backend — backend-developer

- [x] 3.1 SUPERSEDED by 11.1 (queues are append-only, design D-2). The first build kept one mutable record per task in JetStream KV buckets (`tasks`, `ids`, `dead`). Those buckets are inert on an install that ran it.
- [x] 3.2 SUPERSEDED by 11.2. The first build claimed by compare-and-swap on the task record.
- [x] 3.3 A watch-fed local mirror for pending and settled ids, so reconcile never reads the server.
- [x] 3.4 Sweeps: parked-id release, TTL expiry, queue-log and dead-letter retention caps.
- [x] 3.5 Pass the conformance suite and bench against the in-process server.
- [x] 3.6 Extend the static import test to name `internal/state/nats` and `commandbus/nats` as the only importers of `nats.go`.

## 4. Manager wiring — backend-developer

- [x] 4.1 Select the backend from `COMMANDBUS_BACKEND`, reading `COMMANDBUS_NATS_URL`, `COMMANDBUS_NATS_USER` and `COMMANDBUS_NATS_PASSWORD`. Never read `NATS_*`.
- [x] 4.2 Run the consumers on every replica, one pool per queue, with a configurable concurrency.
- [x] 4.3 Report not ready while the bus is unreachable.
- [x] 4.4 Refuse `memory` with more than one replica.

## 5. Accept and process — backend-developer

- [x] 5.1 Build the accept step: authentication, scope, shape, unknown channel or source, signal Wired and filter reasons, the receipt, the move stamp, the thread-to-conversation lookup, then mint `invocationId`, enqueue, reply `202 {invocationId, taskIds}` additively.
  - DONE for `reopen`, `delete`, `reset-context` and `/channel/inbound`.
    The thread lookup, self-input and closed-thread refusals run at accept with no task.
  - DONE for `/coordinate/close` and `/coordinate/escalate`.
    The move stamp runs at accept, on a successful `Submit` (see 5.2 and 5.5).
    The reply stays `200` for these two, per the proposal. Every other converted verb replies `202`.
  - `/signal/inbound` is OUT OF SCOPE here, moved to a separate follow-up change (decided 2026-11-13).
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
- [x] 5.5 `escalate` is one task per conversation (D-8). Its command id is `escalate:<conversation>`, derived from the conversation and not the clock. The two-task split was not built.
  PARTIAL. The dedup-id bug (`chat.Router.Escalate`'s member-branch append id embedded `time.Now().UnixNano()`, so a redelivery appended the result twice) is FIXED. The id is the stable `"escalate:"+conv.Name`. Covered by `TestEscalateMemberRedeliveryAppendsTheResultOnlyOnce`.
  `/coordinate/escalate` is now ACCEPT-THEN-PROCESS (`VerbEscalate`/`taskEscalate`, 18 existing `coordinate_test.go` cases updated to drain before asserting the effect), closing the gap this note used to carry. NOT done: the literal two-task split this task names for a member's own branch (a cascading close task, THEN a separate append-result task). `taskEscalate` instead calls `Router.Escalate` UNCHANGED — one task performing cascade-close-then-append together, same as it always has, now just running later, UNLIKE `/coordinate/close`'s own member decomposition (5.3's note) — this split was out of scope for this pass, a deliberate choice named rather than guessed at.
- [x] 5.6 Process inbound chat and inbound signals on `conv` and `in.signal`. Accept the optional `Idempotency-Key` and derive a key from channel, thread, text and sender when it is absent.
  DONE for inbound CHAT (`/channel/inbound` → one `conv` task, `Idempotency-Key` honoured, else a derived hash of channel/thread/text/sender — `inboundMessageCommandID`). Inbound SIGNALS are OUT OF SCOPE, moved to a separate follow-up change (decided 2026-11-13): `handleSignalInbound`'s `answerCommands`/`publish` stay synchronous. `in.signal` carries no producer or consumer in this change. See the hand-back report for why (the ~15 existing `publish_test.go` tests assert synchronous storage, and this is the highest-traffic ingestion path — deserves its own pass).
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

- [x] 6.1 Re-implement `OpQueue` over the bus as the `out.<channel>` queues, keyed by conversation, keeping `Enqueue*`, `Settled`, `Pending` and `Stats`. Its adapter-facing read, hand-over and report calls are section 13.
- [x] 6.2 Keep the dedup rule: a stable id is suppressed while pending, parked or settled. A failure never suppresses its own recovery.
- [x] 6.3 Keep `Complete`'s handlers synchronous before the `200`. A failing handler fails the message, final, and reconciliation re-derives it after the cool-off.
- [x] 6.4 Give ephemeral sends their short TTL and `delete-conversation` a TTL of the finalizer's grace, never parked.
- [x] 6.5 Run in-process providers as a consumer over `out.<channel>`.
- [x] 6.6 Keep `/api/queues` working from the per-queue statistics.

**This section built the first facade and is superseded in part by sections 10 to 13.**
`OpQueue.Claim` and `Complete`'s retry-on-failure behaviour gave way to `Read`,
`Acknowledge` and `Complete` under the hand-over contract (design D-17). What
this section still decides:

- `OpQueue` keeps its `Enqueue*`, `Settled`, `Pending` and `Stats` surface over
  the bus, and its dedup rule (6.2) and synchronous completion handlers (6.3).
- **In-process providers run via a real `consumer.Pool`**, one per
  `Registry`-registered channel type, started from `OpQueue.Run`. That is a
  `manager.RunnableFunc` registered unconditionally from `cmd/manager`. It also
  carries the watch-fed Pending, Settled and Stats mirror. No channel type is
  registered in production today, since `platform/console` is an external
  adapter, so this path is exercised by tests only.
- `commandbus.QueueStats` gained `OldestPendingID`, `OldestPendingKey`,
  `OldestRunningAge`, `OldestRunningID` and `OldestRunningKey`, because
  `Stats()` needs an op's identity and not only a count.
- `out.deleteConversation` keeps a `DedupWindow` of `0`, so `OpQueue.enqueue`
  checks its local settled mirror (success only, never dead) before asking the
  bus. `finalizeClose` re-enqueues on every pass regardless.
- A known gap, left open. `Bus` has no call to list every queue in use, so
  `OpQueue.Stats()` learns channel queues from this process's own enqueues and
  from `watchMirror`. A fresh replica lists a channel only after it has seen one.

## 7. Leader proxy and claim placeholder — backend-developer

- [x] 7.1 Exempt `/channel/ops`, `/channel/ops/{id}/done`, `/state/tasks*` and `/state/dead-letters` from the leader proxy. Accept endpoints still proxy.
- [x] 7.2 Stop writing `status.threads[].claim` and ignore a value found there. The five-minute lease is owned by the bus, and `claimStalenessSeconds` is inert.
- [ ] 7.3 Add the retired-vocabulary entry for `status.threads[].claim` and `OpClaim` in the change that deletes the field from the CRD and the `OpClaim` Go type. Today the field is no longer written but the type and the CRD field remain for display, so an entry now would retire a name still in use. The published `durable-chat-ops-broker` spec also still describes the field until archive.

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
- [x] 8.2 Add `commandBus.backend`, `commandBus.nats.url`, `commandBus.nats.auth.*`, per-queue declarations and the dead-letter retention caps as values.
  - DONE for `commandBus.backend`, `commandBus.nats.url`, `.enabled`, `.auth.*`, `.replicas`, `.image`, `.persistence`, `.probes` and `.podDisruptionBudget`.
  - DONE for `commandBus.consumerConcurrency`, wiring `COMMANDBUS_CONSUMER_CONCURRENCY`.
    `commandBusConsumerConcurrency()` in `cmd/manager/commandbus.go` reads it.
    Both `OpQueue.Run`'s pool and `httpapi`'s registration in `main.go` call it.
  - NOT DONE for the per-queue declarations: attempts, backoff, TTL, dedup window, park cool-off, in-flight cap.
  - NOT DONE for the retention caps: sweep interval, queue-log retention, dead-letter max age and count.
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
  store, not the command bus. So the log and lease (design D-2) live
  in the bus's own NATS server, never the manager process. The test
  therefore asserts the STRONGER of the two claims the task text allows:
  the SAME task UID the old manager minted is the one that later
  settles — not merely the same eventual effect under a new one — and
  that exactly one more outbound call (the farewell) lands on the bound
  Telegram thread, no duplicate.
- [ ] 9.7 Flow: a failing channel surface backs off, then leaves a dead-letter record, and a later re-derive delivers once it recovers.
  `TestAsyncChannelFailureBacksOffDeadLettersThenRecovers`, against the
  real fake Bot API's `/control/fail` toggle on `createForumTopic`
  (ensure-topic).
  **STALE AGAINST D-17, TO BE REWRITTEN BEFORE IT IS RUN.** It was written for
  the bus retrying channel messages with backoff. A channel message now has no
  bus retry: the adapter retries until the delivery TTL and a failure is final.
  The rewritten flow asserts a failed `ensure-topic` is not redelivered, reaches
  `/state/dead-letters`, and is derived again once its id is released.
  **DECISION on recovery path:** recovery is proven through a close and reopen
  once the surface is healthy again. `EnqueueEnsureTopic` mints a fresh
  `CommandID` per reopen count, so the reopen's re-derive is an unsuppressed
  submission. That avoids waiting out the park cool-off for the same
  `CommandID`, whose timing the conformance suite already pins (task 2.6,
  "park then release").

## 10. Per-channel queues and one fan-out point — backend-developer

- [x] 10.1 Key the `out` family per Channel resource, `out.<channel>`, keyed by conversation. An adapter drains the queues of every Channel that names it, so `/channel/ops?adapter=` is unchanged.
- [x] 10.2 Add one fan-out function, `chat.FanOutThread`, recording a thread entry and enqueueing it to each bound channel's queue in causal order, less the surface that displayed it.
- [x] 10.3 Route inputs, run replies, escalations and notices through it. Remove the separate producers in the reconcile pass and the stopgap flush of run replies before later inputs. The reconciler stays the backstop and calls the same function.
- [x] 10.4 SUPERSEDED by 13.0 and 13.7. A per-channel sequence and a resumable console stream were built here and withdrawn (design D-16). Nothing in the bus or the API carries a cursor or a sequence.
- [x] 10.5 Move the console onto a push stream and leave Telegram on its long-poll over the same per-channel queue. Both now use the one contract of section 13.

## 11. Append-only queues — backend-developer

Measured live: a claim landing between a submit's create and its second write
failed the submit with NATS 10071, so an escalation was lost. Every queue
becomes an append-only log (design D-2): nothing is rewritten.

- [x] 11.1 Rebuild the `nats` backend as JetStream streams of immutable entries, subjects `<queue>.<key>.msg` and `.ev`, with a separate `dead` log. Remove the mutable task records, the order field and the second write at submit.
- [x] 11.2 Claim, complete, fail and dead-letter are appended entries. A claim is appended conditionally on the key's last entry, so only one of two racing claims lands. State is folded from a task's own entries, and a lapsed lease reads as retryable with no write.
- [x] 11.3 Rebuild the `memory` backend as the same append-only log, so both meet one contract.
- [x] 11.4 SUPERSEDED. Using the command id as an SSE cursor was withdrawn with the retained feed (design D-16). A task carries no sequence or order, and `/state/tasks/{id}` shows none.
- [x] 11.5 Delete a message and its entries when it settles as succeeded (the server removes them, as a regular queue does). Retain an unconsumed message for 7 days by default, then trim it. Keep dead letters in a separate log (7 days, 10000). Dedup after settle rests on an in-memory mark per replica for the queue's dedup window.
- [ ] 11.6 Redeploy the manager and console to the live cluster and repeat the "How is ha doing?" scenario: no escalation fails, the analysis precedes the nudge, and nothing needs a refresh.

## 12. Console as a retained feed — superseded

Built and withdrawn (design D-16). A retained log with a cursor and a replay
window needed render-once bookkeeping and made one lost frame invisible. Section
13 replaces it, and nothing of it remains in the code or the contract.

- [x] 12.1 SUPERSEDED by 13.0. The console channel's log as a retained feed trimmed by age and count.
- [x] 12.2 SUPERSEDED by 13.7. `GET /channel/ops/stream` reading by cursor and `Last-Event-ID`.
- [x] 12.3 SUPERSEDED by 13.0. Running the delivery side effects when a frame is written, in place of an acknowledgement.
- [x] 12.4 KEPT, by 13.0. The manager assigns the console's thread id when it enqueues `ensure-topic`, and the console uses it.
- [x] 12.5 SUPERSEDED by 13.0. The console reconnecting with its last received id and keeping no acknowledgement cache.
- [ ] 12.6 Redeploy and repeat the Tuya scenario and a delete: answers arrive once, in order, without a refresh, and a delete is not held waiting for the console. Performed with 13.5.

## 13. Adapter-owned delivery — backend-developer

One contract for every channel adapter (design D-17): read, acknowledge, then
report done or failed, with a delivery TTL per message kind.

- [x] 13.0 Move the console to the same contract: its acknowledgement is receipt from the manager, with no delivery TTL and no report, and the manager deletes the message at the acknowledgement. Remove the retained feed of section 12 and its replay window.
- [x] 13.0a The mechanism is generic: no adapter-name branch or per-adapter inventory entry in the manager or the bus, TTLs per message kind, and the console exception is purely configuration (`CHANNEL_RECEIPT_ADAPTERS`). A test runs the full contract for a made-up adapter name with no code change.
- [x] 13.1 Add the three-step contract to the channel adapter API: `GET /channel/ops` reads without change, `POST /channel/ops/{id}/ack` hands over and starts the delivery TTL, `POST /channel/ops/{id}/done` reports done or failed. Declare a delivery TTL per message kind (stable, ephemeral, topic, `delete-conversation`), set by `CHANNEL_DELIVERY_TTL_*`.
- [x] 13.2 Remove the lease redelivery for acknowledged ops and the `reclaimAfterSeconds` field. The manager fails a message with no report at its TTL as a backstop.
- [x] 13.3 Move `channels/telegram` to it: retry against the gateway until the TTL less a report margin, then report failed. The `ensure-topic` topic id travels back in the done report.
- [x] 13.4 Keep the manager's delivery markers and the close and delete gating driven by the done and failed reports.
- [ ] 13.5 Redeploy and confirm Telegram delivery, a delivery failure ending as failed at its TTL, and an adapter restart not duplicating a message.
- [x] 13.6 Fix the console's receipt of a message to answer `consumed: true` at the acknowledgement, so the manager performs the side effects and deletes the message with no later report.
- [x] 13.7 `GET /channel/ops/stream` pushes each unacknowledged message once per connection to a receipt adapter, re-offers only never-acknowledged ones after a reconnect, has no cursor, and answers `404` to any other adapter.
- [x] 13.8 Chart: `commandBus.channelReceiptAdapters` and `commandBus.deliveryTtl.{stable,ephemeral,topic,deleteConversation}` render `CHANNEL_RECEIPT_ADAPTERS` and `CHANNEL_DELIVERY_TTL_{STABLE,EPHEMERAL,TOPIC,DELETE_CONVERSATION}`. Owned by the chart lane, ticked once its render tests pass.

## 14. Unit tests

Audited across the whole repo, not just re-run. Three gaps closed:

- An `ErrorCode.Retryable()` and `ParseErrorCode` table test (14.4).
- A `TestCommandBusCredentialPrecedence` test mirroring the state-store
  bundle's own (14.5).
- `TestActivityProxiesToLeaderWhenReachable`, deleted as collateral by section
  7 and restored in `internal/integration/activity_leaderproxy_test.go`.

- [x] 14.1 `go test ./...` in `platform/manager`, including the conformance suite and bench against `memory` and against the in-process `nats-server`.
- [x] 14.2 Handler tests for every verb in the inventory against the `memory` backend: accept replies, accept-time refusals, idempotent redelivery, the escalate decomposition, the cascade re-check.
- [x] 14.3 `OpQueue` facade tests: the dedup rule after a failure, synchronous completion handlers, ephemeral TTL, in-process providers.
- [x] 14.4 The static import test, the error-code derivation table test and the leader-proxy exemption test.
- [x] 14.5 The chart's render and guard tests for every permutation: default, URL, `memory`, each contradiction, credential precedence, `--is-upgrade`.
- [x] 14.6 `go test` in every other module still passes, and `python3 .github/scripts/publication-guard.py` is clean.
- [x] 14.7 Unit-test the single fan-out point (`fanout_test.go`): an entry reaches each bound channel once less the displaying surface, and a run reply is enqueued before the corrective input its turn caused.
- [x] 14.8 Unit-test the per-channel queues (`channelqueue_test.go`): one channel not acknowledging never delays another channel of the same adapter type.
- [x] 14.9 Unit-test the receipt stream (`channelstream_test.go`): messages are pushed in queue order once per connection, a reconnect is offered only what was never acknowledged on any replica, a dead connection strands nothing, and an acknowledgement consumes the message and runs its side effects.
- [ ] 14.9a Unit-test the console client (`platform/console/opstream_test.go`): it applies then acknowledges each message in order and reports no done, a failed acknowledgement is offered again and rendered once, and a silent connection is reconnected. Not run in this session, since `go test` in `platform/console` fails setup without a built `ui/dist`.
- [x] 14.10 Conformance, walked for both backends: no entry is ever written twice, a submit succeeds when a consumer claims at once (the 10071 regression, 200 submits against a racing consumer), state folds from entries, and a lapsed lease redelivers with no write.
- [x] 14.11 Unit-test the three-step contract (`handover_helpers_test.go`, `generic_contract_test.go`, bus `conformance/handover.go`): a read changes nothing, an acknowledgement is idempotent and never resent, a message with no report fails at its TTL, a reported failure is final, a restart after the acknowledgement does not duplicate, and a made-up adapter name gets the full contract with the receipt exception purely configured.
- [x] 14.12 Unit-test the Telegram adapter's delivery (`channels/telegram`): read, acknowledge, deliver, report, no delivery of an op the manager will not hand over, and a delivery that cannot finish inside the TTL reports failed.

## 15. System tests

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

None of this is a pass of the lanes themselves. That needs the real k3d
cluster, images and chart `system-full.yml` provisions, which this role did not
run. A cluster-less session records the tier as not performed and dispatches it
(15.8), as `remote-session.md` requires.

- [ ] 15.1 Add lanes to `platform/manager/test/system/`, run against THIS working copy's chart and images, not the default branch's.
  **PACK-WIDE GUARANTEE, not a per-lane assertion.** `install.go`'s own
  `Install` always renders `filepath.Join(repoRoot(), "chart")`, this
  checkout's working tree, never a fetched ref. `valuesYAML()` pins
  `image: agentops-manager tag: e2e`, the tag `images.go`'s
  `BuildAndImport` just built from this commit and imported into the node.
  The one named exception is `commandBus.nats.image` (`chart/values.yaml`), a
  pinned third-party image (`nats:2.10-alpine` by digest).
- [ ] 15.2 Lane: the bundle comes up, the credential is projected by the kubelet, and the NetworkPolicy admits only the manager.
  `TestAsyncCommandBusBundleUpCredentialProjectedNetworkPolicyEnforced`,
  three `t.Run` steps:

  | Step | Asserts |
  |---|---|
  | bundle up | the StatefulSet at its chart default of 3 ready replicas |
  | credential | `COMMANDBUS_NATS_USER`/`PASSWORD` on the manager pod as `valueFrom.secretKeyRef`, never a literal. The manager's own Available status stands in for the kubelet-resolved connection, since its image is distroless with no shell to exec a probe from |
  | NetworkPolicy | the first live enforcement lane in this package. Enables `global.agentops.networkPolicy.enabled`, execs `wget` from a non-manager pod against the bus's client port, and asserts the real CNI drops it while the manager stays Available through its own admitted rule |
- [ ] 15.3 Lane: a claim on one manager replica completes on another, and a lapsed lease is redelivered.
  `TestAsyncClaimOnOneReplicaCompletesOnAnother`. **DOCUMENTED DECISION:**
  nothing this pack exposes names which replica holds a task's claim, and
  `taskClose`/`taskDelete` are too fast to catch between claimed and settled.
  So the lane scales to exactly one replica first, adds a live sibling, then
  force-deletes the original mid-accept and waits past the lease's 5-minute
  worst case (8 minutes) for the same task UID to settle exactly once, with the
  accept endpoint never going down.
- [ ] 15.4 Lane: ordering holds under parallel consumers across two replicas.
  `TestAsyncOrderingHoldsAcrossTwoReplicasUnderParallelConsumers`. Two live
  replicas, five inbound messages in rapid succession against one conversation
  (one key), observed on the task stream to settle in submission order.
- [ ] 15.5 Lane: a manager restart and a bus-server rolling restart lose no accepted task.
  `TestAsyncManagerRestartAndBusServerRestartLoseNoAcceptedTask`, two halves.
  The manager half reuses 9.6's restart mechanism. The bus-server half rolls
  the bundle's 3 StatefulSet pods one at a time, each interleaved with a
  newly accepted task, asserting every one still succeeds.
- [ ] 15.6 Lane: a channel failure against a surface that really fails.
  `TestAsyncBackoffIntoADeadLetterAgainstARealFailure` repoints
  `telegram.apiBase` at a cluster-internal host nothing answers.
  **STALE AGAINST D-17, TO BE REWRITTEN BEFORE IT IS RUN.** It was written for bus
  retry with backoff. The rewritten lane asserts the adapter retries until the
  delivery TTL, reports failed, the message is not redelivered, and it reaches
  `/state/dead-letters`.
- [ ] 15.7 Lane: drop the console stream while messages are produced and reconnect, then confirm nothing is lost, duplicated or reordered.
  `TestAsyncConsoleStreamResumesAfterADropWithoutLossOrDuplicates`.
  **STALE AGAINST D-17, TO BE REWRITTEN BEFORE IT IS RUN.** It asserts a strictly
  increasing sequence read by cursor. The rewritten lane reads
  `GET /channel/ops/stream` as the console, drops it before acknowledging some
  messages, reconnects, and asserts it is offered exactly the unacknowledged
  ones, none twice, in queue order.
- [ ] 15.8 Run the pack and record the verdict. A cluster-less remote session records this as not performed and dispatches the cluster tier instead.
  **NOT PERFORMED — no cluster in this environment.** Left for whoever opens
  the pull request to dispatch:
  ```sh
  gh workflow run system-smoke.yml --ref change/async-command-bus
  ```

## 16. Documentation

Written against the as-built state of sections 1 to 15, not the design's
aspirational target. The named deviations are stated plainly in
`contracts.md`, `configuration.md`, `concepts.md`, the guide and the changelog.

- `/signal/inbound` stays synchronous.
- A plain `/close` and a caused `escalate` stay wholesale inside one task.
- Per-queue tuning has no chart value. The channel delivery TTLs and receipt adapters do.
- `claimStalenessSeconds` is inert.
- `GET /state/tasks` serves per-queue statistics, not a flat list of tasks.

### 16.1 Reference docs

- [x] 16.1.1 `docs/concepts.md`: where work and state live, what a reply means, the per-channel queues and the one fan-out point, and the restart matrix rows for an op and the queue logs.
- [x] 16.1.2 `docs/contracts.md`: the accept semantics of each endpoint, `/state/tasks` (no order field) and `/state/dead-letters`, the channel ops endpoints and the leader-proxy exemptions.
- [x] 16.1.3 `docs/configuration.md`: `commandBus.*`, including `channelReceiptAdapters` and `deliveryTtl.*`.
- [x] 16.1.4 `docs/security.md`: the new trust boundary and its control, then re-run `threat-model.py`. The re-run changed no output file.
- [x] 16.1.5 `docs/testing.md`: the bus conformance suite, the new lanes and the end-to-end flows.
- [x] 16.1.6 `docs/CHANGELOG.md`: the breaking changes and upgrade notes, newest first. The tasks, ids and dead buckets of an earlier build are inert and may be deleted, and channel adapter images upgrade together with the manager.
- [x] 16.1.7 A `docs/guides/` page for the bundle, `docs/guides/command-bus.md`, in `_data/nav.yml`.
- [x] 16.1.8 `.claude/rules/structure.md`, `invariants.md` and `gotchas.md`: the append-only logs, the get, ack, done contract, a claim and ack contract on a push stream stranding ops, and the 10071 submit race. `terminology.md` is untouched, since task, invocation and dead letter are new words, not renamed old ones.
- [x] 16.1.9 Re-run `python3 .github/scripts/docs-generate.py --check`, because chart values changed.
- [x] 16.1.10 `docs/contracts/channel-adapter.md`: the get, ack, done-or-failed contract with the delivery TTL as a new major version, and the console as the receipt-only exception.
- [x] 16.1.11 `.github/retired-vocabulary.json`: the retired channel-delivery words, and `retired-vocabulary-guard.py` and `publication-guard.py` clean. Two entries wait for the archive (16.3).

### 16.2 Adopter site

- [x] 16.2.1 `docs/installation.md`: the new workload and volumes, uninstall residue, and the adapter-image upgrade note.
- [x] 16.2.2 `docs/getting-started.md`, `docs/introduction.md` and the landing page, wherever they say a close or delete returns when the work is done. Searched all three directly. None describes a reply as meaning the work finished, so nothing changes.
- [x] 16.2.3 `README.md`: the component list. It never goes past top-level directories and no new one was added, so nothing changes.
- [ ] 16.2.4 The console moves to the stream, so `npm run screenshots` and `npm run demo` in `platform/console/ui` are re-run and the live thread screenshotted. Zero files under `platform/console/ui/` are touched by this change's own commits, so this is a cluster-tier check.
- [ ] 16.3 At archive, when the published base specs `channel-adapter-contract`, `contract-conformance-suite` and `telegram-channel-adapter` are rewritten, add the retired-vocabulary entries `channel-reclaim-after` (`reclaimAfterSeconds`) and `channel-claim-window` (`claim window`). They are deferred because the base specs still carry both terms until archive and the guard would stay red.
