## 1. Claim fields on the Conversation CR — api-architect

- [ ] 1.1 Add a claim (holder, claimed-at) to `ConversationStatus.Threads[channel]` in `platform/manager/api/v1alpha1/conversation_types.go`. Verify: the field compiles and carries doc comments matching `durable-chat-ops-broker`'s spec requirements.
- [ ] 1.2 Regenerate deepcopy and CRDs (`controller-gen object` and `controller-gen crd`, per `build-test.md`). Verify: `git diff chart/crds/` shows only the new field added, nothing else changed.
- [ ] 1.3 Apply the regenerated CRDs directly to a cluster used for verification in this change's own worktree. Verify: `kubectl get crd conversations.agentops.dev -o jsonpath='{...}'` shows the new field present.

## 2. Leader-only claim and staleness logic — backend-developer

- [ ] 2.1 Implement the claim write (resourceVersion-conditioned Patch) inside the Conversation reconciler's `ensureTopics`, replacing the unconditional `EnqueueEnsureTopic` call. Verify: a new envtest case claims an op and asserts the claim fields land on the CR.
- [ ] 2.2 Implement the staleness check: a claim older than a configurable bound, with no thread yet, is cleared and the op retried. Verify: an envtest case ages a claim past the bound and asserts it is cleared and re-dispatched.
- [ ] 2.2b On each reconcile pass, clear at once a claim whose `holder` is not the current `Lease` holder, without waiting for the staleness bound, and re-dispatch the op. Verify: an envtest case writes a claim naming a former leader and asserts it is cleared and re-dispatched immediately.
- [ ] 2.3 Size the staleness bound from measured `ensure-topic` round-trip latency against the slowest shipped adapter (Telegram), and expose it as a chart value rather than a constant (`design.md`'s Open Question). Verify: the value renders into the manager's env/config and the default has a documented rationale in the chart comment.
- [ ] 2.4 Rewrite `internal/chat/ops.go`'s manager-side API as a `Publisher`/`Subscriber` pair in `github.com/ThreeDotsLabs/watermill`'s interface shape, backed by the claim-aware Conversation CR logic from 2.1–2.2. Verify: every existing `OpQueue` unit test is ported and passes against the new implementation.
- [ ] 2.5 Migrate every existing call site — `ensureTopics`, `deliverEscalation`, `chat.DeliverInputs`, `deliverRunReplies` in `conversation_controller.go`, plus `internal/chat/router.go` and `internal/chat/message.go` — to the new API. Verify: `go build ./...` and the full existing `internal/chat` and `internal/controller` test suites pass unchanged in intent.

## 3. `/channel/ops` rejects on a non-leader — backend-developer

- [ ] 3.1 Add a leader check to `handleChannelOps` in `internal/httpapi/server.go`. A non-leader returns 503 with a `Retry-After` header, never the leader's own claim-derivation path. Verify: a unit test against a non-leader `Server` asserts 503, and against a leader asserts the existing 200/204 behavior unchanged.
- [ ] 3.2 Confirm `/channel/ops/{id}/done` needs no leader check — its Patch is an ordinary, unconditional write safe from any replica (`design.md`). Verify: a unit test completes an op from a non-leader-configured `Server` and asserts the Patch still lands.

- [ ] 3.3 Reject a poll carrying the retired `?type=` parameter, or an absent or unsupported `contract=` version, with 400 naming the replacement. Verify: a unit test asserts 400 for each and 200/204 for a conforming request.

## 4. Shipped adapters retry immediately on 503 — backend-developer

- [ ] 4.1 `channels/telegram`'s poll loop treats 503 the same as an empty/timed-out 204 — retry at once, no backoff, no error logged as a failure. Verify: a conformance test (`test/conformance/`) against a stub manager returning 503 asserts an immediate re-poll.
- [ ] 4.2 `platform/console`'s own poll loop gets the identical treatment. Verify: the same conformance pattern as 4.1, run against the console's binary.

## 4b. Durable derivation of owed ops — backend-developer

- [ ] 4b.1 Release an op's dedup-window entry when it completes with an error and its conversation still exists, so reconciliation re-derives it. Verify: an envtest case fails an op and asserts it is re-derived.
- [ ] 4b.2 Surface a reply whose delivery to a bound thread has failed as `status.threads[].undeliveredReply` (the owed run id) on that thread in `Conversation.status`. Verify: an envtest case asserts the status entry appears on failure and clears on success.
- [ ] 4b.3 Remove `close-topic`'s delivery exemption: add `status.threadsArchived[]`, re-derive `close-topic` for any bound thread missing from it, and mark it on completion. Verify: an envtest case fails a close-topic and asserts it is re-derived, and that a delete-conversation failure is not.

## 5. Chart default and docs values re-enabled — deployment-engineer

- [ ] 5.1 Flip `chart/values.yaml`'s `replicas` back above 1 once sections 1–4 are verified, updating the value's own comment to state the new resilience story instead of "NOT safe to raise yet." Verify: `helm template` renders the new default and the chart's existing render tests pass.
- [ ] 5.2 Deploy this working copy's chart (the worktree's tree, via `chartPath`, per `deploy-worktree-chart`'s own note — never the published chart) to a verification cluster and confirm a fresh chat-bound conversation gets its thread regardless of which manager pod answers the poll. Verify: observed live, both replicas' logs show at least one claim or rejection during the test.

## 6. Unit tests

- [ ] 6.1 `go test ./...` in `platform/manager`, including the new envtest cases from 2.1–2.2 and the `httpapi` cases from 3.1–3.2.
- [ ] 6.2 `go test -tags conformance -count=1 ./test/conformance/` covering the adapter changes from 4.1–4.2.
- [ ] 6.3 The chart's render tests (`platform/manager/internal/integration/charttemplate_test.go`) cover the new `replicas` default and the staleness-bound value from 2.3.

## 7. E2E tests

- [ ] 7.1 Add an e2e lane in `platform/manager/test/e2e/` that runs the manager at `replicas: 2` against a real cluster, posts a chat-bound signal repeatedly, and asserts every resulting conversation gets its thread — the one failure mode this change exists to close, which only a real multi-pod Service and a real kubelet-scheduled rollout can exercise.
- [ ] 7.2 Run the pack (`go test -tags e2e`) and confirm the new lane passes, plus the existing lanes are unaffected by the `OpQueue` rewrite.

## 8. Documentation

### Reference docs

- [ ] 8.1 `docs/contracts.md` — the `/channel/ops` section gains the 503-retry case, matching the modified `channel-adapter-contract` spec.
- [ ] 8.2 `.claude/rules/invariants.md` — state the claim-write exception to "HTTP API is NOT leader-gated," and record the staleness pattern as a named precedent beside the existing runtime-pod-reaping one.
- [ ] 8.3 `.claude/rules/gotchas.md` — record the replicas-break-chat-delivery incident measured this session, so the next person raising `replicas` finds it before re-discovering it live.
- [ ] 8.4 `docs/configuration.md` — the `replicas` row's caveat is rewritten to match the new, safe behavior.
- [ ] 8.5 `docs/CHANGELOG.md` — a new entry naming the adapter-facing 503-retry contract change and that `replicas` is safe to raise again.
- [ ] 8.7 `docs/configuration.md` — document the chart value that sets the claim staleness bound from 2.3, with its default and rationale.
- [ ] 8.6 Re-run `python3 .github/scripts/docs-generate.py` — the CR reference and any generated resource block covering `ConversationStatus` are build output and must not go stale.

### Adopter site

- [ ] 8.7 Confirm no adopter-site page beyond `docs/configuration.md`'s `replicas` row needs a word changed (`proposal.md`'s own Impact section states why) — ticked as a stated "not applicable" rather than silently skipped.
