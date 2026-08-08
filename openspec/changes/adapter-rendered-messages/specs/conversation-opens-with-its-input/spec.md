## ADDED Requirements

### Requirement: Conversations record where they came from

A conversation SHALL record the Pipeline that originated it, and every input
SHALL record its own origin as `{kind: signal|api|channel, name}`. Both are
materialized state, snapshotted like `profileRef` and `channelRefs` and never
set by hand. The per-input origin SHALL replace the job-only `jobName` field, so
the originating source is recorded for every input kind rather than for jobs
alone. Both SHALL be optional so conversations with no originating pipeline, and
those created before this change, remain valid.

#### Scenario: An alert conversation names its route and source

- **WHEN** an alert from source `vm-alerts` is routed by pipeline `prod-oncall`
- **THEN** the conversation records `prod-oncall` and the input records
  `{kind: signal, name: vm-alerts}`

#### Scenario: Provenance answers who and why from the CR alone

- **WHEN** an operator inspects a conversation
- **THEN** the originating pipeline and the source of each input are readable
  without correlating logs or guessing from the signature

#### Scenario: Pipeline-less conversations stay valid

- **WHEN** a conversation is created with no originating pipeline
- **THEN** it records no pipeline reference and is otherwise unaffected

### Requirement: Bound channels receive every input the channel did not originate

When an input is appended to a conversation, it SHALL be posted to the
conversation's bound channels as a `signal` message, unless its origin is a
channel. Inputs whose origin kind is `signal` or `api` SHALL be posted; inputs
whose origin kind is `channel` SHALL NOT, since the originating surface already
shows them and sibling surfaces receive them through the existing relay. The
decision SHALL be made from the recorded origin rather than by enumerating input
types, so a new input type inherits correct behavior.

Posted messages SHALL name the originating pipeline and source.

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

#### Scenario: An API-started task explains itself

- **WHEN** a conversation is created through `POST /task`
- **THEN** the task text is posted to the bound channels, so the topic does not
  appear without a stated cause

#### Scenario: A user's own message is not echoed

- **WHEN** a chat message or a thread reply becomes an input
- **THEN** it is not posted back to the channel it came from

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
