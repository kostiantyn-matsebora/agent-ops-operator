# chat-signal-origination — delta

## REMOVED Requirements

### Requirement: Chat grouping defaults preserve human behavior

**Reason**: Signature grouping and the one-shot fingerprint rule it leaned on are retired. A chat surface now mints or echoes a correlation id, and no lane-default labels exist anywhere.

**Migration**: Replaced by "Chat defaults preserve human behavior" in this change — `cooldownHours: 0` stays chat's own, and the two chat modes carry the id.

## ADDED Requirements

### Requirement: Chat defaults preserve human behavior

A chat `SignalSource` SHALL default to `cooldownHours: 0`, because a person repeating themselves means it twice while a machine re-delivering an id does not. That default remains chat's own.

A chat surface applies the contract's one id rule — minted or propagated — and its two cases are instances of it, not modes of a special lane:

| Case | Instance of | The id |
|---|---|---|
| a new human request | minting — the request is its own aggregation unit | a FRESH `initiationId`, own conversation by construction |
| a person writing into a thread the manager already bound | propagation — the thread carries the manager-issued id back | the `conversationId` the manager stamped on that thread binding, passed through verbatim |

A fresh origination id SHALL be minted deterministically from the transport's own delivery identity (the update or message id), so a re-delivered update repeats the same id instead of posing as a second request.

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
