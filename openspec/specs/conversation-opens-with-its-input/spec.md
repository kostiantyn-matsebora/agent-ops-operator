# conversation-opens-with-its-input

## Purpose

A thread reads as the event followed by the work: every input records where it
came from, and every input is delivered to every bound channel EXCEPT the
surface that displayed it — in parallel with dispatch, once per input and
channel, so the human sees what happened even if the agent never answers.
## Requirements
### Requirement: Inputs record where they came from; the pipeline is inferred

Every input SHALL record its own origin as `{kind: signal|channel, name}` —
materialized state, snapshotted like `profileRef` and `channelRefs` and never set
by hand. It SHALL replace the job-only `jobName` field, so the originating source
is recorded for every input kind rather than for jobs alone. It SHALL be optional
so inputs created before this change remain valid.

No `pipelineRef` SHALL be introduced on the Conversation. The originating
Pipeline SHALL be INFERRED from the conversation's materialized bindings at
render time, exactly as run attribution already is, and SHALL be omitted from a
rendered message when the inference is ambiguous. A message SHALL NOT name a
pipeline that is a guess.

#### Scenario: An alert input names its source

- **WHEN** an alert from source `vm-alerts` is routed by pipeline `prod-oncall`
- **THEN** the input records `{kind: signal, name: vm-alerts}` and the rendered
  message names `prod-oncall`, taken from inference rather than a stored ref

#### Scenario: An ambiguous route is left blank, never guessed

- **WHEN** a conversation's bindings match more than one Ready pipeline
- **THEN** the message renders without a pipeline name and is otherwise complete

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

#### Scenario: A posted task explains itself

- **WHEN** a `kind: task` signal opens a conversation on a claimed source
- **THEN** the task text is delivered to the bound channels, so the topic does
  not appear without a stated cause

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

### Requirement: Input posting runs in parallel with dispatch

The input SHALL be posted once a thread binding for that channel exists, without
waiting for the agent. Posting SHALL NOT gate dispatch, and dispatch SHALL NOT
gate posting.

#### Scenario: The human reads the event while the agent works

- **WHEN** a conversation is dispatched to a runtime
- **THEN** the input card is already posted or posting concurrently, not queued
  behind the run

#### Scenario: A silent agent still leaves context

- **WHEN** a run fails, hangs, or never starts
- **THEN** the thread still contains the event that opened the conversation

#### Scenario: No card before a thread exists

- **WHEN** a conversation has no thread binding on a bound channel yet
- **THEN** nothing is posted to that channel until the binding lands, and the
  card is not lost

### Requirement: Input posting is idempotent per input and channel

Each input SHALL produce at most one message per bound channel. Operation ids
SHALL be stable per conversation, input, and channel so that reconcile-driven
re-enqueues deduplicate.

#### Scenario: Repeated reconciles post once

- **WHEN** a conversation is reconciled repeatedly while an input card is
  pending or recently completed
- **THEN** exactly one card is posted per bound channel

#### Scenario: Each bound channel gets its own copy

- **WHEN** a conversation is bound to two channels
- **THEN** each receives the input in its own thread, rendered by its own adapter
