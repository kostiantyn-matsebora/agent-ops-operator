## Purpose

The command bus gives a component one way to run a command, now in the calling
process or later through a queue, by the delivery mode its type is registered with.

## ADDED Requirements

### Requirement: The bus has a synchronous scenario beside its asynchronous ones
A command type SHALL be registered with one handler and a delivery mode. The mode
SHALL be one of sync, regular, convoy or hand-over. The caller SHALL NOT choose the
mode.

A sync command SHALL run its handler in the process that sent it and SHALL return the
handler's result or refusal in the same call. It SHALL NOT use the bus transport and
SHALL NOT create a task record.

A sync command is not recorded, so a repeated command id SHALL run its handler again.
A sync handler SHALL therefore be safe to run twice.

The modes map onto the queue types the bus already has:

- **regular** is a queue that declares no ordering key, so tasks of different keys are claimable in parallel.
- **convoy** is a queue that declares an ordering key, so tasks sharing a key are serialized.
- **hand-over** is a queue an adapter reads with peek and acknowledge.

These modes SHALL behave as they do without this requirement.

The requirements on task states and on the task log apply to the asynchronous
modes only. A sync command is not a task and SHALL NOT appear in either.

#### Scenario: A sync command returns the handler's result
- **WHEN** a command registered as sync is sent and its handler succeeds
- **THEN** the call returns the handler's result

#### Scenario: A sync command refuses in the call
- **WHEN** a command registered as sync is sent and its handler refuses it
- **THEN** the same call returns the refusal and no state changed

#### Scenario: A repeated sync command runs again
- **WHEN** a sync command is sent twice with the same command id
- **THEN** its handler runs twice, and nothing deduplicates the second

#### Scenario: A sync command survives a bus outage
- **WHEN** the bus transport cannot accept messages and a sync command is sent
- **THEN** the command runs and returns its result

#### Scenario: An asynchronous command is unchanged
- **WHEN** a command registered as convoy is sent
- **THEN** the call answers once the command is durably submitted, and the handler runs in the consumer pool, one at a time per key and in order

#### Scenario: A regular command is unchanged
- **WHEN** a command registered as regular is sent
- **THEN** the call answers once the command is durably submitted, and the handler runs in the consumer pool with no ordering across keys

#### Scenario: A hand-over command is unchanged
- **WHEN** a command registered as hand-over is sent
- **THEN** the message is read by an adapter, acknowledged, and reported done or failed, and the bus never redelivers it after the acknowledgement

#### Scenario: The caller cannot override the mode
- **WHEN** a caller sends a command registered as sync
- **THEN** there is no argument by which the caller makes it asynchronous
