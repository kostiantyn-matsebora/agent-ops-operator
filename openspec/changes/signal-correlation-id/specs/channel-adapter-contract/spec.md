# channel-adapter-contract — delta

## ADDED Requirements

### Requirement: Every outbound operation carries the conversation's correlation id

Each operation delivered on `/channel/ops` SHALL additionally carry the conversation's `conversationId`, beside the channel and conversation names it already carries. The id is OPAQUE to the adapter — carried, compared, never parsed.

This makes the adapter's thread mapping pure transport translation: `conversationId` on one side, the transport's thread id on the other. No adapter needs the retired signature-hash label, a `status.threads` lookup of its own, or a member-origin backref.

The field is ADDITIVE. The wire `contract` version is unchanged, an adapter that ignores the field SHALL remain fully conformant, and the specification document moves MINOR.

#### Scenario: An op arrives with the id

- **WHEN** an adapter receives any operation for a conversation that carries a `conversationId`
- **THEN** the operation names that id verbatim, beside the conversation and channel names

#### Scenario: An adapter that ignores the id stays conformant

- **WHEN** an adapter built before the field existed polls for operations
- **THEN** it receives and completes operations unchanged, declaring the same `contract` version

#### Scenario: A legacy conversation delivers without one

- **WHEN** an operation is delivered for a conversation created before `conversationId` existed
- **THEN** the field is absent and the operation is otherwise complete

### Requirement: A channel adapter never mints a conversation id

A channel adapter SHALL NOT create, derive or alter any conversation id. It sits downstream of signal processing: the manager-issued `conversationId` reaches it with the binding and its operations.

The adapter's only id work is transport translation — `conversationId` on one side, the thread id on the other.

An inbound message on a bound thread stays reply-only (`POST /channel/inbound`, the existing invariant that a channel CARRIES a conversation and never starts one). It is correlated by the binding's `conversationId` — the adapter attaches what it was given and mints nothing.

Origination from a chat surface stays on the signal lane, owned by `chat-signal-origination`, never on this contract.

#### Scenario: A reply carries the id it was given

- **WHEN** a person replies in a bound thread and the adapter posts it inbound
- **THEN** the message is correlated by the `conversationId` the manager issued for that binding, and the adapter has created no id of its own

#### Scenario: No origination through the channel contract

- **WHEN** a channel adapter wants a new conversation started
- **THEN** no `/channel/*` route accepts one — origination travels the signal lane, where the originating surface supplies the `initiationId`
