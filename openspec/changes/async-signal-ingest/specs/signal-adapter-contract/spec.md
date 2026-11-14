## MODIFIED Requirements

### Requirement: The inbound response acknowledges storage, never routing

`POST /signal/inbound`'s success response SHALL report what the manager
ACCEPTED, and nothing it decided later:

| Status | Body |
|---|---|
| `202` | `{"invocationId": "<id>", "taskIds": ["<id>"], "queued": <n>, "reason": "<optional>"}` |
| `400` | `{"error": "<message>"}` — a signal was refused before enqueueing (for example an unknown `severity`), nothing is created |
| `404` | `{"error": "<message>"}` — the named SignalSource does not exist, nothing is enqueued |
| `5xx` | `{"error": "<message>"}` — a signal could not be enqueued, so the batch is not acknowledged |

```json
{
  "invocationId": "inv-4c2a",
  "taskIds": ["tsk-91e0", "tsk-91e1"],
  "queued": 2
}
```

After a `5xx` the adapter re-sends the batch. Any signal that did land is deduplicated by its `signalId`.

- `queued` counts the batch's signals accepted: each has one durably
  enqueued `ingest-signal` task. It does not say a `Signal` exists yet.
  Every admitted signal counts, whatever later processing or routing
  decides.
- `taskIds` lists one task id per accepted signal, in the order posted.
  `invocationId` names the request. An adapter MAY ignore both.
- `reason` is present only when the source's `attributeFilters` refused
  part of the batch — the one decision made before enqueueing.
- `conversations` SHALL NOT be returned. No conversation exists when the
  response is written.
- No routing outcome SHALL appear on the response — unclaimed, ambiguous,
  cooldown, capacity. Each is recorded on the signal's `Signal`, and a chat
  surface is told from there.
- An adapter SHALL treat any `2xx` as accepted, never `200` alone.

A `5xx` asks for the adapter's ordinary at-least-once re-send, with the same
`initiationId` and the same `signalId` where one was sent. A signal with a
`signalId` is deduplicated at creation. One without is stored as a new
signal, and the consumer's cooldown and equality reuse resolve it.

The success status moves from `200` to `202`, and `queued` moves from
"stored" to "accepted". The two new fields are additive:

- no shipped adapter's obligation reads the status value, `conversations` or
  a routing `reason`, and none checks for an exact `200` (verified over
  `signals/*`), so none stops conforming — by the page's own rule that
  is not MAJOR
- the new status, fields and the any-`2xx` rule are the added parts, so the
  version SHALL move to `2.4.0` (MINOR), its changelog recording the status
  change
- every shipped adapter's claimed version moves to `2.4.0` in the same
  change, per "Shipped signal adapters claim a specification version"
- the conformance walk is owed all the same, per "A specification change
  carries conformance"

#### Scenario: The response counts stored signals

- **WHEN** an adapter posts two signals to an unwired source
- **THEN** the response is `202` with `queued: 2`, two `taskIds`, no
  `conversations` and no `reason`

#### Scenario: A storage failure is not acknowledged

- **WHEN** the manager cannot enqueue a signal of the batch
- **THEN** the response is `5xx` with an `error`, and a re-send of the
  batch is accepted: signals with a `signalId` that already landed are
  deduplicated, the rest are enqueued and any duplicate is resolved at
  consumption by cooldown or equality reuse

#### Scenario: A filtered part of the batch is reported

- **WHEN** one of two posted signals fails the source's `attributeFilters`
- **THEN** the response reports `queued: 1`, one task id and the filtered
  `reason`

#### Scenario: One invalid signal refuses the whole batch

- **WHEN** a batch posts one signal with `severity: fatal` and two valid signals
- **THEN** the response is `400` and no task is enqueued for any of the three

#### Scenario: An adapter that checks only for success keeps working

- **WHEN** an adapter that treats any `2xx` as delivered posts a batch
- **THEN** the `202` reply counts as delivered and the adapter changes nothing

#### Scenario: The contract records the replacement

- **WHEN** a reader opens the published specification
- **THEN** its version is `2.4.0`, and the changelog records the `200` to
  `202` move and the two added fields

### Requirement: Normalized signals enter through one inbound endpoint
The manager SHALL expose `POST /signal/inbound` (non-leader-gated) accepting `{source, signals: [{initiationId, signalId?, labels, title?, payload?, kind?, severity?}]}` where `kind` is `alert` (default), `job`, `task`, or `chat`.

`initiationId` is the sender-minted correlation id and is REQUIRED on every signal. "These N signals are one problem" is decided wherever the id was minted — the sender or its adapter — never by the manager.

`severity` is OPTIONAL. When present, it SHALL be one of exactly `critical`, `error`, `warning`, or `info` — the vocabulary `signal-severity` owns. Absent SHALL mean unrated, the only state a signal carried before this field existed.

A signal carrying any other value SHALL be refused with `400` naming the four accepted values. Validation SHALL run before anything is enqueued, so a refused signal creates nothing.

Accepting a signal SHALL mean durably ENQUEUEING it for the `in.signal` consumer, which stores it as a `Signal` (`signal-persistence`), never routing it. The manager routes each stored signal afterwards, by pure id equality:

- an OPEN conversation whose `conversationId` equals the signal's `initiationId`, originated by the claiming pipeline, receives the signal appended as a recurrence
- otherwise a new conversation is created, adopting the `initiationId` as its `conversationId`

The manager hosts no signal transports of its own. Every signal reaches it through this endpoint, which SHALL be the ONLY way work originates from outside a chat surface.

`kind: job` SHALL route as a job-lane input (task-style prompt) with recurrence-on-session, so later ticks resume the agent.

`kind: task` SHALL route as a task-lane input with NO recurrence-on-session, so each posted task is its own request rather than news about a standing one. Unlike `kind: chat` it SHALL NOT require a channel label, because replies reach the claiming Pipeline's channels.

`title` SHALL override the derived conversation title. Conversation binding requires the source's Ready `Pipeline` claim (the pipeline's channels + profile). A signal for an unwired source is accepted, then stored and dropped, its explicit reason recorded on its `Signal` — never on the response. Payloads SHALL be stored out-of-line (`ConversationInput`).

Delivery is at-least-once.

- A re-send carrying the same `signalId` is deduplicated at storage.
- A re-send without one is stored as a further `Signal`. The consumer makes it safe: a repeat inside the source's cooldown window is dropped, and one outside it is appended to the open conversation by equality reuse.

#### Scenario: Claimed source fans out per its pipeline
- **WHEN** an adapter posts a signal for a source claimed by a Ready Pipeline binding two channels
- **THEN** the resulting conversation carries thread bindings for both pipeline channels and uses the pipeline's profile

#### Scenario: Unwired source drops with a reason
- **WHEN** an adapter posts a signal for a source no Ready Pipeline claims
- **THEN** the response reports it queued with no routing reason, no conversation is created, and its `Signal` records `Dropped` with reason `unclaimed`

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
- **THEN** the manager responds 404 and enqueues nothing

#### Scenario: An unknown severity value is refused
- **WHEN** a signal is posted with `severity: fatal`
- **THEN** the manager responds 400 naming `critical`, `error`, `warning`, and `info`, and no task, `Signal`, conversation or input is created

#### Scenario: A valid severity is accepted
- **WHEN** a signal is posted with `severity: critical`
- **THEN** the manager accepts it and routes it exactly as a signal with no severity would route
