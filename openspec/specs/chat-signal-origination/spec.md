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
`SignalSource` served by a Ready `Pipeline`. A message arriving on a channel's
general surface (no thread id) SHALL be delivered as a normalized signal from a
chat `SignalSource`, not created directly by the channel router. No channel
SHALL supply a default profile, and resolution SHALL NOT fall back to any
creation-timestamp ordering among pipelines.

A source served by several Ready Pipelines fans a signal out to all of them, one
conversation each. The chat lane is the ONE exception, and only for a message
addressing no Pipeline: a person asked one question on one surface and is owed
one answer, and — unlike an alert — can say which agent they meant. So a bare
general-surface message SHALL be routed only when the answering Pipeline is
UNAMBIGUOUS:

- exactly ONE Ready Pipeline lists the chat source → the message routes to it;
- TWO OR MORE list it → no Conversation is created and the surface is answered
  with the Pipelines available and the addressed form to use;
- NONE list it → unchanged: `Wired=False` and the drop reason returns to the
  surface.

That distinction SHALL be drawn from the arriving signal's kind, which ingest
already carries. No `SignalSource` or adapter declaration SHALL be required to
mark a source as a chat lane.

An addressed message SHALL be routed by the name it carries, independent of
which Pipelines list the source and independent of how many do.

Messages arriving with a thread id SHALL be unaffected: they continue an
existing Conversation, never travel the signal path, and SHALL NOT require an
address.

#### Scenario: Bare chat message opens a conversation when one agent serves the surface

- **WHEN** a user sends `check the disk` on a channel's general surface and
  exactly one Ready Pipeline lists the paired chat `SignalSource`
- **THEN** the signal core creates the conversation with that Pipeline's profile
  and channel set, and the source's `receivedTotal` increments

#### Scenario: Ambiguous bare message is answered with the choices

- **WHEN** a user sends a bare message on a surface two Ready Pipelines serve
- **THEN** no Conversation is created, neither Pipeline is woken, and the surface
  receives the available Pipelines with the `/<pipeline> <task>` form to use

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
- **THEN** no conversation is created, the source reports `Wired=False`, and the
  user receives the drop reason on the originating surface

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

Chat input that is a command producing only a response — `/agents`, `/help`,
an unknown agent, or a usage error — SHALL emit a `send` operation to the
originating channel and SHALL NOT create a Conversation. `/<profile> <task>`
SHALL create a conversation for the named profile through the signal path.

#### Scenario: Agent listing

- **WHEN** a user sends `/agents` on the general surface
- **THEN** the profile listing is posted to that channel and no Conversation
  exists for it

#### Scenario: Addressed profile still opens a conversation

- **WHEN** a user sends `/k8s-engineer check nodes`
- **THEN** a task conversation is created for `k8s-engineer` exactly as before

### Requirement: Chat grouping defaults preserve human behavior

A chat `SignalSource` SHALL default to `cooldownHours: 0` and SHALL NOT apply
signature grouping unless explicitly configured, so each general-surface message
opens its own conversation.

The no-grouping half of that default SHALL NOT be chat-specific machinery. It is
the general rule for one-shot lanes — `chat` and `task` alike — that a source
declaring no `signatureLabels` keys on the signal's own fingerprint, while the
recurring-subject lanes (`alert`, `job`) keep the default alert labels. Chat
inherits that rule rather than owning it. The `cooldownHours: 0` default remains
chat's own, because a person repeating themselves means it twice while a machine
re-delivering a fingerprint does not.

#### Scenario: Repeating yourself is not dedup

- **WHEN** a user sends the same text twice
- **THEN** two conversations are created, not one suppressed by cooldown

#### Scenario: Chat takes the task lane

- **WHEN** a chat signal opens a conversation
- **THEN** it uses the task-lane prompt, and a later message does not resume the
  earlier session as a recurrence

#### Scenario: Chat keying is the general one-shot rule

- **WHEN** a `chat` signal and a `task` signal each arrive at a source declaring
  no `signatureLabels`
- **THEN** both key on their own fingerprint by the same rule, with no
  kind-specific branch applying to chat alone
