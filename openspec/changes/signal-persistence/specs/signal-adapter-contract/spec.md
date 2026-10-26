# signal-adapter-contract (delta)

## MODIFIED Requirements

### Requirement: Normalized signals enter through one inbound endpoint
The manager SHALL expose `POST /signal/inbound` (non-leader-gated) accepting `{source, signals: [{initiationId, signalId?, labels, title?, payload?, kind?, severity?}]}` where `kind` is `alert` (default), `job`, `task`, or `chat`.

`initiationId` is the sender-minted correlation id and is REQUIRED on every signal. "These N signals are one problem" is decided wherever the id was minted — the sender or its adapter — never by the manager.

`severity` is OPTIONAL. When present, it SHALL be one of exactly `critical`, `error`, `warning`, or `info` — the vocabulary `signal-severity` owns. Absent SHALL mean unrated, the only state a signal carried before this field existed.

A signal carrying any other value SHALL be refused with `400` naming the four accepted values. Validation SHALL run before anything is stored, so a refused signal creates nothing.

Accepting a signal SHALL mean STORING it as a `Signal` (`signal-persistence`), never routing it. The manager routes each stored signal afterwards, by pure id equality:

- an OPEN conversation whose `conversationId` equals the signal's `initiationId`, originated by the claiming pipeline, receives the signal appended as a recurrence
- otherwise a new conversation is created, adopting the `initiationId` as its `conversationId`

The manager hosts no signal transports of its own. Every signal reaches it through this endpoint, which SHALL be the ONLY way work originates from outside a chat surface.

`kind: job` SHALL route as a job-lane input (task-style prompt) with recurrence-on-session, so later ticks resume the agent.

`kind: task` SHALL route as a task-lane input with NO recurrence-on-session, so each posted task is its own request rather than news about a standing one. Unlike `kind: chat` it SHALL NOT require a channel label, because replies reach the claiming Pipeline's channels.

`title` SHALL override the derived conversation title. Conversation binding requires the source's Ready `Pipeline` claim (the pipeline's channels + profile). A signal for an unwired source is stored, then dropped, its explicit reason recorded on its `Signal` — never on the response. Payloads SHALL be stored out-of-line (`ConversationInput`).

Delivery is at-least-once.

- A re-send carrying the same `signalId` is deduplicated at storage.
- A re-send without one is stored as a further `Signal`. The consumer makes it safe: a repeat inside the source's cooldown window is dropped, and one outside it is appended to the open conversation by equality reuse.

#### Scenario: Claimed source fans out per its pipeline
- **WHEN** an adapter posts a signal for a source claimed by a Ready Pipeline binding two channels
- **THEN** the resulting conversation carries thread bindings for both pipeline channels and uses the pipeline's profile

#### Scenario: Unwired source drops with a reason
- **WHEN** an adapter posts a signal for a source no Ready Pipeline claims
- **THEN** the response reports it queued, no conversation is created, and its `Signal` records `Dropped` with reason `unclaimed`

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
- **THEN** the manager responds 400 naming `critical`, `error`, `warning`, and `info`, and no `Signal`, conversation or input is created

#### Scenario: A valid severity is accepted
- **WHEN** a signal is posted with `severity: critical`
- **THEN** the manager accepts it and routes it exactly as a signal with no severity would route

## ADDED Requirements

### Requirement: The inbound response acknowledges storage, never routing

`POST /signal/inbound`'s success response SHALL report what the manager
STORED, and nothing it decided later:

| Status | Body |
|---|---|
| `200` | `{"queued": <n>, "reason": "<optional>"}` |
| `5xx` | `{"error": "<message>"}` — a signal could not be stored, so none of the batch is acknowledged |

- `queued` counts the batch's signals accepted: stored, or deduplicated
  against an existing `Signal` by `signalId`. Every
  admitted signal counts, whatever later routing decides.
- `reason` is present only when the source's `attributeFilters` refused
  part of the batch — the one decision made before storing.
- `conversations` SHALL NOT be returned. No conversation exists when the
  response is written.
- No routing outcome SHALL appear on the response — unclaimed, ambiguous,
  cooldown, capacity. Each is recorded on the signal's `Signal`, and a chat
  surface is told from there.

A `5xx` asks for the adapter's ordinary at-least-once re-send, with the same
`initiationId` and the same `signalId` where one was sent. A signal with a
`signalId` is deduplicated at publication. One without is stored as a new
signal, and the consumer's cooldown and equality reuse resolve it.

The specification's `API-2` is replaced by this requirement:

- no shipped adapter's obligation reads `conversations` or a routing
  `reason`, so none stops conforming — by the page's own rule that is not
  MAJOR
- the `5xx` re-send case and the optional `signalId` are the added parts, so
  the version SHALL move to `2.3.0` (MINOR), its changelog recording the
  replacement and the field
- the conformance walk is owed all the same, per "A specification change
  carries conformance"

#### Scenario: The response counts stored signals

- **WHEN** an adapter posts two signals to an unwired source
- **THEN** the response is `200` with `queued: 2`, no `conversations` and
  no `reason`

#### Scenario: A storage failure is not acknowledged

- **WHEN** the manager cannot store a signal of the batch
- **THEN** the response is `5xx` with an `error`, and a re-send of the
  batch is accepted: signals with a `signalId` that already landed are
  deduplicated, the rest are stored and any duplicate is resolved at
  consumption by cooldown or equality reuse

#### Scenario: A filtered part of the batch is reported

- **WHEN** one of two posted signals fails the source's `attributeFilters`
- **THEN** the response reports `queued: 1` with the filtered `reason`

#### Scenario: The contract records the replacement

- **WHEN** a reader opens the published specification
- **THEN** its version is `2.3.0`, and the changelog records `API-2`'s
  replacement and the removed `conversations` field under it

### Requirement: A signal may carry a per-message id

A normalized signal MAY carry `signalId`, OPTIONAL. The manager SHALL accept its absence:

| Field | Type | Required | Description |
|---|---|---|---|
| `signalId` | string | no | The sender's id for this one MESSAGE, stable across every re-send of it. 1 to 256 bytes, printable ASCII without spaces. Not the `initiationId`, which correlates many messages on purpose. |

- Present: the manager SHALL deduplicate a re-send of the message, per
  `(source, signalId)`, for as long as its stored `Signal` exists — through
  every phase, until the manager's TTL deletes it. Duplicates are
  acknowledged and counted in `queued`.
- Absent: today's behaviour. A re-send is stored again and the manager's
  cooldown and equality reuse resolve it.
- Invalid: the WHOLE batch is refused with `400` naming `signalId`.
- Unknown to an older manager: ignored, per `COMPAT-2`.

The field is additive, so by the page's own rule the change is MINOR. A
conformant `2.2.0` adapter that sends none stays conformant, which is why the
version is `2.3.0` and not MAJOR.

#### Scenario: A message re-sent after a 5xx is stored once

- **WHEN** an adapter's post is answered `5xx` after some signals landed, and
  it re-sends the batch with the same `signalId`s
- **THEN** the signals that landed are not stored again, and the rest are

#### Scenario: A signal without signalId still posts

- **WHEN** an adapter that sends no `signalId` posts a signal
- **THEN** it is accepted exactly as before

### Requirement: Every shipped signal adapter sends a stable signalId

Each shipped signal adapter — `signals/cron`, `signals/alertmanager`,
`signals/k8s-events`, `signals/ha`, `signals/pulse`, `signals/telegram` —
SHALL send a `signalId` on every signal.

The id SHALL be stable across a re-send of the same delivery, including the
re-send after a `5xx` or a transport error. It SHALL differ between two
messages the sender regards as distinct.

Each adapter's own specification states what a message is for that adapter.
The specification SHALL add `signalId` to the adapter's obligations, and its
changelog SHALL record it under `2.3.0`.

The conformance suite (`platform/manager/test/conformance/`) SHALL check that
each shipped adapter:

- sends a non-empty `signalId` within the bound on every post
- re-sends the SAME `signalId` after the fake manager answers `5xx`
- sends a different `signalId` for a different message

#### Scenario: Conformance checks the re-send

- **WHEN** the conformance suite makes the fake manager answer `5xx` to an
  adapter's first post
- **THEN** the adapter's retry carries the same `signalId` as the first post

#### Scenario: A shipped adapter without a signalId fails

- **WHEN** a shipped adapter posts a signal with no `signalId`
- **THEN** its conformance case fails
