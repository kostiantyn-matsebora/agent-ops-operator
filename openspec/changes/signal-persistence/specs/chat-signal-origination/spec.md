# chat-signal-origination (delta)

## MODIFIED Requirements

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

- **WHEN** a worker creates the conversation of an addressed `Signal` and the
  leader stops before the status write
- **THEN** the new leader's pass finds that conversation and records it `Routed`,
  and the person sees one conversation

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
