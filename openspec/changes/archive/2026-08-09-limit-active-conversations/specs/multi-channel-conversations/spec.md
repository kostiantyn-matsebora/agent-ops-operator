## ADDED Requirements

### Requirement: Closing a conversation ends it on every bound channel
Closing a conversation SHALL apply to the whole conversation, not to the channel
the command arrived on: the farewell message SHALL be fanned out to every bound
thread and one `close-topic` operation SHALL be enqueued per bound thread, each
addressed to that channel's serving adapter. A bound channel that never obtained
a thread SHALL be skipped without blocking the others.

#### Scenario: All bound threads are archived
- **WHEN** `/close` is sent in one thread of a conversation bound to three channels
- **THEN** all three threads receive the farewell message and a `close-topic` operation

#### Scenario: Unbound channel is skipped
- **WHEN** a conversation is closed while one of its bound channels has no thread id yet
- **THEN** no `close-topic` operation is enqueued for that channel and the other channels are still archived

#### Scenario: A stalled channel does not hold the others
- **WHEN** one channel's adapter never completes its `close-topic` operation
- **THEN** the other channels are archived and the conversation is deleted after the grace period
