# conversation-naming — delta

## ADDED Requirements

### Requirement: The correlation id never shapes the object name

`conversationId` is the adopted `initiationId` — an arbitrary sender-minted string, not a name-safe token. It SHALL NOT appear in `Conversation.metadata.name` and SHALL NOT replace the word-chain. Names stay deterministic word-chains built as this capability defines.

A recurrence after close takes the next `-N` suffix on the same base name, exactly as before. The new conversation adopts the same `initiationId` as its `conversationId`, while its NAME is numbered by the existing lookup.

#### Scenario: An adopted id does not name the object

- **WHEN** a signal with `initiationId` `{}:{alertname="NodeDown"}` and title text "NodeDown — ns-prod" opens a new conversation
- **THEN** the conversation's name is `alert-node-down-ns`, and the id is recorded on the spec rather than in the name

#### Scenario: Recurrence after close keeps the numbered suffix

- **WHEN** a signal's `initiationId` equals only a CLOSED conversation's `conversationId`, and that conversation is named `alert-node-down-ns`
- **THEN** the new conversation is named `alert-node-down-ns-2` and carries the same `conversationId`
