## MODIFIED Requirements

### Requirement: Invocation is asynchronous

Invoking an AgentCapability SHALL return the created (or reused) member conversation's
identity at once and SHALL NOT wait for a result. The invoke SHALL report
whether it created a conversation or attached to an existing one.

The member's `correlationId` SHALL be minted by the invoke itself,
deterministically from the `agents[]` entry name, never from the signal
that opened the root — `conversation-provenance` records the field, this
requirement owns how it is minted.

A second invoke of the same entry attaches rather than duplicating for this
reason: the same id-equality rule every signal routes by.

#### Scenario: Attach reported
- **WHEN** a coordinator invokes the same `agents[]` entry a live member was already invoked as
- **THEN** the invoke returns that member's `ID` and reports `attached`, because both invokes mint the same id
