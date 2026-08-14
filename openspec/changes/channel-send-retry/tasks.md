## 1. Manager: the completed window records success, not attempt

- [ ] 1.1 Restructure `OpQueue.Complete` (`internal/chat/ops.go:392-425`) so a failed op releases its dedup entry by default, with `OpCloseTopic` the single exemption; keep the two existing releases that cover a successful call whose Kubernetes write failed
- [ ] 1.2 Replace the bare `case res.Error != ""` log with one that carries the op id and kind — the incident was diagnosable only by correlating counts, because the line logged neither
- [ ] 1.3 Add `TestFailedSendIsReDerivable` to `internal/chat/ops_test.go`: complete a stable `send` with an error, re-enqueue the same id, assert it is queued rather than suppressed
- [ ] 1.4 Add `TestFailedCloseTopicIsNotReDerivable`: same shape for `OpCloseTopic`, asserting suppression holds
- [ ] 1.5 Add `TestFailedInputCardIsReDerivable` covering the stable `input:` id, so the card path is pinned independently of the reply path

## 2. Manager: advertise the reclaim interval

- [ ] 2.1 Add `reclaimAfterSeconds` to the op JSON written by `handleChannelOps` (`internal/httpapi/server.go`), sourced from `chat.ReclaimAfter`
- [ ] 2.2 Document the field in `docs/contracts.md` under the channel op contract as additive and optional
- [ ] 2.3 Extend the httpapi test to assert the field is present and equals `ReclaimAfter` in seconds

## 3. Manager: surface owed replies

- [ ] 3.1 Write a `DeliveryPending` condition in `deliverRunReplies` (`internal/controller/conversation_controller.go:564`): `True` with the undelivered channel names while any tracked run is owed to a bound thread, `False` when all are delivered
- [ ] 3.2 Add the condition to the conversation's printer columns or leave it condition-only — decide against `kubectl get conversations` readability, then record the choice here
- [ ] 3.3 Cover it in `internal/integration/` with a conversation whose send fails, asserting the condition appears and clears after a successful re-derivation

## 4. Telegram adapter: pacing

- [ ] 4.1 Add a dependency-free token bucket to `channel-telegram/` (mutex + `time.Timer`), with a `wait(ctx)` that respects cancellation
- [ ] 4.2 Instantiate two buckets: global 30/s per bot, per-`chat_id` 20/min; resolve the chat id from the served `Channel` config the adapter already parses
- [ ] 4.3 Gate `opsLoop` (`channel-telegram/main.go:210`) on the buckets **before** `NextOp`, so unsendable work stays unclaimed in the manager
- [ ] 4.4 Unit-test the bucket for rate, burst, and context cancellation
- [ ] 4.5 Test that pacing never claims: assert `NextOp` is not called while the budget is exhausted

## 5. Telegram adapter: retry_after

- [ ] 5.1 Parse `parameters.retry_after` from Bot API error responses in `channel-telegram/telegram.go`
- [ ] 5.2 Wrap Bot API calls so a `429` sleeps exactly `retry_after` and retries the same call, for `sendMessage` and `createForumTopic` alike
- [ ] 5.3 Read `reclaimAfterSeconds` from the claimed op, budget total in-process wait at half of it, default 60s when absent
- [ ] 5.4 Report the op failed once the budget is exhausted, so the manager re-derives while the claim is still valid
- [ ] 5.5 Test against a stub Bot API: `429` then `200` reports success with exactly one post; sustained `429` reports failure inside the budget

## 6. Verification and rollout

- [ ] 6.1 Run the module build/vet/test matrix from CLAUDE.md for the root module and `channel-telegram`
- [ ] 6.2 Run the full envtest suite with `KUBEBUILDER_ASSETS`
- [ ] 6.3 Build and push `agentops-manager` and `agentops-channel-telegram` with fresh tags — never overwrite a pushed tag
- [ ] 6.4 Deploy via the `_home-data-center` GitOps repo, not by hand; server-side dry-run before applying
- [ ] 6.5 After rollout, confirm every conversation's runs are delivered to `home-ops` and `DeliveryPending` is absent namespace-wide
- [ ] 6.6 Spot-check that the 22 previously empty topics now carry their card and answer

## 7. Documentation

- [ ] 7.1 `docs/contracts.md`: the backpressure rule (adapters absorb retryable conditions inside the claim window) and `reclaimAfterSeconds`
- [ ] 7.2 `docs/telegram-bundle.md`: pacing constants and the expected alert latency under burst
- [ ] 7.3 `CLAUDE.md`: add the gotcha — the completed-op window records success, not attempt; a failed derivable op must release its entry or reconciliation can never re-derive it
- [ ] 7.4 `CHANGELOG.md` only if the image tags ship as a chart version bump; no CRD or values change otherwise
