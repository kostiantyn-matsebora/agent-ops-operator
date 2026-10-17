## MODIFIED Requirements

### Requirement: Topics are archived at the close, and the archive is derivable
The manager SHALL enqueue one `close-topic` operation per bound thread when the
conversation reaches phase `Closed`, and SHALL record each completed archive in
`status.threadsArchived[]`. A bound thread absent from that list is an archive
still owed and SHALL be re-derived by reconciliation.

`close-topic` SHALL therefore NO LONGER be the one operation that is not
re-derivable from CR state. It was the exception only because it was
enqueued while the object was disappearing, leaving nothing to record
against.

The object now survives its close. A failed archive is an ordinary
owed operation. EVERY operation kind is now derivable.

The `agentops.dev/close-topics` finalizer SHALL remain for the one path
where the object really does go away: a `Conversation` deleted without
having been closed, by `kubectl delete` or by autodelete.

It SHALL release once the outstanding operations complete, or after a
bounded grace period of 2 minutes, so deletion never wedges on an
adapter that is down.

Deleting an already-closed conversation SHALL find its threads archived
and release immediately.

A thread whose topic was never created has no thread id to carry. The
manager SHALL NOT enqueue `close-topic` for it. There is nothing to
close, so an op that can only ever fail would retry forever and log an
error on every pass.

The manager SHALL re-evaluate that thread on the next reconcile pass
instead. Once the topic exists — a later `ensure-topic` attempt
succeeds — `close-topic` is enqueued normally.

#### Scenario: Topic archived on close
- **WHEN** a conversation bound to a channel with a thread is closed
- **THEN** a `close-topic` operation carrying that thread id is delivered to the serving adapter and the channel is recorded in `status.threadsArchived[]`

#### Scenario: A failed archive is re-derived
- **WHEN** an adapter reports a `close-topic` operation as failed for a closed conversation
- **THEN** reconciliation re-enqueues it, because the thread is still missing from `status.threadsArchived[]`

#### Scenario: Down adapter cannot block deletion
- **WHEN** a conversation is deleted and no adapter claims the `close-topic` operation within the grace period
- **THEN** the finalizer is removed and the object is deleted anyway

#### Scenario: Manual deletion of a never-closed conversation archives too
- **WHEN** an operator runs `kubectl delete conversation <name>` on a conversation that was never closed
- **THEN** the bound threads receive `close-topic` operations before the object disappears

#### Scenario: Deleting a closed conversation does not archive twice
- **WHEN** a conversation that was properly closed is deleted
- **THEN** its threads are already recorded as archived, no further `close-topic` is enqueued, and the finalizer releases immediately

#### Scenario: A thread with no topic yet is not retried as a failure
- **WHEN** a closed conversation's thread on a channel has an empty thread id
- **THEN** no `close-topic` operation is enqueued for it, and it is not recorded in `status.threadsArchived[]`

#### Scenario: A thread's topic arrives after close
- **WHEN** a closed conversation's thread gets a thread id from a later successful `ensure-topic` attempt
- **THEN** the next reconcile pass enqueues `close-topic` for it normally
