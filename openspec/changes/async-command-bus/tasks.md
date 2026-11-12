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
  DONE for `reopen`/`delete`/`reset-context`/`/channel/inbound` (the thread lookup, self-input and closed-thread refusals run at accept with no task). NOT done for `/signal/inbound` (no Wired/filter-reason or receipt change — publish stays synchronous, see 5.6's note) or the `/coordinate/*` move stamp (see 5.5's note) — both deferred, not guessed at.
- [ ] 5.2 Move `close` (console single and bulk, chat, coordinated, the idle timer, a member's `escalate`, budget-exceeded), `exit`, `reopen`, `delete` and `reset-context` into handlers. Keep immediate refusals that need only a cheap read, such as `delete` of a conversation not Closed.
  DONE: chat `/close`/`/exit` (via the inbound-message task), the idle timer, budget-exceeded (both call sites — the reconciler's deadline edge and `handleWorkDone`'s maxTurns edge), `reopen`, `delete`, `reset-context`. "Console bulk" is confirmed to be N individual chat `/close` messages (no bulk HTTP endpoint exists), so it is already covered by the chat path — see the hand-back report. NOT done: `coordinated` close (`/coordinate/close`) and a member's `escalate` via `/coordinate/escalate` stay fully synchronous — deferred, see 5.5.
- [ ] 5.3 Enqueue a cascade at accept, members before the parent on the root's key. The ancestor re-checks its descendants at run time and closes any open one or fails `precondition_failed`.
  PARTIAL: implemented to the letter for DELETE only (5.10). A plain close/auto-close/budget-exceeded cascade instead runs wholesale inside ONE task, calling the existing recursive `chat.Router.CloseConversation`/`AutoCloseConversation`/`CloseBudgetExceeded` (unchanged, already re-checks live) — durable and correct, but without the per-member ledger visibility the fuller decomposition would add. Named as a simplification in the hand-back report, not left unstated.
- [x] 5.4 Confirm where `CloseBudgetExceeded` is called relative to `recordRun`. Enqueue it as a `close` task after the run is recorded, or record why it must stay inline.
  Confirmed: in `internal/httpapi/server.go`'s `handleWorkDone`, the call sits AFTER `s.recordRun` (and after the phase-setting `state.Mutate`) — reading the lines between them shows no reordering needed. Now enqueued as a `conv`/`close`/`budget-exceeded` task there (`Server.closeBudgetExceeded`), and likewise at the reconciler's own deadline-edge call site (`ConversationReconciler.closeBudgetExceeded`) — both fall back to calling the Router inline when no Bus is wired.
- [ ] 5.5 Split `escalate`: a root patch task, and for a member a cascading `close` then `append-result`. Derive the append's dedup id from the task, not the clock.
  PARTIAL: the actual bug (`chat.Router.Escalate`'s member-branch append id embedded `time.Now().UnixNano()`, so a redelivery appended the result twice) is FIXED. The id is now the stable `"escalate:"+conv.Name`, which a member can only ever need once in its lifetime (`closeWithCascade` already makes it inert), covered by a new test, `TestEscalateMemberRedeliveryAppendsTheResultOnlyOnce`. The surrounding TASK decomposition (`/coordinate/escalate` becoming two async tasks) is NOT done — that endpoint stays fully synchronous, deferred along with `/coordinate/close`. 18 existing synchronous unit tests in `coordinate_test.go` assert immediate effects — converting both verbs properly is its own focused pass, not a side effect of this one, see the hand-back report.
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

## 6. `OpQueue` migration — backend-developer

- [ ] 6.1 Re-implement `OpQueue` over the bus as `out.<channelType>`, keyed by conversation and channel, keeping `Enqueue*`, `Claim`, `Complete`, `Settled`, `Pending` and `Stats`.
- [ ] 6.2 Keep the dedup rule: a stable id is suppressed while pending, retrying, parked or settled. A failure never suppresses its own recovery.
- [ ] 6.3 Keep `Complete`'s handlers synchronous before the `200`. A failing handler releases the id.
- [ ] 6.4 Give ephemeral sends their short TTL and `delete-conversation` a TTL of the finalizer's grace, never parked.
- [ ] 6.5 Run in-process providers as a consumer over `out.<type>`.
- [ ] 6.6 Keep `/api/queues` working from the per-queue statistics.

## 7. Leader proxy and claim placeholder — backend-developer

- [ ] 7.1 Exempt `/channel/ops`, `/channel/ops/{id}/done`, `/state/tasks*` and `/state/dead-letters` from the leader proxy. Accept endpoints still proxy.
- [ ] 7.2 Stop writing `status.threads[].claim` and ignore a value found there. Keep the five-minute lease through `claimStalenessSeconds`.
- [ ] 7.3 Add the retired-vocabulary entry for `status.threads[].claim` in this change's apply, when the field stops being written. Doing it at proposal time would fail the guard on the published `durable-chat-ops-broker` spec, which still describes the field until archive. Run `retired-vocabulary-guard.py`.

## 8. Chart — deployment-engineer

- [ ] 8.1 Decide whether the existing `nats` subchart can be instantiated a second time under an alias. If it cannot, give the bus its own template.
- [ ] 8.2 Add `commandBus.backend`, `commandBus.nats.url`, `commandBus.nats.auth.*`, per-queue declarations and the dead-letter retention caps as values.
- [ ] 8.3 Render a bundle by default, with a PodDisruptionBudget and a chart-generated credential resolved in the usual order (explicit, existing Secret, generation on install only).
- [ ] 8.4 Add the NetworkPolicy rule admitting only the manager to the bus server.
- [ ] 8.5 Inject the connection variables into the manager Deployment.
- [ ] 8.6 Extend the guard: exactly one of a rendered bundle or a URL, a URL needs its own credential, `memory` with more than one replica fails, every contradiction names the value.

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
