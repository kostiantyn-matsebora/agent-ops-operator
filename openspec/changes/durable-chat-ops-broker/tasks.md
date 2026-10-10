## 1. Claim fields on the Conversation CR — api-architect

- [x] 1.1 Add a claim (holder, claimedAt) to `ConversationStatus.Threads[channel]` in `platform/manager/api/v1alpha1/conversation_types.go`. Verify: the field compiles and carries doc comments matching `durable-chat-ops-broker`'s spec requirements.
- [x] 1.2 Regenerate deepcopy and CRDs (`controller-gen object` and `controller-gen crd`, per `build-test.md`). Verify: `git diff chart/crds/` shows only the new field added, nothing else changed.
- [x] 1.3 Apply the regenerated CRDs directly to a cluster used for verification in this change's own worktree. RUN against the local Rancher Desktop cluster (`kubectl apply -f chart/crds/`), additive-only diff confirmed before applying.

## 2. Leader-only claim and staleness logic — backend-developer

- [x] 2.1 Implement the claim write (resourceVersion-conditioned Patch) inside the Conversation reconciler's `ensureTopics`, replacing the unconditional `EnqueueEnsureTopic` call. Verify: a new envtest case claims an op and asserts the claim fields land on the CR. (`TestEnsureTopicsWritesClaimOnFirstDispatch`)
- [x] 2.2 Implement the staleness check: a claim older than a configurable bound, with no thread yet, is cleared and the op retried. Verify: an envtest case ages a claim past the bound and asserts it is cleared and re-dispatched. (`TestEnsureTopicsRetriesAStaleClaim`)
- [x] 2.2b On each reconcile pass, clear at once a claim whose `holder` is not the current `Lease` holder, without waiting for the staleness bound, and re-dispatch the op. Verify: an envtest case writes a claim naming a former leader and asserts it is cleared and re-dispatched immediately. (`TestEnsureTopicsClearsAFormerLeadersClaimImmediately`)
- [x] 2.3 Size the staleness bound from measured `ensure-topic` round-trip latency against the slowest shipped adapter (Telegram), and expose it as a chart value rather than a constant (`design.md`'s Open Question). Verify: the value renders into the manager's env/config and the default has a documented rationale in the chart comment.
- [x] 2.6 Fix `isLeader`'s exact-equality comparison in `internal/httpapi/status.go`: controller-runtime's own `leaderelection.NewResourceLock` mints the real Lease's `HolderIdentity` as `os.Hostname() + "_" + a per-process uuid`, never the bare `ReplicaIdentity` main.go compares against — so the check never matched, even for the real leader, and every `/channel/ops` poll 503'd forever. Measured live in the e2e pack (`ensure-topic`/`send` never completed for any channel). Fixed to a PREFIX match (`holder == identity || strings.HasPrefix(holder, identity+"_")`) — hostnames are DNS-1123 names with no `_`, so the delimiter rules out a false match between e.g. `pod-1` and `pod-10`. Verify: `TestChannelOpsRejectsNonLeader` extended with a suffixed holder, both matching and a different replica's. (commit `b49a617c`)
- [x] 2.7 Close the connection on a 503 in BOTH shipped adapters' `NextOp` (`platform/console/manager.go`, `channels/telegram/manager.go`). "Retry at once" (4.1/4.2) only reaches a different replica if the retry actually dials one — a Kubernetes Service picks a backend per TCP CONNECTION, not per request, so an HTTP/1.1 keep-alive connection pinned to a non-leader asks that SAME non-leader forever, with no error ever surfacing once 2.6 made 503 silent. Measured live: a long-poll loop pinned this way never completed a single op across several minutes and a pod restart, despite the leader's queue holding one the whole time. Fixed with `m.HTTP.CloseIdleConnections()` on the 503 path, forcing the next poll to dial fresh. Verify: `TestNextOpClosesTheConnectionOnANonLeaderAnswer` in both packages, using `httptrace.GotConnInfo.Reused` — fails without the fix, passes with it.
- [x] 2.8 Stop `deliverRunReplies` (`internal/controller/conversation_controller.go`) from delivering into a claim-only `ThreadBinding` placeholder (durable-chat-ops-broker's own `setClaim`, written before `ensureTopics` completes, carries `Claim` but no `ThreadID`). The function iterated `conv.Status.Threads` directly instead of through `ThreadFor` (which already treats an empty `ThreadID` as "does not exist"), so a run-reply enqueued with an empty thread id landed on the adapter's channel-level pseudo-thread and was marked `Delivered` PERMANENTLY — the real thread created moments later by `ensureTopics` never received the answer. Measured live under the 3-replica parallel lane (7.3): roughly 1 in 10 conversations hit the race. Fixed by skipping (not delivering, but still recording the channel as owed in the `DeliveryPending` condition) while `ThreadID == ""`. Verify: `TestDeliverRunRepliesSkipsAClaimOnlyPlaceholder` (fails without the fix) and `TestDeliverRunRepliesDeliversOnceTheThreadIsReal`.
- [ ] 2.4 Rewrite `internal/chat/ops.go`'s manager-side API as a `Publisher`/`Subscriber` pair in `github.com/ThreeDotsLabs/watermill`'s interface shape, backed by the claim-aware Conversation CR logic from 2.1–2.2. **DEFERRED.** The correctness fix (2.1–2.3, 3, 4, 4b) does not depend on this rename — it is a naming/interface preference the design itself frames as optional polish ("gives the migration a familiar, documented contract" rather than a behavior change). Renaming `OpQueue`'s public shape touches ~15 files across `internal/chat`, `internal/controller`, `internal/httpapi` and both integration test packages. Attempting it in the same pass as the correctness fix, with no second reviewer before merge, was judged higher-risk than the value it adds. Left for a follow-up change.
- [ ] 2.5 Migrate every existing call site — `ensureTopics`, `deliverEscalation`, `chat.DeliverInputs`, `deliverRunReplies` in `conversation_controller.go`, plus `internal/chat/router.go` and `internal/chat/message.go` — to the new API. **DEFERRED with 2.4**, for the same reason — there is no new API to migrate to.

## 3. `/channel/ops` rejects on a non-leader — backend-developer

- [x] 3.1 Add a leader check to `handleChannelOps` in `internal/httpapi/server.go`. A non-leader returns 503 with a `Retry-After` header, never the leader's own claim-derivation path. Verify: a unit test against a non-leader `Server` asserts 503, and against a leader asserts the existing 200/204 behavior unchanged. (`TestChannelOpsRejectsNonLeader`)
- [x] 3.2 Confirm `/channel/ops/{id}/done` needs no leader check — its Patch is an ordinary, unconditional write safe from any replica (`design.md`). Verify: a unit test completes an op from a non-leader-configured `Server` and asserts the Patch still lands. Confirmed unchanged — `handleChannelOpDone` carries no leader check, and `TestEnsureTopicRoundTripAndDispatchGate` already exercises it end to end.
- [x] 3.3 Reject a poll carrying the retired `?type=` parameter, or an absent or unsupported `contract=` version, with 400 naming the replacement. Verify: a unit test asserts 400 for each and 200/204 for a conforming request. Already shipped (`adapterParam` / `contractOK` in `internal/httpapi/server.go`) before this change — confirmed still covered by `TestChannelAuthRequired` and the conformance suite's contract-handshake assertions.

## 4. Shipped adapters retry immediately on 503 — backend-developer

- [x] 4.1 `channels/telegram`'s poll loop treats 503 the same as an empty/timed-out 204 — retry at once, no backoff, no error logged as a failure. Verify: a conformance test (`test/conformance/`) against a stub manager returning 503 asserts an immediate re-poll. Plus a fast unit test (`TestNextOpTreats503AsNoOpRatherThanAnError`, `TestPollOnceDoesNotSleepOnANonLeaderRejection`).
- [x] 4.2 `platform/console`'s own poll loop gets the identical treatment. Verify: the same conformance pattern as 4.1, run against the console's binary. Plus a unit test (`TestNextOpTreats503AsNoOpRatherThanAnError`).

## 4b. Durable derivation of owed ops — backend-developer

- [x] 4b.1 Release an op's dedup-window entry when it completes with an error and its conversation still exists, so reconciliation re-derives it. Verify: an envtest case fails an op and asserts it is re-derived. Already shipped (`OpQueue.Complete`'s dedup release) before this change — confirmed still passing (`TestFailedSendIsReDerivable`, `TestFailedInputCardIsReDerivable`).
- [x] 4b.2 Surface a reply whose delivery to a bound thread has failed as `status.threads[].undeliveredReply` (the owed run id) on that thread in `Conversation.status`. Verify: an envtest case asserts the status entry appears on failure and clears on success. (`TestRunReplyFailureSurfacesUndeliveredReplyAndSuccessClearsIt`)
- [x] 4b.3 Remove `close-topic`'s delivery exemption: add `status.threadsArchived[]`, re-derive `close-topic` for any bound thread missing from it, and mark it on completion. Verify: an envtest case fails a close-topic and asserts it is re-derived, and that a delete-conversation failure is not. Already shipped before this change — confirmed still passing (`TestFailedCloseTopicIsReDerivable`, `TestFailedDeleteConversationIsNotReDerivable`).

## 5. Chart default and docs values re-enabled — deployment-engineer

- [x] 5.1 Flip `chart/values.yaml`'s `replicas` back above 1 once sections 1–4 are verified, updating the value's own comment to state the new resilience story instead of "NOT safe to raise yet." Verify: `helm template` renders the new default and the chart's existing render tests pass. (`TestManagerReplicasDefaultsAboveOneAndIsOverridable`)
- [x] 5.2 Deploy this working copy's chart to a verification cluster and confirm a fresh chat-bound conversation gets its thread regardless of which manager pod answers the poll. RUN against the local Rancher Desktop cluster, in a disposable `agent-ops-test` namespace beside the live `agent-ops` release — see 7.3's own record for what it found and fixed (2.6–2.8).

## 6. Unit tests

- [x] 6.1 `go test ./...` in `platform/manager`, including the new envtest cases from 2.1–2.2 and the `httpapi` cases from 3.1–3.2. Green.
- [x] 6.2 `go test -tags conformance -count=1 ./test/conformance/` covering the adapter changes from 4.1–4.2. Green.
- [x] 6.3 The chart's render tests (`platform/manager/internal/integration/charttemplate_test.go` — added as `charttemplate_replicas_test.go`) cover the new `replicas` default and the staleness-bound value from 2.3. Green.

## 7. E2E tests

- [x] 7.1 Add an e2e lane in `platform/manager/test/e2e/` that runs the manager at `replicas: 2` against a real cluster, posts a chat-bound signal repeatedly, and asserts every resulting conversation gets its thread. (`replicas_test.go`'s `TestReplicasTwoDeliversEveryConsoleThread`) — the chart's own default is now 2, so the WHOLE pack installs at two manager pods, not only this lane.
- [x] 7.2 The pack run is the cluster tier's, dispatched to `e2e-smoke.yml` on the pull request's branch and judged by CI. Green on `b49a617c` (run 38038804112), including `TestReplicasTwoDeliversEveryConsoleThread`.
- [x] 7.3 Harsher sibling added after a live-cluster run (5.2) on a local 3-replica install found 2.6–2.8: `TestReplicasThreeDeliversEveryConsoleThreadInParallel`.
  - THREE replicas, not the chart's default two.
  - TRUE concurrency — goroutines, not a sequential loop, so two polls can race at the same instant.
  - A stronger assertion than 7.1's: a real thread AND a delivered reply, not merely a thread. 7.1 could not have caught 2.8's race.
  - `scaleManagerReplicas` strips the chart's hard pod anti-affinity for this lane's own duration, since the single-node k3d cluster cannot schedule a third replica otherwise, and restores it on cleanup.
  - `assertManagerRunsAtLeastNReadyReplicas` generalizes 7.1's precondition to check READY pods, not merely listed ones.
  - Verified live against the local Rancher Desktop cluster (10/10 conversations, thread + delivery, in 10-14s across repeated runs) before being written back into this binary. Dispatch to `e2e-smoke.yml` on this change's final head is this task's own completion evidence.

## 8. Documentation

### Reference docs

- [x] 8.1 `docs/contracts.md` — the `/channel/ops` section gains the 503-retry case, matching the modified `channel-adapter-contract` spec.
- [x] 8.1a `docs/guides/channel-adapter.md` — "Take operations from the queue" gains the 503-retry note.
- [x] 8.2 `.claude/rules/invariants.md` — state the claim-write exception to "HTTP API is NOT leader-gated," and record the staleness pattern as a named precedent beside the existing runtime-pod-reaping one.
- [x] 8.3 `.claude/rules/gotchas.md` — record the replicas-break-chat-delivery incident measured this session, so the next person raising `replicas` finds it before re-discovering it live.
- [x] 8.4 `docs/configuration.md` — the `replicas` row's caveat is rewritten to match the new, safe behavior.
- [x] 8.5 `docs/CHANGELOG.md` — a new entry naming the adapter-facing 503-retry contract change and that `replicas` is safe to raise again.
- [x] 8.6 `docs/configuration.md` — document the chart value that sets the claim staleness bound from 2.3, with its default and rationale.
- [x] 8.6a `.github/retired-vocabulary.json` and `.claude/rules/retired-vocabulary.md` — the retired `/channel/ops` `?type=` parameter term already exists (shipped with the earlier `adapter=`/`contract=` work per 3.3) — confirmed present, no new entry needed.
- [x] 8.7 Re-run `python3 .github/scripts/docs-generate.py` — the CR reference and any generated resource block covering `ConversationStatus` are build output and must not go stale.

### Adopter site

- [x] 8.7a `docs/concepts.md` — add `claim` and `undeliveredReply` rows to the restart-resilience matrix, as the `state-durability` spec requires.
- [x] 8.8 Confirm no adopter-site page beyond `docs/configuration.md`'s `replicas` row needs a word changed (`proposal.md`'s own Impact section states why) — confirmed by grep across `docs/installation.md`, `getting-started.md`, `security.md`, `console.md`, `console-guide.md`, `introduction.md`, `index.md`: no mention of `replicas`, `OpQueue` or `channel/ops`.
