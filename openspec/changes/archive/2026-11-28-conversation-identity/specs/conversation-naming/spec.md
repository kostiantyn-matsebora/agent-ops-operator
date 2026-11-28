## MODIFIED Requirements

### Requirement: The correlation id never shapes the object name

`correlationId` is the adopted `initiationId` — an arbitrary sender-minted string, not a name-safe token. It SHALL NOT appear in `Conversation.metadata.name` and SHALL NOT replace the word-chain. Names stay deterministic word-chains built as this capability defines.

A recurrence after close takes the next `-N` suffix on the same base name, exactly as before. The new conversation adopts the same `initiationId` as its `correlationId`, while its NAME is numbered by the existing lookup.

#### Scenario: An adopted id does not name the object

- **WHEN** a signal with `initiationId` `{}:{alertname="NodeDown"}` and title text "NodeDown — ns-prod" opens a new conversation
- **THEN** the conversation's name is `alert-node-down-ns`, and the id is recorded on `spec.correlationId` rather than in the name

#### Scenario: Recurrence after close keeps the numbered suffix

- **WHEN** a signal's `initiationId` equals only a CLOSED conversation's `correlationId`, and that conversation is named `alert-node-down-ns`
- **THEN** the new conversation is named `alert-node-down-ns-2` and carries the same `correlationId`

## ADDED Requirements

### Requirement: The name is display, and allocation hands back the created record

A conversation's name SHALL be what a person reads and types and the store key, and SHALL NOT be the identifier the system uses to refer to the conversation (`conversation-identity`). Allocation SHALL NOT change: the highest live suffix plus one, the create-conflict protection and the pod name `agentops-conv-<name>` are as this capability already defines.

Allocation SHALL return the record the store created, carrying its backend-minted `ID` and `CreatedAt`, to every creation path. A caller SHALL NOT receive a record whose `ID` is empty and SHALL NOT read the record back to learn it.

A name freed by a delete MAY be issued again, so the same name MAY name two conversations over time. The two SHALL differ by `ID`.

#### Scenario: A freed suffix is reissued under a new identity

- **WHEN** `alert-node-down-ns-3` is the highest live suffix, is deleted, and a new conversation is allocated from the same word-chain
- **THEN** the new conversation is named `alert-node-down-ns-3` again and its `ID` differs from the deleted one's

#### Scenario: Every creation path holds the identity

- **WHEN** a chat origination, a coordinator member creation and a signal route each allocate a conversation
- **THEN** each receives a record whose `ID` and `CreatedAt` are populated

