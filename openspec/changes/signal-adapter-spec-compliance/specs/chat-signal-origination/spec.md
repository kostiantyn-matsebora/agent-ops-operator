# chat-signal-origination delta

## MODIFIED Requirements

### Requirement: Chat defaults preserve human behavior

A chat `SignalSource` SHALL default to `cooldownHours: 0`, because a person repeating themselves means it twice while a machine re-delivering an id does not. That default remains chat's own.

A chat surface applies the contract's one id rule — minted or propagated — and its two cases are instances of it, not modes of a special lane:

| Case | Instance of | The id |
|---|---|---|
| a new human request | minting — the request is its own aggregation unit | a FRESH `initiationId`, own conversation by construction |
| a person writing into a thread the manager already bound | propagation — the thread carries the manager-issued id back | the `conversationId` the manager stamped on that thread binding, passed through verbatim |

A fresh origination id SHALL be minted deterministically from the transport's own delivery identity, scoped to the matched SOURCE — never from the delivery identity alone.

The delivery identity is whichever id the transport attached to this input: the update, the message or the selection. A selection answering a surface's choice prompt mints exactly as a typed message does.

A transport's delivery id is typically scoped per sender, so two SignalSources a Pipeline shares (sources are shareable, never exclusive) could otherwise collide on the same bare id.

Scoping the mint to the source keeps a re-delivered update idempotent, while two different sources never collide.

A thread reply routes by the same equality rule as every signal: the propagated id matches the open conversation and is appended as an input.

No chat-specific routing branch and no second addressing mode SHALL exist manager-side.

#### Scenario: Repeating yourself is not dedup

- **WHEN** a user sends the same text twice on a general surface
- **THEN** two conversations are created, because each message minted its own `initiationId` and no manager-side policy merges them

#### Scenario: Chat takes the task lane

- **WHEN** a chat signal opens a conversation
- **THEN** it uses the task-lane prompt, and a later general-surface message does not resume the
  earlier session as a recurrence

#### Scenario: A thread reply echoes the stamped id

- **WHEN** a person writes into a thread the manager bound, and the surface sends the `conversationId` stamped on that binding as the signal's `initiationId`
- **THEN** equality routing appends the message to that open conversation, with no special inbound path

#### Scenario: A re-delivered update is not a second question

- **WHEN** the transport re-delivers the same update after a crash
- **THEN** the signal repeats the same `initiationId` and routes to the conversation the first delivery opened, rather than opening a second one

#### Scenario: A selection mints from its own delivery id

- **WHEN** a person answers a surface's choice prompt and the transport delivers that answer as a selection carrying its own delivery id
- **THEN** the minted `initiationId` derives from the selection's id scoped to the source, so a re-delivered selection routes to the conversation the first delivery opened

#### Scenario: Two sources never collide on the same bare delivery id

- **WHEN** two SignalSources a Pipeline shares each receive an update carrying the same transport-local delivery id
- **THEN** each mints a distinct `initiationId`, scoped to its own source, and the two signals open two separate conversations
