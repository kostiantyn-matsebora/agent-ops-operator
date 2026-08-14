## ADDED Requirements

### Requirement: Deletion tells every bound thread that the conversation is gone
When a `Conversation` is deleted — by autodelete, by a surface's delete verb, or
by `kubectl delete` — the manager SHALL enqueue one `delete-conversation`
operation per bound thread before releasing the close-topics finalizer, and
SHALL NOT enqueue `close-topic` on that path.

This SHALL happen whether or not the conversation was closed first. A closed
conversation's threads were told it could be reopened; deletion makes that false,
and correcting it is the point.

The finalizer SHALL wait for those operations to complete or for the existing
bounded grace of 2 minutes, whichever comes first, and SHALL then release
regardless. The operation is NOT re-derivable — the object is disappearing, so
there is nowhere to record what is owed — and a deletion SHALL never be wedged
by an adapter that is down or that does not implement the kind.

#### Scenario: A closed conversation's threads are corrected on deletion
- **WHEN** a conversation that was closed, and whose threads were archived, is deleted
- **THEN** each bound thread receives a `delete-conversation` operation saying the conversation is gone

#### Scenario: Deletion is not wedged by a silent adapter
- **WHEN** no adapter completes the `delete-conversation` operation within the grace
- **THEN** the finalizer releases and the object is deleted anyway
