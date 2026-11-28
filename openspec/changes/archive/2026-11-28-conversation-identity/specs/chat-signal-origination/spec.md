## MODIFIED Requirements

### Requirement: Commands are answered without creating a conversation

A command producing only a response — `/pipelines`, `/help`, a bare
`/<pipeline>` asking for its task, or a usage error — opens and continues
nothing, so it is not a signal. The inbound endpoint SHALL answer it in
place, as today:

- no `Signal` is stored for it
- its reply reaches the originating channel
- no Conversation is created

An addressed `/<name> <task>` naming no existing Pipeline or Coordinator IS
a signal — it carries a task. Its consumer ends it `Dropped` with reason
`unclaimed`, and the consumer tells the originating surface.

`/<pipeline> <task>` SHALL create a conversation for the named PIPELINE
through the signal path, carrying that Pipeline's profile AND its
capabilities.

The named Pipeline is the `Signal`'s one claimant. Its conversation adopts
the signal's `initiationId` as its `correlationId`, so a repeated pass
finds it instead of opening a second.

The retired listing spelling `/agents` SHALL keep answering. It SHALL NOT be
printed, offered or registered anywhere.

#### Scenario: Pipeline listing

- **WHEN** a user sends `/pipelines` on the general surface
- **THEN** the listing of Ready Pipelines is posted to that channel, and no
  `Signal` and no Conversation exists for it

#### Scenario: The retired spelling still answers and is never advertised

- **WHEN** a user sends the retired `/agents`
- **THEN** the same listing is posted, while nothing agent-ops prints, offers or
  registers as a command ever names that spelling

#### Scenario: An addressed Pipeline still opens a conversation

- **WHEN** a user sends `/k8s-ops check nodes` naming a Ready Pipeline
- **THEN** a task conversation is created for `k8s-ops`, carrying its profile,
  channel set and capabilities, with the `Signal`'s `initiationId` as its
  `correlationId`

#### Scenario: A repeated pass over an addressed message opens one conversation

- **WHEN** a consumer creates the conversation of an addressed `Signal` and the
  leader stops before the status write
- **THEN** the new leader's pass finds that conversation and records it `Routed`,
  and the person sees one conversation

### Requirement: Chat defaults preserve human behavior

A chat `SignalSource` SHALL default to `cooldownHours: 0`, because a person repeating themselves means it twice while a machine re-delivering an id does not. That default remains chat's own.

A chat surface applies the contract's one id rule — minted or propagated — and its two cases are instances of it, not modes of a special lane:

| Case | Instance of | The id |
|---|---|---|
| a new human request | minting — the request is its own aggregation unit | a FRESH `initiationId`, own conversation by construction |
| a person writing into a thread the manager already bound | propagation — the thread carries the manager-issued id back | the `correlationId` the manager stamped on that thread binding, passed through verbatim |

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

- **WHEN** a person writes into a thread the manager bound, and the surface sends the `correlationId` stamped on that binding as the signal's `initiationId`
- **THEN** equality routing appends the message to that open conversation, with no special inbound path

#### Scenario: A re-delivered update is not a second question

- **WHEN** the transport re-delivers the same update after a crash
- **THEN** the signal repeats the same `initiationId` and routes to the conversation the first delivery opened, rather than opening a second one

#### Scenario: A selection mints from its own delivery id

- **WHEN** a person answers a surface's choice prompt and the transport delivers that answer as a selection (a `chat` signal whose `initiationId` is minted by the adapter, per `signal-adapter-contract`) carrying its own transport-local delivery id
- **THEN** the minted `initiationId` derives from the selection's id scoped to the source, so a re-delivered selection routes to the conversation the first delivery opened

#### Scenario: Two sources never collide on the same bare delivery id

- **WHEN** two SignalSources a Pipeline shares each receive an update carrying the same transport-local delivery id
- **THEN** each mints a distinct `initiationId`, scoped to its own source, and the two signals open two separate conversations

### Requirement: A chat message carries a signalId from its delivery identity

`signals/telegram` SHALL send a `signalId` on every chat signal. A message
here is one transport delivery: an update, or a tap on an offered control.

The `signalId` is the delivery identity scoped to the matched source.

For a message it is `tg-<source>-<update id>`. For a tap it is `tg-cb-<source>-<callback id>`. The update id and the callback id are the transport's own identities for that delivery.

Each adapter tags the ids it mints with its own tag, and this adapter's tag is `tg`.

The `signalId` is the same string a fresh `initiationId` is minted as.

For a thread reply the `initiationId` is the propagated `correlationId`
while the `signalId` stays the delivery's own, so two replies in one thread
are two messages that correlate to one conversation.

#### Scenario: A re-delivered update is one message

- **WHEN** the transport re-delivers the same update after a crash
- **THEN** the post carries the same `signalId`, and one `Signal` exists

#### Scenario: Two replies in one thread are two messages

- **WHEN** a person sends two replies in a bound thread
- **THEN** both carry the thread's `correlationId` as `initiationId` and
  different `signalId`s, and two `Signal`s exist
