## ADDED Requirements

### Requirement: The conversation record holds no queue of inputs
A conversation's control record SHALL NOT carry the inputs waiting to run, and
SHALL NOT carry the ids of inputs already processed. The record SHALL keep the
current run, and each recorded run SHALL keep the inputs it consumed.

A record written by an earlier release that still carries waiting inputs SHALL
be read, and its inputs SHALL be moved to the conversation's queue without loss
and without a duplicate.

#### Scenario: A new input does not touch the record's input list
- **WHEN** an input is accepted for a conversation
- **THEN** the conversation record has no input list, and the input is waiting in the conversation's queue

#### Scenario: A recorded run keeps its inputs
- **WHEN** a run finishes
- **THEN** the run recorded on the conversation lists the inputs it consumed

#### Scenario: A legacy input is moved once
- **WHEN** a record written by an earlier release carries two waiting inputs
- **THEN** both are waiting in the conversation's queue afterwards, once each, and the record no longer carries them

#### Scenario: A legacy input written during a rollout is moved too
- **WHEN** a replica on the earlier release appends an input to a record after the move has run
- **THEN** that input is also moved, once

## MODIFIED Requirements

### Requirement: Input payloads are blobs by key
An input's payload SHALL be stored once under its own key, read by that key,
and deleted by that key. A payload SHALL NOT be embedded in the conversation's
control record. A payload SHALL NOT be embedded in the conversation's queue
message either. The message SHALL carry the key.

#### Scenario: A pruned input releases its payload
- **WHEN** the input a payload belongs to is settled and leaves its queue
- **THEN** the payload is deleted by its key and a later read of that key
  reports it absent

#### Scenario: A purged input releases its payload
- **WHEN** a conversation is deleted with an input still waiting
- **THEN** that input's payload is deleted by its key
