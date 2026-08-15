## Context

`OpQueue.Complete` (`internal/chat/ops.go:355-426`) removes an operation from
the pending map and adds its id to a 512-entry completed window on *every*
completion, success or failure. Two branches then delete the entry again when
their follow-up work fails: `ensure-topic` (`:396`) and `markDelivered`
(`:410`). A failed `send` reaches neither — it falls to `case res.Error != ""`
at `:423`, which logs and returns.

`enqueue` (`:258`) suppresses any op that is `stable` and present in that
window. Run replies (`send:<conv>:<channel>:<runId>`) and input cards
(`input:<conv>:<inputID>:<channel>`) are both stable, both re-derived by
`deliverRunReplies` and the input-card pass on every reconcile. So a single
failed send silently disables its own recovery for the life of the process.

The 2026-08-13 incident is the proof: 105 `createForumTopic` rejections all
recovered (every one of 44 topics exists), while 74 `sendMessage` rejections
never did (22 topics empty, 3 partial, no cards anywhere).

Constraints that shape the fix:

- The adapter modules are dependency-free by project rule, so no
  `golang.org/x/time/rate`.
- `Claim` already leases one op at a time and re-queues unfinished claims after
  `ReclaimAfter = 5m`; the manager *is* the queue.
- The manager holds no transport knowledge (`THE MANAGER COMPOSES MEANING;
  ADAPTERS COMPOSE PRESENTATION`), so pacing cannot live there.
- All topics in a Telegram forum share one `chat_id`, so every card, reply and
  topic creation for the whole channel contends for the **same** per-chat
  budget. This is why one alert burst starved the entire channel.

## Goals / Non-Goals

**Goals:**

- A failed derivable op is re-derived by reconciliation without a restart.
- Telegram bursts are paced within Bot API budgets instead of rejected.
- `429` is treated as backpressure, not failure, and `retry_after` is honored.
- An owed-but-undelivered reply is visible on the Conversation.
- Rolling the fix recovers the existing empty topics with no manual step.

**Non-Goals:**

- An adapter-side operation queue. The manager's `Claim`/`ReclaimAfter` lease is
  the queue; a second one would be in-memory and lost on adapter restart.
- Manager-side rate limiting, a pacing knob on `Channel.spec.config`, or any
  Telegram constant in `internal/`.
- Changing `close-topic`'s terminal semantics.
- Retrying `close-topic` or making it derivable — the finalizer's 2-minute grace
  stands.
- Backfilling the incident's lost *cards* by hand; the rollout re-derives them.

## Decisions

### 1. The completed window records success, not attempt

Restructure `Complete` so releasing the dedup entry is the **default** for a
failed op, with `close-topic` the single exemption, rather than a special case
two branches remember to perform.

```
if res.Error != "" && op.Kind != OpCloseTopic {
        release dedup entry            // reconciliation re-derives
}
```

The two existing conditional releases (`finishEnsureTopic` error,
`markDelivered` error) remain: they cover a *successful* transport call whose
Kubernetes write failed, which the new rule does not.

*Alternative considered — track an attempt count and retry in the manager.* This
puts transport-failure policy in the manager, which owns no transport knowledge,
and duplicates a recovery path reconciliation already provides for free. The
whole point of "derivable from CR state" is that the queue does not need to
remember.

*Alternative considered — never add failed ops to the window at all.* Equivalent
for stable ops, but it loses the ability to distinguish "completed with error"
from "never seen" in `Settled`, which the finalizer consults.

### 2. The manager advertises `ReclaimAfter`; the adapter budgets against it

The adapter's retry budget must stay under the manager's reclaim interval or a
second claimant posts a duplicate. Two constants in two dependency-free modules
that must satisfy an inequality is a drift bug waiting to happen — tuning
`ReclaimAfter` down would silently start duplicating messages.

The `/channel/ops` response gains an additive `reclaimAfterSeconds` field. The
adapter budgets one operation's total in-process wait at **half** the advertised
value, defaulting to 60s if the field is absent (older manager).

*Alternative considered — hardcode 2 minutes in the adapter.* Simpler by five
lines, and wrong the first time anyone tunes the manager constant.

### 3. Pacing gates the long-poll, not the send

The adapter's `opsLoop` is already strictly sequential: claim one, execute,
complete, repeat. Pacing is therefore a token-bucket `wait()` placed **before**
`NextOp`, not before the Bot API call.

Gating the claim rather than the call is what keeps the "no adapter queue"
property real: work the adapter cannot yet send stays unclaimed in the manager,
still derivable from CR state, and an adapter restart at that moment loses
nothing.

Two buckets, both adapter constants:

| Bucket | Budget | Rationale |
|---|---|---|
| global, per bot | 30/s | Bot API global send limit |
| per `chat_id` | 20/min | group/supergroup limit; the binding one for a forum |

A hand-rolled token bucket (~30 lines, `time.Timer` + mutex) keeps the module
dependency-free.

*Alternative considered — pace inside the Bot API client.* It would also work,
but the op is already claimed by then, so a crash while waiting loses it until
`ReclaimAfter`.

### 4. Retries are in-process and `retry_after`-driven

The Bot API client gains a wrapper: on `429`, read `parameters.retry_after`,
sleep exactly that long, retry the same call. Never substitute a computed
backoff for a stated interval. Abandon and report failure when the accumulated
wait would exceed the budget from decision 2.

Retrying is safe because a rejected Telegram call posts nothing — the retry
cannot double-post.

### 5. Owed replies surface as a Conversation condition

Add a `DeliveryPending` condition alongside `TopicReady` and `ToolingResolved`:
`True` with the channel names while any run in `status.runs` is undelivered to a
bound thread, cleared when all are delivered. This is derived state, written by
the same reconcile pass that re-enqueues, so it adds no new source of truth.

## Risks / Trade-offs

- **A burst now takes minutes to drain rather than failing fast.** 44 topics +
  44 cards + 56 replies is ~144 calls against a 20/min per-chat budget — over
  seven minutes. This is Telegram's limit, not a design choice; the alternative
  is the current behavior, which loses the messages entirely. → Accept, and note
  it in the bundle docs so alert latency under burst is expected, not a defect.
- **Rolling the fix re-posts every card that failed in the incident**, including
  to the 19 topics that did receive their answer. → Correct behavior (those
  cards were never posted), but it will look like a flood. Roll during a quiet
  window and expect ~69 messages paced over several minutes.
- **Input cards carry no CR-side delivery marker**, so a manager restart can
  re-derive a card that *did* post. → Pre-existing property of the input-card
  design, unchanged here; the in-memory window is the only guard. Out of scope,
  worth a follow-up.
- **`reclaimAfterSeconds` is a contract addition.** → Additive and optional; an
  older adapter ignores it and a newer adapter defaults to 60s against an older
  manager.
- **Releasing the dedup entry on failure means a genuinely undeliverable op
  retries every reconcile.** → Bounded in practice by the reconciler's own
  backoff, and now visible via `DeliveryPending`; a permanently failing channel
  is a condition an operator should see, not one the system should hide.

## Migration Plan

1. Land the manager and adapter changes together; the adapter tolerates an older
   manager and vice versa, so ordering is not load-bearing.
2. Build and push `agentops-manager` and `agentops-channel-telegram` with new
   tags — never overwrite a pushed tag.
3. Deploy through the GitOps repo (`_gitops` helmfile), not by hand.
4. On rollout the manager's completed window starts empty, so the backstop
   re-derives the 22 empty topics' replies and all outstanding cards
   automatically, paced by the new limiter.
5. Verify with `kubectl get conversations` that no run remains undelivered to
   `home-ops` and that `DeliveryPending` is absent across the namespace.

Rollback: revert both image tags. The dedup change is behavioral only — no CRD
schema change, no chart values change, no stored-state migration — so a rollback
returns to the previous behavior with no cleanup.

## Open Questions

- Should `DeliveryPending` also cover input cards? They have no marker to derive
  from, so it would need one; deferred with the card-marker follow-up above.
- Is 20/min the right per-chat constant for a forum supergroup? Telegram
  documents the limit loosely; the constant may need tuning against observed
  `retry_after` values once pacing is live.
