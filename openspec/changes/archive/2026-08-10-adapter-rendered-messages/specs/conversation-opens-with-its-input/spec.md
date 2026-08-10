## ADDED Requirements

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

### Requirement: Bound channels receive every input the channel did not originate

When an input is appended to a conversation, it SHALL be posted to the
conversation's bound channels as a `signal` message, unless a human has already
seen it. Inputs whose origin kind is `signal` SHALL be posted; inputs whose
origin kind is `channel` SHALL NOT, since the originating surface already shows
them and sibling surfaces receive them through the existing relay. An input
carrying NO origin SHALL NOT be posted, so inputs predating this change do not
produce cards on the first reconcile after upgrade. The decision SHALL be made
from the recorded origin rather than by enumerating input types, so a new input
type inherits correct behavior.

One exception SHALL be stated rather than derived: an input originating from a
`kind: chat` signal SHALL NOT be posted. A chat message reaches the manager
through a `SignalSource` like any other signal, so its origin kind is `signal`,
but the person who typed it has already seen it on their own surface.

Posted messages SHALL name the originating source, and the pipeline when it can
be inferred.

#### Scenario: An alert thread opens with the alert

- **WHEN** an alert opens a conversation bound to a channel
- **THEN** the thread's first message is the event — its pipeline, source,
  title, labels, and payload — before the agent has produced anything

#### Scenario: A new input type inherits the rule

- **WHEN** an input type is added whose origin kind is `signal`
- **THEN** it is posted without the posting rule being edited

#### Scenario: A recurrence posts into the existing thread

- **WHEN** the same problem recurs and appends an input to an existing
  conversation
- **THEN** the recurrence is posted to the bound threads, so the thread reads as
  a log of what happened

#### Scenario: A posted task explains itself

- **WHEN** a `kind: task` signal opens a conversation on a claimed source
- **THEN** the task text is posted to the bound channels, so the topic does not
  appear without a stated cause

#### Scenario: A user's own message is not echoed

- **WHEN** a chat signal or a thread reply becomes an input
- **THEN** it is not posted back to the channel it came from, even though a chat
  signal carries a `signal` origin

#### Scenario: Inputs predating provenance produce no cards

- **WHEN** a conversation created before this change is reconciled after upgrade
- **THEN** its origin-less inputs post nothing, so upgrading does not fill open
  threads with history

#### Scenario: Siblings still see relayed user messages

- **WHEN** a user replies on one channel of a multi-channel conversation
- **THEN** the sibling channels receive it through the existing relay, not as an
  input card

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
