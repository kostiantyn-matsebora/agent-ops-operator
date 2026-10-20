## MODIFIED Requirements

### Requirement: Normalized signals enter through one inbound endpoint
The manager SHALL expose `POST /signal/inbound` (non-leader-gated) accepting `{source, signals: [{initiationId, labels, title?, payload?, kind?, severity?}]}` where `kind` is `alert` (default), `job`, `task`, or `chat`.

`initiationId` is the sender-minted correlation id and is REQUIRED on every signal. "These N signals are one problem" is decided wherever the id was minted — the sender or its adapter — never by the manager.

`severity` is OPTIONAL. When present, it SHALL be one of exactly `critical`, `error`, `warning`, or `info` — the vocabulary `signal-severity` owns. Absent SHALL mean unrated, the only state a signal carried before this field existed.

A signal carrying any other value SHALL be refused with `400` naming the four accepted values. Validation SHALL run before any routing, so a refused signal creates nothing.

The manager SHALL route each signal by pure id equality through its single ingest core:

- an OPEN conversation whose `conversationId` equals the signal's `initiationId`, originated by the claiming pipeline, receives the signal appended as a recurrence
- otherwise a new conversation is created, adopting the `initiationId` as its `conversationId`

The manager hosts no signal transports of its own. Every signal reaches it through this endpoint, which SHALL be the ONLY way work originates from outside a chat surface.

`kind: job` SHALL route as a job-lane input (task-style prompt) with recurrence-on-session, so later ticks resume the agent.

`kind: task` SHALL route as a task-lane input with NO recurrence-on-session, so each posted task is its own request rather than news about a standing one. Unlike `kind: chat` it SHALL NOT require a channel label, because replies reach the claiming Pipeline's channels.

`title` SHALL override the derived conversation title. Conversation binding requires the source's Ready `Pipeline` claim (the pipeline's channels + profile). Signals for an unwired source are dropped with an explicit reason in the response. Payloads SHALL be stored out-of-line (`ConversationInput`).

Delivery is at-least-once. A duplicate `initiationId` within the source's cooldown is safely absorbed.

#### Scenario: Claimed source fans out per its pipeline
- **WHEN** an adapter posts a signal for a source claimed by a Ready Pipeline binding two channels
- **THEN** the resulting conversation carries thread bindings for both pipeline channels and uses the pipeline's profile

#### Scenario: Unwired source drops with a reason
- **WHEN** an adapter posts a signal for a source no Ready Pipeline claims
- **THEN** nothing is created and the response reports queued 0 with a not-wired reason

#### Scenario: A repeated id lands on its open conversation
- **WHEN** a signal arrives whose `initiationId` equals an open conversation's `conversationId` for the claiming pipeline, outside the source's cooldown
- **THEN** the signal is appended to that conversation as a recurrence, and no new conversation is created

#### Scenario: A fresh id opens a fresh conversation
- **WHEN** a signal arrives whose `initiationId` matches no open conversation for the claiming pipeline
- **THEN** a new conversation is created with `conversationId` set to that id

#### Scenario: Distinct ids never merge
- **WHEN** two signals with different `initiationId` values arrive on one source while both conversations are open
- **THEN** each routes to its own conversation, and neither is appended to the other's

#### Scenario: Job-kind signal takes the task lane
- **WHEN** an adapter posts a signal with `kind: job`
- **THEN** the resulting input dispatches with the task/job prompt template, not the read-only investigation template

#### Scenario: Task-kind signal opens its own conversation
- **WHEN** two `kind: task` signals with different `initiationId` values are posted to one source
- **THEN** two conversations are created, each with a task-lane input — the second does not resume the first

#### Scenario: A task needs no chat surface
- **WHEN** a `kind: task` signal is posted carrying no `agentops.dev/channel` label
- **THEN** it is accepted, and the conversation binds the claiming Pipeline's channels

#### Scenario: Unknown source rejected
- **WHEN** `/signal/inbound` names a SignalSource that does not exist
- **THEN** the manager responds 404 and creates nothing

#### Scenario: An unknown severity value is refused
- **WHEN** a signal is posted with `severity: fatal`
- **THEN** the manager responds 400 naming `critical`, `error`, `warning`, and `info`, and no conversation or input is created

#### Scenario: A valid severity is accepted
- **WHEN** a signal is posted with `severity: critical`
- **THEN** the manager accepts it and routes it exactly as a signal with no severity would route
