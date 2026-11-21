## ADDED Requirements

### Requirement: The conversation input family is durable and never dropped
An input is a person's message and SHALL NOT be derivable from anything else.
The bus SHALL keep an input until it is settled or its conversation is deleted.

An input SHALL NOT expire. A failed delivery SHALL be retried after a delay and
SHALL NOT end in the dead-letter drop.

A backend that does not keep messages across a restart SHALL declare that, and
SHALL NOT be offered for an install that serves more than one replica.

#### Scenario: A waiting input survives a restart
- **WHEN** an input is waiting and the bus and the manager both restart
- **THEN** the input is still waiting afterwards and is delivered once

#### Scenario: A waiting input does not expire
- **WHEN** an input stays unclaimed for longer than the retention of every other queue
- **THEN** it is still waiting and is delivered once the conversation can run it

#### Scenario: A failed delivery is retried and never dropped
- **WHEN** a claimed input is failed more times than any other queue allows
- **THEN** it is still waiting, redelivered after a delay

### Requirement: An input is not redelivered while its run may still be running
A claimed input SHALL NOT be delivered a second time while its run is still
allowed to be running. The wait before the bus redelivers a conversation's input
SHALL be longer than the longest run the runtime permits.

#### Scenario: A long run is not redelivered
- **WHEN** a run lasts longer than the ack wait of any other queue
- **THEN** its input is not delivered again before the run reports

#### Scenario: A pod that dies releases its input
- **WHEN** the pod running an input is gone
- **THEN** the input is failed with a delay and delivered again, without waiting for the redelivery wait

### Requirement: Deleting a conversation purges its queue
Deleting a conversation SHALL discard the inputs still waiting in its queue and
remove the queue. Closing a conversation SHALL NOT discard them.

A purge SHALL NOT record a success for any input it discards.

#### Scenario: Delete discards waiting inputs
- **WHEN** a conversation with two waiting inputs is deleted
- **THEN** its queue no longer exists and neither input is reported as settled

#### Scenario: Close keeps waiting inputs
- **WHEN** a conversation with a waiting input is closed and then reopened
- **THEN** the input is still waiting and runs after the reopen
