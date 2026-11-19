## RENAMED Requirements

- FROM: `### Requirement: A channel adapter never mints a conversation id`
- TO: `### Requirement: A channel adapter never mints a correlation id`

## MODIFIED Requirements

### Requirement: Every outbound operation carries the conversation's correlation id

Each operation delivered on `/channel/ops` SHALL additionally carry the conversation's `correlationId`, beside the channel and conversation names it already carries. The id is OPAQUE to the adapter — carried, compared, never parsed.

This makes the adapter's thread mapping pure transport translation: `correlationId` on one side, the transport's thread id on the other. No adapter needs the retired signature-hash label, a `status.threads` lookup of its own, or a member-origin backref.

The operation's `conversationId` field changes meaning. It carried the signal's correlation key and now carries the conversation's system identity, while the correlation key moves to `correlationId`. That is a BREAKING change for an adapter that read it: the wire `contract` value moves from `2` to `3` and the specification document moves MAJOR. An adapter declaring `2` SHALL be refused with the existing `400` naming the version to build against, never served an operation whose `conversationId` silently means something else.

#### Scenario: An op arrives with the id

- **WHEN** an adapter receives any operation for a conversation that carries a `correlationId`
- **THEN** the operation names that id verbatim, beside the conversation and channel names

#### Scenario: An adapter that ignores the id stays conformant

- **WHEN** an adapter declaring `contract` `3` never reads the field
- **THEN** it receives and completes operations unchanged, as any conformant adapter does

#### Scenario: An outdated adapter is refused, not served

- **WHEN** an adapter declaring the retired wire `contract` value polls for operations
- **THEN** the manager refuses it with the `400` that names the version to build against

#### Scenario: A legacy conversation delivers without one

- **WHEN** an operation is delivered for a conversation created before `correlationId` existed
- **THEN** the field is absent and the operation is otherwise complete

### Requirement: A channel adapter never mints a correlation id

A channel adapter SHALL NOT create, derive or alter any correlation id. It sits downstream of signal processing: the manager-issued `correlationId` reaches it with the binding and its operations.

The adapter's only id work is transport translation — `correlationId` on one side, the thread id on the other.

An inbound message on a bound thread stays reply-only (`POST /channel/inbound`, the existing invariant that a channel CARRIES a conversation and never starts one). It is correlated by the binding's `correlationId` — the adapter attaches what it was given and mints nothing.

Origination from a chat surface stays on the signal lane, owned by `chat-signal-origination`, never on this contract.

#### Scenario: A reply carries the id it was given

- **WHEN** a person replies in a bound thread and the adapter posts it inbound
- **THEN** the message is correlated by the `correlationId` the manager issued for that binding, and the adapter has created no id of its own

#### Scenario: No origination through the channel contract

- **WHEN** a channel adapter wants a new conversation started
- **THEN** no `/channel/*` route accepts one — origination travels the signal lane, where the originating surface supplies the `initiationId`

## ADDED Requirements

### Requirement: Every outbound operation carries the conversation's identity

Each operation delivered on `/channel/ops` SHALL additionally carry `conversationId`, the conversation's system identity (`conversation-identity`), beside the display name in `conversation` and the `correlationId`. The value is OPAQUE to the adapter — carried, compared for equality, never parsed, and never shown to a person as a name.

`conversationId` is the field an adapter keys its own per-conversation state on. `conversation` stays what a person reads. A name can be reused by a later conversation and `conversationId` never is, so state keyed on the name can be inherited by a stranger.

An adapter points back at a conversation by `threadId` on `POST /channel/inbound` and `POST /channel/read`, never by name or by `conversationId`. Those requests are unchanged.

The field travels with the `contract` `3` bump of the requirement above. An adapter that does not use it SHALL remain fully conformant, ignoring it as it ignores any field it does not know.

The three names are distinct and SHALL NOT be conflated:

| Field | Is |
|---|---|
| `conversation` | the display name, reusable, what a person types |
| `conversationId` | the system identity, unique for one life of a conversation |
| `correlationId` | the signal's adopted `initiationId`, shared by many conversations, empty for a chat-originated one |

#### Scenario: An op carries all three

- **WHEN** an adapter receives an operation for a signal-originated conversation named `alert-node-down-ns-2`
- **THEN** the operation carries `conversation` as `alert-node-down-ns-2`, `conversationId` as the identity minted for that life of the name, and `correlationId` as the adopted `initiationId`

#### Scenario: A reused name is told apart

- **WHEN** a conversation is deleted, a new one is created under the same name, and an adapter still holds state keyed on the first one's `conversationId`
- **THEN** operations for the new conversation carry a different `conversationId`, and the adapter's state for the first is not applied to it

#### Scenario: An adapter at the current contract may ignore the field

- **WHEN** an adapter declaring `contract` `3` never reads `conversationId`
- **THEN** it receives and completes operations as any conformant adapter does
