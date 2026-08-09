## MODIFIED Requirements

### Requirement: Topics are archived before the conversation object disappears
The manager SHALL hold the deleting conversation with a finalizer while it
enqueues one `close-topic` operation per bound thread, and SHALL release the
finalizer once those operations complete or after a bounded grace period of 2
minutes, whichever comes first. Deletion SHALL therefore never wedge on an
adapter that is down or that does not implement the operation. Deletion by any
means — the `/close` command or a direct `kubectl delete` — SHALL take this
path.

`close-topic` SHALL be the ONLY operation kind that is not re-derivable from CR
state: its failure is logged rather than written as a condition, and it is never
regenerated, because the object that would carry the obligation is on its way
out. The finalizer is what keeps the derivability rule true while one is
outstanding. Every other operation kind, `send` included, SHALL be derivable and
SHALL be re-enqueued by reconciliation after a manager restart.

#### Scenario: Topic archived on close
- **WHEN** a conversation bound to a channel with a thread is closed
- **THEN** a `close-topic` operation carrying that thread id is delivered to the serving adapter

#### Scenario: Down adapter cannot block deletion
- **WHEN** no adapter claims the `close-topic` operation within the grace period
- **THEN** the finalizer is removed and the conversation object is deleted anyway

#### Scenario: Manual deletion archives too
- **WHEN** an operator runs `kubectl delete conversation <name>`
- **THEN** the bound threads receive `close-topic` operations before the object disappears

#### Scenario: Close-topic is not regenerated after a restart
- **WHEN** the manager restarts while a `close-topic` operation is outstanding and the grace period has expired
- **THEN** the operation is not re-enqueued and the conversation object is deleted, leaving at most an open topic a person can close by hand
