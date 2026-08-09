## MODIFIED Requirements

### Requirement: Manager fans out agent replies on multi-channel conversations
For conversations bound to one or more channels, the run's result SHALL be fanned out as one `send` op per bound channel targeting that channel's thread. Failed or empty results SHALL fan out a short failure notice — mirrored surfaces never mistake silence for success. Composition uses the existing op pipeline only: external adapters and in-process providers require no changes.

Fan-out SHALL be recorded per bound thread in `Conversation.status`, so delivery is a durable fact rather than a queue entry. `POST /work/done` SHALL NOT be the only path that can produce the reply: reconciliation SHALL enqueue a `send` for any completed run whose result is recorded and whose bound thread carries no delivery marker. A thread already marked delivered SHALL NOT receive the reply again.

#### Scenario: One answer, every surface
- **WHEN** a run completes with a result on a conversation bound to telegram and web
- **THEN** both channels receive the result as a message in the conversation's own thread

#### Scenario: Failure is visible on every surface
- **WHEN** a run finishes with status failed and no result
- **THEN** every bound channel receives a short failure notice in the thread

#### Scenario: Restart between completion and delivery loses no answer
- **WHEN** the manager restarts after the run result was recorded but before any bound thread was marked delivered
- **THEN** reconciliation enqueues the reply for every bound thread and each receives it once

#### Scenario: Partially delivered fan-out completes without duplicating
- **WHEN** a restart interrupts fan-out after one of three bound threads was marked delivered
- **THEN** the remaining two threads receive the reply and the delivered thread receives nothing further
