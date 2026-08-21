## MODIFIED Requirements

### Requirement: Bound channels receive every input the destination has not already seen

When an input is appended to a conversation, it SHALL be delivered to every
bound channel EXCEPT the surface it entered on.

The decision SHALL be made per DESTINATION, never once per message. "Already
seen" is a fact about a surface: a message typed on surface A was displayed by A
when it was typed, and is new to every other bound channel. Deciding once, from
the input's origin KIND, is what withheld a person's words from channels that
had never shown them.

The rule SHALL be read off the origin SURFACE — the source or channel the input
entered through — so a new input kind inherits correct behavior without the rule
being edited. There SHALL be no separate clause for chat: a `kind: chat` signal
entered on a surface like any other message, and that surface is the one
destination it is not delivered to.

An input carrying NO origin SHALL be delivered nowhere, so inputs predating
provenance do not fill open threads on the first reconcile after upgrade.

Delivered messages SHALL name where they came from, and the pipeline when it can
be inferred.

#### Scenario: An alert thread opens with the alert
- **WHEN** an alert opens a conversation bound to a channel
- **THEN** the thread's first message is the event — its pipeline, source,
  title, labels, and payload — before the agent has produced anything

#### Scenario: A person's message reaches the surfaces that did not show it
- **WHEN** a person sends a message on surface A of a conversation bound to A and B
- **THEN** B receives it attributed to its sender, and A receives nothing,
  because A displayed it when it was typed

#### Scenario: A single-surface viewer receives its own users' messages
- **WHEN** a person sends a message on a surface that does not display it itself
- **THEN** that surface receives it like any other destination, because whether
  a transport echoes is a fact about the transport and not about the message

#### Scenario: A chat message needs no special case
- **WHEN** a `kind: chat` signal opens a conversation bound to several channels
- **THEN** every bound channel except the originating surface receives it, with
  no rule naming the chat lane

#### Scenario: A new input type inherits the rule
- **WHEN** an input type is added
- **THEN** it is delivered by the same per-destination rule, unedited

#### Scenario: A recurrence posts into the existing thread
- **WHEN** the same problem recurs and appends an input to an existing
  conversation
- **THEN** the recurrence is delivered to the bound threads, so the thread reads
  as a log of what happened

#### Scenario: Inputs predating provenance are delivered nowhere
- **WHEN** a conversation created before provenance existed is reconciled
- **THEN** its origin-less inputs deliver nothing

#### Scenario: A source nobody reads still reaches every channel
- **WHEN** an input enters on a source that is no channel's surface
- **THEN** every bound channel receives it, because none of them displayed it
