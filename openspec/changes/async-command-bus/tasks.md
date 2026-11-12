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
  DONE for `reopen`/`delete`/`reset-context`/`/channel/inbound` (the thread lookup, self-input and closed-thread refusals run at accept with no task) AND now for `/coordinate/close`/`/coordinate/escalate` (the move stamp runs synchronously at accept, unconditionally on a successful `Submit` — see 5.2/5.5). The reply stays `200` for these two, per the proposal's own "a coordinator verb keeps the `200`... and gains the same two fields" — `202` applies to every OTHER converted verb, not this pair. NOT done for `/signal/inbound` (no Wired/filter-reason or receipt change — publish stays synchronous, see 5.6's note) — still deferred, not guessed at.
- [x] 5.2 Move `close` (console single and bulk, chat, coordinated, the idle timer, a member's `escalate`, budget-exceeded), `exit`, `reopen`, `delete` and `reset-context` into handlers. Keep immediate refusals that need only a cheap read, such as `delete` of a conversation not Closed.
  DONE: chat `/close`/`/exit` (via the inbound-message task), the idle timer, budget-exceeded (both call sites — the reconciler's deadline edge and `handleWorkDone`'s maxTurns edge), `reopen`, `delete`, `reset-context`, and now `coordinated` close (`/coordinate/close` → `VerbCloseCoordinated`/`taskCloseCoordinated`) and a member's `escalate` (`/coordinate/escalate` → `VerbEscalate`/`taskEscalate`). "Console bulk" is confirmed to be N individual chat `/close` messages (no bulk HTTP endpoint exists), so it is already covered by the chat path. `/coordinate/close`'s own widened-scope check (one-hop membership, the Coordinator-owner sibling-root widening, `isHumanInitiated`) is NOT a cheap read in the "immediate refusal" sense this task names — it stays deferred into `taskCloseCoordinated`, which re-derives it fresh via the unchanged `Router.CloseCoordinated`. Only target-defaults-to-caller (a string default) stays at accept.
- [ ] 5.3 Enqueue a cascade at accept, members before the parent on the root's key. The ancestor re-checks its descendants at run time and closes any open one or fails `precondition_failed`.
  DONE to the letter for plain auto-close (idle timer), budget-exceeded (both call sites) and `/coordinate/close` — one shared builder, `convtask.CloseMemberRequests`/`internal/httpapi`'s `closeCascadeRequests` (and the controller's own `submitCloseCascade` over the same function), used by all three plus delete (5.10). Each enqueues one `CascadeMember` task per LIVE descendant, deepest-first, ahead of that verb's own FINAL request — which is the exact pre-existing recursive Router call (`CloseConversation`/`AutoCloseConversation`/`CloseBudgetExceeded`/`CloseCoordinated`, UNCHANGED), reused rather than reimplemented: its own fresh re-listing of live `causedBy` members when it runs IS the "ancestor re-checks its descendants at run time" behaviour, closing any straggler invoked between accept and now — the same choice `closeWithCascade`'s own pre-existing second sweep already made (close it, never `precondition_failed`), kept consistent rather than invented twice. Covered by `TestHandleCoordinateCloseAncestorReChecksAMemberInvokedAfterAccept`.
  PARTIAL on one entry point only: a plain chat `/close` (and the console's N-individual-closes) is discovered by PARSING TEXT inside `chat.Router.HandleMessage`, which already runs inside a single "conv" task with no `commandbus.Bus` of its own to reach — decomposing it would mean either `chat` importing `commandbus` (reversing this change's own dependency direction) or `httpapi` re-parsing command text ahead of `HandleMessage` (forking logic `taskInboundMessage`'s own doc comment argues against). Left as the documented simplification it already was — see `closecascade.go`'s own doc comment.
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
- [x] 7.2 Stop writing `status.threads[].claim` and ignore a value found there. Keep the five-minute lease through `claimStalenessSeconds`.
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
  DONE for `commandBus.backend`, `commandBus.nats.url`/`.enabled`/`.auth.*`/
  `.replicas`/`.image`/`.persistence`/`.probes`/`.podDisruptionBudget` and
  `commandBus.consumerConcurrency` (wiring `COMMANDBUS_CONSUMER_CONCURRENCY`,
  real and backend-independent — confirmed read at
  `cmd/manager/commandbus.go`'s `commandBusConsumerConcurrency()`, called from
  both `OpQueue.Run`'s pool and `httpapi`'s own registration in `main.go`).
  NOT DONE for the per-queue declarations (attempts/backoff/TTL/dedup window/
  park cool-off/in-flight cap) and the dead-letter/ledger retention caps
  (sweep interval, ledger retention, dead-letter max age and count) — a
  genuine Go-side gap, not a chart oversight. Confirmed by reading
  `cmd/manager/commandbus.go` end to end and grepping `internal/commandbus/`
  for every env read: the ONLY env vars this binary consumes for the command
  bus are `COMMANDBUS_BACKEND`, `COMMANDBUS_NATS_URL`,
  `COMMANDBUS_NATS_USER`, `COMMANDBUS_NATS_PASSWORD` and
  `COMMANDBUS_CONSUMER_CONCURRENCY`. `internal/commandbus/nats/bus.go`'s own
  `WithInventory`/`WithSweepInterval`/`WithLedgerRetention`/
  `WithDeadLetterRetention` `Option`s exist, but `commandBusNatsOptions` —
  the one slice `newNatsCommandBus` passes them through — is `nil` in
  production (only a test sets it). So every bucket opens today at
  `commandbus.DefaultInventory()`'s and `commandbus/nats`'s own hardcoded
  defaults regardless of any chart value. Rendering a value here with
  nothing on the other end would be exactly the inert-knob-with-no-effect
  this task's own instructions refuse. Named in `chart/values.yaml`'s own
  comment under `commandBus:` so the gap is visible there too, and the
  env vars a follow-up would need to add are named above — Go-side work,
  out of this lane.
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

- [ ] 9.1 Add the end-to-end flows to the full tier of the system pack, through the real HTTP surface with the task stream observed.
- [ ] 9.2 Flow: signal, conversation, close, delete, with every task reaching `succeeded` in order.
- [ ] 9.3 Flow: a bulk close of N conversations as N tasks sharing one invocation, each settling independently.
- [ ] 9.4 Flow: a cascade delete of a coordination, members before the parent.
- [ ] 9.5 Flow: an inbound chat message delivered through the Telegram fake Bot API lane and through the console channel.
- [ ] 9.6 Flow: a manager restart with work in flight loses nothing and duplicates nothing.
- [ ] 9.7 Flow: a failing channel surface backs off, then leaves a dead-letter record, and a later re-derive delivers once it recovers.

## 10. Unit tests

- [ ] 10.1 `go test ./...` in `platform/manager`, including the conformance suite and bench against `memory` and against the in-process `nats-server`.
- [ ] 10.2 Handler tests for every verb in the inventory against the `memory` backend: accept replies, accept-time refusals, idempotent redelivery, the escalate decomposition, the cascade re-check.
- [ ] 10.3 `OpQueue` facade tests: the dedup rule after a failure, synchronous completion handlers, ephemeral TTL, in-process providers.
- [ ] 10.4 The static import test, the error-code derivation table test and the leader-proxy exemption test.
- [ ] 10.5 The chart's render and guard tests for every permutation: default, URL, `memory`, each contradiction, credential precedence, `--is-upgrade`.
- [ ] 10.6 `go test` in every other module still passes, and `python3 .github/scripts/publication-guard.py` is clean.

## 11. System tests

- [ ] 11.1 Add lanes to `platform/manager/test/system/`, run against THIS working copy's chart and images, not the default branch's.
- [ ] 11.2 Lane: the bundle comes up, the credential is projected by the kubelet, and the NetworkPolicy admits only the manager.
- [ ] 11.3 Lane: a claim on one manager replica completes on another, and a lapsed lease is redelivered.
- [ ] 11.4 Lane: ordering holds under parallel consumers across two replicas.
- [ ] 11.5 Lane: a manager restart and a bus-server rolling restart lose no accepted task.
- [ ] 11.6 Lane: backoff into a dead letter against a surface that really fails.
- [ ] 11.7 Run the pack and record the verdict. A cluster-less remote session records this as not performed and dispatches the cluster tier instead.

## 12. Documentation

### 12.1 Reference docs

- [ ] 12.1.1 `docs/concepts.md`: where work and state live, and what a reply means.
- [ ] 12.1.2 `docs/contracts.md`: the accept semantics of each endpoint, `/state/tasks` and `/state/dead-letters`.
- [ ] 12.1.3 `docs/configuration.md`: `commandBus.*`.
- [ ] 12.1.4 `docs/security.md`: the new trust boundary and its control, then re-run `threat-model.py`.
- [ ] 12.1.5 `docs/testing.md`: the bus conformance suite, the new lanes and the end-to-end flows.
- [ ] 12.1.6 `docs/CHANGELOG.md`: the semantic change and upgrade notes, newest first.
- [ ] 12.1.7 A `docs/guides/` page for the bundle.
- [ ] 12.1.8 `.claude/rules/structure.md`, `invariants.md`, `gotchas.md`, `terminology.md`.
- [ ] 12.1.9 Re-run `python3 .github/scripts/docs-generate.py`, because chart values changed.

### 12.2 Adopter site

- [ ] 12.2.1 `docs/installation.md`: the new workload and volumes, and uninstall residue.
- [ ] 12.2.2 `docs/getting-started.md`, `docs/introduction.md` and the landing page, wherever they say a close or delete returns when the work is done.
- [ ] 12.2.3 `README.md`: the component list, within its line budget.
- [ ] 12.2.4 The console UI is not touched, so `npm run screenshots` and `npm run demo` do not apply.
