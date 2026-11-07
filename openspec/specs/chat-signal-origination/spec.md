# chat-signal-origination

## Purpose

Conversations are born one way only: from a signal routed against a
`SignalSource` some Ready `Pipeline` serves. A message on a channel's general
surface is such a signal — normalized, wiring-checked and observable like an
alert or a cron job — so who answers is DECLARED by the wiring rather than
inferred from which pipeline happens to reference the channel. Where the wiring
declares SEVERAL answerers, a bare message is refused with the choices rather
than resolved by any hidden order. This capability covers the chat signal shape,
the chat-appropriate grouping defaults, and command input that produces a reply
without a conversation.

## Requirements

### Requirement: Conversations originate only from served signal sources

A Conversation SHALL be created only from a signal routed against a
`SignalSource` served by a Ready `Pipeline`.

- A general-surface message (no thread id) SHALL arrive as a normalized
  signal from a chat `SignalSource`, never created directly by the channel
  router.
- No channel SHALL supply a default profile.
- Resolution SHALL NOT fall back to any creation-timestamp ordering among
  pipelines.

Every general-surface message that can open or continue a conversation SHALL
become a `Signal` first (`signal-persistence`), bare or addressed with a
task. A command that only answers is the exception, below.

- Every decision in this requirement is the signal CONSUMER's.
- It is made on that `Signal`, never in the inbound call.
- The surface is told by the consumer, through the channel-operation queue, fire-and-forget. The `Signal` records the drop reason either way.

A source served by several Ready Pipelines fans a signal out to all of them,
one conversation each. The chat lane is the ONE exception, for a message
addressing no Pipeline: a person asked one question and is owed one answer.

A bare general-surface message SHALL be routed only when the answering
Pipeline is UNAMBIGUOUS:

| Ready Pipelines listing the chat source | Outcome |
|---|---|
| exactly one | the message routes to it |
| two or more | no Conversation, `Signal` `Dropped` with reason `ambiguous`, the surface answered with the Pipelines and the addressed form |
| none | `Wired=False`, `Signal` `Dropped` with reason `unclaimed`, the drop reason returned to the surface |

The lane is told apart by the signal's kind, which the `Signal` carries. No
`SignalSource` or adapter declaration SHALL mark a source as a chat lane.

An addressed message SHALL be routed by the name it carries, however many
Pipelines list the source.

A message arriving with a thread id SHALL be unaffected. It continues an
existing Conversation, never travels the signal path, and needs no address.

#### Scenario: Bare chat message opens a conversation when one agent serves the surface

- **WHEN** a user sends `check the disk` on a channel's general surface and
  exactly one Ready Pipeline lists the paired chat `SignalSource`
- **THEN** a consumer creates the conversation from its `Signal` with that
  Pipeline's profile and channel set, and the source's `receivedTotal`
  increments

#### Scenario: Ambiguous bare message is answered with the choices

- **WHEN** a user sends a bare message on a surface two Ready Pipelines serve
- **THEN** no Conversation is created, its `Signal` records `Dropped` with
  reason `ambiguous`, and the surface receives the available Pipelines with
  the `/<pipeline> <task>` form to use

#### Scenario: A non-chat signal on the same shared source still fans out

- **WHEN** an alert arrives on a source two Ready Pipelines serve
- **THEN** both Pipelines open a conversation, because no person is waiting on a
  single answer and no address could have been given

#### Scenario: Addressing works however many pipelines serve the surface

- **WHEN** a user addresses a Pipeline by name on a surface served by several
- **THEN** the conversation is created for the named Pipeline, with its profile
  and capabilities, and no ambiguity arises

#### Scenario: Unserved chat source drops with a reason

- **WHEN** a general-surface message arrives for a chat source no Ready Pipeline
  serves
- **THEN** no conversation is created, the source reports `Wired=False`, its
  `Signal` records `Dropped` with reason `unclaimed`, and the user receives
  the drop reason on the originating surface

#### Scenario: No timestamp tiebreak remains

- **WHEN** two Ready Pipelines serve the same chat source and a bare
  general-surface message arrives
- **THEN** it is refused as ambiguous rather than resolved by pipeline creation
  order

#### Scenario: Thread replies need no address

- **WHEN** a user replies inside a conversation's thread with no prefix, on a
  surface served by several Pipelines
- **THEN** the reply is appended to that conversation as an input, unaffected by
  this rule

#### Scenario: Topic messages are not originations

- **WHEN** a message arrives in a topic the manager does not recognize
- **THEN** no conversation is adopted or created

### Requirement: Chat signals carry their originating surface

A chat signal SHALL carry `kind: chat` and the reserved labels
`agentops.dev/channel` (the originating `Channel` name) and optionally
`agentops.dev/sender`. The manager SHALL reject a `chat` signal without
`agentops.dev/channel` rather than accept one it cannot answer.

#### Scenario: Reply lands on the originating surface

- **WHEN** a chat signal opens a conversation
- **THEN** acks and output reach the originating channel and every other channel
  bound by the claiming Pipeline

#### Scenario: Unaddressable chat signal is refused

- **WHEN** a `chat` signal arrives without `agentops.dev/channel`
- **THEN** `/signal/inbound` rejects it naming the missing label

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
the signal's `initiationId` as its `conversationId`, so a repeated pass
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
  `conversationId`

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

- **WHEN** a person answers a surface's choice prompt and the transport delivers that answer as a selection (a `chat` signal whose `initiationId` is minted by the adapter, per `signal-adapter-contract`) carrying its own transport-local delivery id
- **THEN** the minted `initiationId` derives from the selection's id scoped to the source, so a re-delivered selection routes to the conversation the first delivery opened

#### Scenario: Two sources never collide on the same bare delivery id

- **WHEN** two SignalSources a Pipeline shares each receive an update carrying the same transport-local delivery id
- **THEN** each mints a distinct `initiationId`, scoped to its own source, and the two signals open two separate conversations

### Requirement: A Coordinator is a claimant like a Pipeline

Wherever the claimants of a source are enumerated — fan-out, `Wired`, the
unwired drop, the bare-chat choice list — a Ready Coordinator listing that
source SHALL count as one claimant beside Ready Pipelines. A bare chat message
on a source one Coordinator alone claims SHALL route to it.

#### Scenario: Coordinator among the choices
- **WHEN** a bare chat message arrives on a source a Pipeline and a Coordinator both claim
- **THEN** the choice list names both, each in its addressed `/<name> <task>` form

### Requirement: A Coordinator is addressable by name from a wired surface

`/<coordinator> <task>` typed on a surface a chat source feeds SHALL open a
root conversation for that Coordinator exactly as `/<pipeline> <task>` opens
one for a Pipeline.

- It is stored as a `Signal`. Its consumer takes the Coordinator as the one
  claimant, by a plain lookup by name across both kinds — no claim check, no
  Ready check.
- The ORIGIN SURFACE is bound at creation, because a person is waiting on
  it. That is escalation at origin.
- The Coordinator's snapshotted `escalationChannelRefs` bind only when it
  escalates.
- Pipelines and Coordinators share one name space for addressing. A
  Coordinator named after a manager command is unreachable by it.

#### Scenario: A person addresses a coordinator
- **WHEN** `/incident-coordinator investigate api latency` is typed on a wired surface
- **THEN** a root conversation opens for that Coordinator with the surface bound and the text as its first input
- **AND** no other channel of the Coordinator is bound until it escalates

### Requirement: A chat message carries a signalId from its delivery identity

`signals/telegram` SHALL send a `signalId` on every chat signal. A message
here is one transport delivery: an update, or a tap on an offered control.

The `signalId` is the delivery identity scoped to the matched source:
`tg-<source>-<update id>` for a message, `tg-cb-<source>-<callback id>` for a
tap. It is the same string a fresh `initiationId` is minted as.

For a thread reply the `initiationId` is the propagated `conversationId`
while the `signalId` stays the delivery's own, so two replies in one thread
are two messages that correlate to one conversation.

#### Scenario: A re-delivered update is one message

- **WHEN** the transport re-delivers the same update after a crash
- **THEN** the post carries the same `signalId`, and one `Signal` exists

#### Scenario: Two replies in one thread are two messages

- **WHEN** a person sends two replies in a bound thread
- **THEN** both carry the thread's `conversationId` as `initiationId` and
  different `signalId`s, and two `Signal`s exist
