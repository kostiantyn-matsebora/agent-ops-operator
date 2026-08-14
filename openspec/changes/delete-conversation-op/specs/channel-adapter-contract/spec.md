## ADDED Requirements

### Requirement: A delete-conversation operation reports that a conversation ended for good
The manager SHALL deliver an operation of kind `delete-conversation` once per
bound thread when a `Conversation` is being deleted. It SHALL carry the target
thread id and a typed message in the ordinary markdown subset, stating that the
conversation and its record are gone and that a new message starts a new
conversation.

The operation reports a FACT about the conversation; what it means for a thread
SHALL be the adapter's decision. An adapter MAY post the message, archive the
thread, delete it, rename it, or express the ending in whatever way its
transport affords. It SHALL NOT silently do nothing: an adapter that cannot act
SHALL complete the operation with an error.

It is named for the CONVERSATION rather than the topic because the conversation
is what ended. `ensure-topic` and `close-topic` instruct an adapter about a
thread; this one informs it about a lifecycle event whose thread consequence the
adapter chooses.

`delete-conversation` SHALL REPLACE `close-topic` on the deletion path. A
conversation being deleted SHALL receive one or the other, never both, so an
adapter never has to decide whether a pair means one ending or two.

An adapter that has not implemented the kind SHALL remain correct: unknown kinds
are already tolerated, the operation is reported failed, and deletion proceeds
once the grace expires.

#### Scenario: A deleted conversation's threads are told
- **WHEN** a `Conversation` bound to two channels is deleted
- **THEN** each serving adapter receives one `delete-conversation` operation carrying that channel's thread id and the notice

#### Scenario: Closing and deleting are distinguishable
- **WHEN** an adapter receives `close-topic` and later `delete-conversation` for the same thread
- **THEN** the first means the thread is archived and the conversation may return, and the second means it will not

#### Scenario: A conversation deleted without being closed gets one operation
- **WHEN** a conversation that was never closed is deleted
- **THEN** its threads receive `delete-conversation` and no `close-topic`

#### Scenario: An adapter that does not implement the kind still deletes cleanly
- **WHEN** an adapter completes `delete-conversation` with an unknown-kind error
- **THEN** the deletion proceeds after the bounded grace and the object is removed
