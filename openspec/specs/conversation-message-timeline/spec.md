# conversation-message-timeline

## Purpose

A conversation holds its messages — what people sent and what the agent answered
— as ONE durable ordered record in the Kubernetes API, so every viewer reads the
same thread in the same order after a restart, a reopen, or a surface joining
late. The input queue stays a queue and stays pruned; what changed is that
pruning is no longer the only copy of what a person said.

## Requirements

### Requirement: A conversation holds its messages as one ordered record
A conversation SHALL hold every message it carried — what people sent and what
the agent answered — as ONE record, in the order the messages happened.

The record SHALL live in the Kubernetes API, so it survives a manager restart, a
viewer restart, a reopen, and a surface that joins after the conversation began.
Two readers of one conversation SHALL see the same messages in the same order.

A message SHALL carry what a reader needs to render it without inference: its
text, when it happened, and where it entered the system.

#### Scenario: The sequence reads as a conversation
- **WHEN** a conversation has taken two messages and answered both
- **THEN** its record reads message, answer, message, answer in that order

#### Scenario: A viewer that was not running reconstructs the whole thread
- **WHEN** a viewer starts after a conversation has finished
- **THEN** it renders the same sequence a viewer present throughout would show,
  apart from what was never durable

#### Scenario: A surface joining late reads the history
- **WHEN** a channel is bound to a conversation that already has messages
- **THEN** the conversation's history is available to it, rather than beginning
  at the next message

### Requirement: The work queue and the record are different things
The queue of inputs awaiting dispatch and the record of messages SHALL be
separate. Consuming an input SHALL remove it from the queue and SHALL NOT remove
it from the record.

Pruning the queue is what stops answered work from running twice, and that
behavior SHALL be unchanged. What SHALL change is that pruning no longer
destroys the only copy of what a person sent.

#### Scenario: An answered message stays readable
- **WHEN** an input has been dispatched, answered, and pruned from the queue
- **THEN** its text is still part of the conversation's record

#### Scenario: Pruning still prevents rework
- **WHEN** a conversation is reconciled after a run consumed its inputs
- **THEN** the consumed inputs are not dispatched again

### Requirement: The record is bounded
Message text SHALL be retained inline only up to a stated size, and larger
payloads SHALL remain referenced rather than copied.

A conversation's record SHALL NOT grow without bound. The system already
offloads large payloads to keep object size small, and the record SHALL NOT
undo that.

#### Scenario: A large payload is referenced, not copied
- **WHEN** an input's payload exceeds the inline limit
- **THEN** the record keeps the reference and the reader is told the text is
  elsewhere, rather than the object carrying the whole payload

#### Scenario: An ordinary message is kept whole
- **WHEN** a person sends a message of ordinary length
- **THEN** its text is readable from the record with no further lookup

### Requirement: Records predating this change render as what they are
A conversation whose messages were never recorded SHALL render the part that was
recorded, and SHALL NOT have missing messages invented, reconstructed from a
title, or back-filled from anywhere else.

#### Scenario: An older conversation shows its answers
- **WHEN** a conversation from before this change is opened
- **THEN** it shows the answers it durably holds and does not fabricate the
  questions
