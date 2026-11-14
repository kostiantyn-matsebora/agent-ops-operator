## Context

Phase is the coarse lifecycle (`Pending/Idle/Queued/Working/Closed`). Retention is
decided by the reconciler's autoclose (idle window) and autodelete (closed window).
Read state (`conversation-read-state`) is the precedent for a manager-written,
operator-driven mark on a Conversation, reachable by adapters only through a
manager verb. See proposal.md for motivation.

## Goals / Non-Goals

**Goals:** one shared mark that protects a conversation from automatic reclaim and
is visible and filterable in the console.

**Non-Goals:** per-reader bookmarks, folders or tags, a saved-for-later reading
list, retention beyond "not reclaimed automatically", and any new chart value.

## Decisions

**D1. A mark, not a phase.** `status.savedAt` (timestamp, absent = not saved).
A phase `Saved` would collide with Closed: a closed conversation is the common
thing to keep. Alternative `spec.saved`: rejected, the operator is not the
conversation's author and spec is wiring resolved at creation. The timestamp over
a bool gives display ("saved 3 days ago") for free.

**D2. Shared, not per-reader.** Unlike `readAt`, saving states a fact about the
conversation's retention, which is one decision for the whole deployment. Per-reader
would need an identity the manager deliberately does not store.

**D3. Two verbs on the existing contract**, `save` and `unsave` under
`/channel/conversations/{name}/`, accept-and-process like `reopen`. Idempotent:
saving a saved conversation keeps the original `savedAt`. A thread command
(`/save`, `/unsave`) calls the same verb, so chat and console share one
implementation, as `/close` does.

**D4. What Saved protects.** Autoclose skips it. Autodelete skips it. A manual
`delete` answers `409` at once (a cheap read against the request, like delete of a
non-Closed conversation). `close`, `exit` and reset-context stay allowed: they
leave the object and context intact. Alternative, also refusing manual close:
rejected, it would trap a Working conversation.

**D5. Members inherit from the root.** A member has no binding and no menu of its
own. Save/unsave on a member answers `409`. A root's autodelete cascade and the
idle timer's per-descendant close treat a member of a saved root as saved.
Alternative, saving cascades `savedAt` to members: rejected, it writes N records and
drifts when a member is added later.

**D6. Housekeeping needs no change.** Its listing is phase-blind and a Closed
conversation keeps its CR, so a saved one's disk state is never an orphan.

**D7. Console** reads `savedAt` through `/state/conversations`, writes only through
the manager verb. Filter is client-side over the list the console already holds.

## Risks / Trade-offs

- A saved conversation accrues storage forever → it is an explicit operator act,
  shown by the filter, and unsave is one click.
- Autoclose skipping a saved Idle conversation leaves its runtime pod to the
  existing idle-runtime eviction, which is unchanged, so no slot is held.
- Replica race between save and an in-flight autodelete task → the task rechecks
  `savedAt` at decision time, under the optimistic-locking rule.

## Migration Plan

Additive optional status field; existing conversations are unsaved. Regenerate and
apply CRDs on install (`gotchas.md`). Rollback: older managers ignore the field, so
saved conversations become reclaimable again.
