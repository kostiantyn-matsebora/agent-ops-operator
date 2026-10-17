## Why

A closed conversation whose topic was never successfully created on a
channel still gets a `close-topic` operation enqueued on every
reconcile pass.

The op carries an empty thread id. The adapter correctly rejects it,
so the archive never completes and the conversation retries forever.

This logs an error every few seconds for as long as the conversation
exists. Found live while diagnosing a telegram gateway auth failure:
`channel-telegram`'s `close-topic` handler rejected the op with
`"close-topic threadId is not a telegram topic id: "`, repeating
indefinitely.

## What Changes

- `reconcileClosed` no longer enqueues `close-topic` for a thread whose
  `status.threads[].threadId` is empty — there is nothing to close yet.
  The thread is skipped, not marked archived, so a later successful
  `ensure-topic` (the topic eventually gets created) still gets its
  `close-topic` enqueued normally on a subsequent pass.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `conversation-close`: the "Topics are archived at the close" requirement
  gains a new scenario — a thread with no thread id (topic never created)
  is skipped by `close-topic` reconciliation instead of retried forever
  against an operation that can never succeed.

## Impact

- `platform/manager/internal/controller/conversation_controller.go`
  (`reconcileClosed`) — the only code change.
- `openspec/specs/conversation-close/spec.md` — the modified requirement.
- `docs/concepts.md` does not describe this reconciliation detail and is
  untouched. No adopter-site page documents close-topic retry behavior.
  None needs updating.
