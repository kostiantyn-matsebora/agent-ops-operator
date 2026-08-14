## Why

On 2026-08-13 a burst of 44 alert conversations opened in four minutes and
Telegram rate-limited the resulting Bot API calls: 105 `createForumTopic` and 74
`sendMessage` rejections, all `Too Many Requests`. Every forum topic was created
— and 22 of the 44 stayed **completely empty**, 3 received a partial answer, and
none received its opening signal card. Four and a half hours later the answers
were still sitting undelivered in `Conversation.status.runs[].result`.

That asymmetry is the whole diagnosis. `ensure-topic` recovered because its
failure path drops the operation's dedup entry so reconciliation regenerates it
(`internal/chat/ops.go:392-400`). A failed `send` takes the fall-through branch
that only logs (`:423`), leaving its **stable** id in the completed window, so
`enqueue`'s dedup (`:258`) suppresses the reconciler backstop permanently — no
restart required, and no restart recovers it either, because the reconciler's
re-derivation is exactly what is being suppressed.

The result is a durability claim the system does not honor: a reply recorded as
a fact on the CR becomes unenqueueable, and the operator reports itself healthy
while a human watches an empty chat thread.

## What Changes

- The manager's completed-operation window SHALL record operations that
  **succeeded**, not operations that were **attempted**. A derivable operation
  that fails releases its dedup entry so reconciliation re-derives it — the rule
  `ensure-topic` already follows, applied to every derivable kind.
- `close-topic` keeps its terminal semantics and is explicitly exempt: the CR
  that would carry the obligation is being deleted.
- Failed deliveries become visible instead of a log line: a run owed to a bound
  thread is reported on the Conversation rather than silently pending.
- The Telegram adapter gains **pacing** — a per-chat and global limiter sized to
  Telegram's documented Bot API budget — so bursts are spread rather than
  rejected.
- The Telegram adapter honors `retry_after`: a 429 is slept out and retried
  in-process, and only a terminal failure is reported to `/channel/ops/{id}/done`.
- The in-process retry budget is bounded **strictly below** the manager's
  `ReclaimAfter` (5 minutes), so the manager can never re-issue an operation the
  adapter is still working on and produce a duplicate message.
- **Not** in scope, deliberately: an adapter-side operation queue. The manager
  already is one (`Claim` leases a single op at a time, holds it until
  `Complete`, and returns unfinished claims to the front of the queue after
  `ReclaimAfter`). A second queue inside the adapter would be in-memory, lost on
  pod restart, and a second record of what is owed — which
  `state-durability` forbids.

## Capabilities

### New Capabilities

None. This corrects behavior three existing capabilities already claim.

### Modified Capabilities

- `state-durability`: the requirement that outbound operations are derivable
  from CR state gains the failure case. Today it is specified and tested only
  across a **restart**; the dedup window silently defeats it **within** one
  process lifetime, which is how a transient transport error becomes permanent
  data loss.
- `channel-adapter-contract`: an adapter SHALL absorb transient, retryable
  transport backpressure inside its claim window and report only terminal
  failures; reporting a retryable error as an operation failure is what makes
  the manager's recovery path load-bearing for a condition the adapter could
  have ridden out.
- `telegram-channel-adapter`: new requirements for outbound pacing and
  `retry_after` compliance, alongside the existing single-consumer and
  topic-closing requirements.

## Impact

- `internal/chat/ops.go` — `Complete`'s branch structure: the dedup release
  moves from two special cases to the default for derivable kinds.
- `internal/controller/conversation_controller.go` — surfacing an owed-but-
  undelivered reply as a condition.
- `channel-telegram/` — a limiter and a `retry_after`-aware call wrapper around
  the Bot API client; no new module dependencies (the module is dependency-free
  and stays that way).
- Images: `agentops-manager` and `agentops-channel-telegram` both need a tag
  bump; no CRD schema change, no chart values change.
- Recovery of the 22 empty topics is an operational follow-up, not a code
  change — the fixed backstop re-derives them once the manager rolls.
