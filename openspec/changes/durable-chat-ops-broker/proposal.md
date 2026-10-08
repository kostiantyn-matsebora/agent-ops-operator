## Why

`internal/chat.OpQueue` is in-memory and per-process, populated only by the
manager's leader-elected Conversation reconciler.

`httpapi`'s `/channel/ops` — what a channel adapter long-polls to claim
`ensure-topic`/`send`/`close-topic`/`delete-conversation` ops — is
explicitly non-leader-gated (`channel-adapter-contract`'s own published
requirement).

Raising `manager.replicas` above 1 therefore silently breaks chat
delivery. A poll landing on the non-leader replica sees a permanently
empty queue.

So `ensure-topic` never completes for roughly half of all new chat-bound
conversations. They sit in phase `Queued` forever, idling out and cycling
their runtime pod on `RUNTIME_IDLE_TTL_M`, with no error anywhere.

Measured live on the reference install. The manager is pinned back to
`replicas: 1` as a stopgap.

## What Changes

- Replace `internal/chat.OpQueue`'s backing store for channel ops with the
  Conversation CR itself. `status.threads[channel]` gains a claim (holder +
  claimedAt), written by ordinary resourceVersion-conditioned Patches — the
  same optimistic-concurrency primitive this codebase already uses elsewhere
  (`AgentsInvoked`).
- Only the LEADER ever writes claim state — both claiming an op out to an
  adapter and deciding a stale claim is abandoned. This is deliberate, not
  an oversight: a bare CAS claim with no owner tracking survives "wrong
  replica answers" but not "claimant pod crashes mid-work" (the claim would
  be stuck forever, since nobody re-derives it). Routing writes through the
  leader reuses the existing, already-proven
  `k8s.io/client-go/tools/leaderelection` Lease failover for that case: a
  dead leader's lease stops renewing, a new leader takes over within the
  lease's own bound, and the claim — which lived only in the dead leader's
  memory — is simply gone, so the new leader's next reconcile pass
  re-derives cleanly. No new heartbeat/TTL machinery is needed for a
  crashed LEADER.
- A separate, smaller staleness bound is still needed on the claim itself,
  for the case where the SAME leader's in-flight op is legitimately slow
  (an `ensure-topic` round-trip to a real chat API is not instant) versus
  actually abandoned. Same shape as this codebase's existing precedent for
  a runtime pod that never starts (`invariants.md`): a deadline after which
  a stuck claim is treated as abandoned and retried, decided entirely by
  the one process that is ever allowed to decide it, so no locking is
  needed for THIS case either.
- A poll landing on a non-leader REJECTS (503, Retry-After) rather than
  being proxied. The adapter's own long-poll retry already tolerates an
  empty/timeout response by design, so rejecting is strictly less code than
  resolving the current `Lease` holder's pod IP and reverse-proxying — and
  it was the option this project's own research into existing solutions
  favored. No generic "forward to current leader" library exists, but a
  reject-and-retry middleware does, used elsewhere for the identical
  problem.
- Adopt `github.com/ThreeDotsLabs/watermill`'s `Publisher`/`Subscriber`
  interface shape for the manager-side abstraction (clean ack semantics,
  no existing backend needs reinventing router/retry machinery), backed by
  a CUSTOM implementation against the Conversation CR — no existing
  watermill backend is Kubernetes-CR-based, so the durable half is custom
  work regardless of the interface chosen.
- Audit and migrate every existing `OpQueue` call site:
  `internal/controller/conversation_controller.go` (`ensureTopics`,
  `deliverEscalation`, `chat.DeliverInputs`, `deliverRunReplies`),
  `internal/chat/router.go`, `internal/chat/message.go`.
- **BREAKING (contract-level, not URL-level)**: a conforming channel
  adapter MUST now treat a `503` from `/channel/ops` the same as an empty
  `204` — retry — rather than treating it as an error. The URL, query
  parameters, and op shape are unchanged.
- `manager.replicas` becomes safe to raise above 1 once this lands. The
  chart default stays at `1` until this change ships.

## Capabilities

### New Capabilities

- `durable-chat-ops-broker`: the claim/staleness mechanism itself — how a
  channel op is claimed, by whom, how a stale claim is recovered, and what
  a non-leader replica does with a poll it cannot safely answer.

### Modified Capabilities

- `channel-adapter-contract`: the `GET /channel/ops` requirement's
  "non-leader-gated" claim is no longer simply true — a poll SHALL now be
  rejected with `503` on a non-leader replica, and a conforming adapter
  MUST retry on that response exactly as it already does on `204`.
- `state-durability`: "the in-memory operation queue SHALL remain the hot
  path and SHALL NOT become the record of what is owed" is replaced — the
  Conversation CR's claim field becomes the record of what is currently
  claimed, not merely what has succeeded, and the in-memory queue on the
  leader becomes a cache of that CR state rather than the sole owner of it.

## Impact

**Code**: `platform/manager/internal/chat/ops.go` (rewritten backing
store), `internal/controller/conversation_controller.go` (every `OpQueue`
call site), `internal/httpapi/server.go` (`/channel/ops` — leader check, 503 path).

**API**: `api/v1alpha1` (the new claim fields and
`status.threads[].undeliveredReply` on `ConversationStatus`), plus CRD
regeneration in `chart/crds/`.

**Chart**: `chart/values.yaml` and `chart/templates/` (the new
`claimStalenessSeconds` value, rendered into the manager's configuration).

**Dependency**: new, `github.com/ThreeDotsLabs/watermill` in
`platform/manager/go.mod` (the one module in this repository that already
takes dependencies).

**Reference docs**:

- `docs/contracts.md` — the `/channel/ops` section gains the 503-retry case.
- `docs/guides/channel-adapter.md` — "Take operations from the queue" gains the 503-retry note.
- `.claude/rules/invariants.md` — "HTTP API is NOT leader-gated" needs the
  claim-write exception stated, plus the staleness pattern recorded as a
  named precedent.
- `.claude/rules/gotchas.md` — the replicas-break-chat-delivery incident,
  so the next person to raise `replicas` without reading this far does not
  re-discover it live.
- `docs/configuration.md` — the new `claimStalenessSeconds` row, and the `replicas` row's "NOT safe to raise yet"
  caveat is lifted once this ships.
- `docs/CHANGELOG.md` — the adapter-facing 503-retry contract change, and
  that `replicas` is safe again.
- `python3 .github/scripts/docs-generate.py` — re-run, since the new
  `ConversationStatus` fields change the generated CR reference.

**Adopter site**: no page beyond `docs/configuration.md`'s `replicas` row
is affected.

This is an internal resilience fix with no new install-time decision.
`claimStalenessSeconds` is a new chart value an adopter MAY set, and it
has a default. The URL and op shapes are
unchanged. The adapter-author guide gains only the 503-retry note named above.
