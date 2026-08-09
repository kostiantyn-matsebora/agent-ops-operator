## ADDED Requirements

### Requirement: Adapter closes forum topics on close-topic operations
The Telegram channel adapter SHALL serve the `close-topic` operation by closing
the corresponding forum topic through the Bot API (`closeForumTopic`) for the
chat the Channel's config names, then completing the operation with an empty
result. A Bot API failure SHALL be reported as the operation's error and SHALL
NOT be retried by the adapter — the manager treats a failed `close-topic` as
terminal.

#### Scenario: Forum topic is closed
- **WHEN** the adapter receives a `close-topic` operation for thread id `9876`
- **THEN** it calls `closeForumTopic` for that message thread and completes the operation with an empty result

#### Scenario: Bot API failure is reported, not retried
- **WHEN** `closeForumTopic` returns an error
- **THEN** the adapter completes the operation with that error and does not retry it

#### Scenario: Duplicate close is tolerated
- **WHEN** the same `close-topic` operation id is delivered twice
- **THEN** the adapter completes it without treating an already-closed topic as a failure
