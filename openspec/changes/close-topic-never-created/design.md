## Context

`reconcileClosed` (`platform/manager/internal/controller/conversation_controller.go`)
loops every bound thread of a `Closed` conversation and calls
`EnqueueCloseTopic` unconditionally. See proposal.md - Why.

## Goals / Non-Goals

**Goals:**
- Stop the infinite retry-and-fail loop for a thread whose topic was
  never created.
- Preserve the existing, intentional infinite retry for a thread whose
  topic WAS created but whose `close-topic` op genuinely fails
  (conversation-close spec's "A failed archive is re-derived").

**Non-Goals:**
- Changing `ensure-topic`'s own retry behavior for a closed
  conversation. It keeps trying to create the topic so a pending answer
  can still be delivered — a separate, already-intentional concern.
- Bounding how long an ordinary (not-being-deleted) `Closed` conversation
  may wait for its topic to appear. Only the finalizer path has a grace
  period, and that is unchanged.

## Decisions

**Skip the enqueue, do not mark the thread archived.** Two options were
considered for a thread with `ThreadID == ""`:

1. Mark it `ThreadsArchived` immediately (nothing to close, so call it
   done).
2. Skip enqueueing `close-topic` this pass, leave it unarchived, and
   let the next reconcile re-evaluate.

Option 1 is wrong: if `ensure-topic` later succeeds, a real topic
exists but the thread already reads as archived, and it is never
closed. Option 2 is chosen. The check costs nothing extra — the loop
already reads `t.ThreadID` for the enqueue call.

## Risks / Trade-offs

**A permanently broken adapter (bot removed from the group, token never
fixed) leaves the thread unarchived forever.** Mitigation: this was
already true for the general "failed archive is re-derived" case.

A conversation in this state produces no further log noise now,
instead of one error every few seconds. The finalizer's own bounded
grace period (2 minutes) still bounds the one path that can block:
actual object deletion.
