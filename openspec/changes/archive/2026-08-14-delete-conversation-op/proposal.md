## Why

A deleted conversation tells its threads nothing, and the last thing they were
told is now false.

`conversation-housekeeping` split ending a conversation into two stages. Closing
posts a farewell that says the conversation **can be reopened**, which is true
while it is `Closed`. Deletion — by autodelete, by the console's delete verb, or
by `kubectl delete` — then removes the object, and every bound thread is left
holding a promise nobody can keep. On a chat surface that is worse than silence:
someone reads "reopen it from the console", goes looking, and finds nothing.

The deletion path posts nothing at all today. `finalizeClose` enqueues
`close-topic` for threads that are not yet archived and releases; for a
conversation that was properly closed the threads are already archived, so it
says nothing whatsoever.

Only the CONSOLE currently notices, and only because it holds the CR in a watch
cache and can see the row disappear. Every other adapter learns nothing.

## What Changes

- **A new outbound operation kind, `delete-conversation`**, enqueued once per
  bound thread when a `Conversation` is being deleted. It carries the thread id
  and a typed message; what it MEANS for a transport is the adapter's decision.
- **It replaces `close-topic` on the deletion path.** Closing and deleting are
  different facts and now say so: `close-topic` means "archived, and it may come
  back", `delete-conversation` means "gone, and a new message starts a new
  conversation". A conversation deleted without having been closed gets
  `delete-conversation`, not both.
- **`channel-telegram` implements it** by un-archiving the topic if needed,
  posting the notice, and closing it again — a closed forum topic refuses posts,
  which is exactly the transport knowledge that belongs in the adapter and
  nowhere else.
- **The console implements it** by marking the transcript archived and keeping
  it readable for the session, as it already does for `close-topic`.
- **An adapter that does not implement the kind is not broken.** The contract
  already requires tolerating unknown kinds; the operation is reported failed,
  the finalizer's grace expires, and deletion proceeds — the same posture
  `close-topic` has always had.

Not in scope: changing what deletion itself does, the retention windows, or the
console's delete verb. This is about telling the surfaces.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `channel-adapter-contract`: a fourth operation kind, its payload, and the rule
  that the adapter decides what ending a conversation looks like on its
  transport.
- `conversation-close`: the deletion path enqueues `delete-conversation` rather
  than `close-topic`, and the finalizer waits on it under the same grace.
- `telegram-channel-adapter`: the un-archive → post → re-archive sequence, and
  why a closed topic makes it necessary.

## Impact

- `internal/chat/ops.go` — the new kind, its enqueue helper and its stable id.
- `internal/controller/conversation_controller.go` — `finalizeClose` enqueues
  the new operation instead of `close-topic`.
- `channel-telegram/` — the handler and its Bot API sequence; new image.
- `console/` — the handler; new image.
- `docs/contracts.md` (the kind and the adapter's freedom), `docs/concepts.md`
  (what a deleted conversation's threads are told), `CLAUDE.md` (the op list).
- No CRD change, no chart values change, no new RBAC.
