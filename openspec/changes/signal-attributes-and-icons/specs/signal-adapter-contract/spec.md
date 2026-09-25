## MODIFIED Requirements

### Requirement: Normalized signals enter through one inbound endpoint
The manager SHALL expose `POST /signal/inbound` (non-leader-gated) accepting `{source, signals: [{fingerprint, labels, title?, payload?, kind?, severity?}]}`.

- `kind` is `alert` (default), `job`, `task`, or `chat`.
- `severity` is `critical`, `error`, `warning` or `info`, or absent for unrated. Any other value SHALL be refused with 400 naming the vocabulary, and nothing SHALL be created from that batch.
- For each signal the manager SHALL apply the source's grouping policy (cooldown by fingerprint, signature from `labels` × `signatureLabels`, window reuse, recurrence-on-session) through its single ingest core.
- The manager hosts no signal transports of its own. Every signal reaches it through this endpoint, which SHALL be the ONLY way work originates from outside a chat surface.
- `kind: job` SHALL route as a job-lane input (task-style prompt) with recurrence-on-session, so later ticks resume the agent.
- `kind: task` SHALL route as a task-lane input with NO recurrence-on-session, so each posted task is its own request. Unlike `kind: chat` it SHALL NOT require a channel label, because replies reach the claiming Pipeline's channels.
- `title` SHALL override the derived conversation title.
- Conversation binding requires the source's Ready `Pipeline` claim (the pipeline's channels + profile), filtered by the claim's and the bindings' matchers. Signals for an unwired source are dropped with an explicit reason in the response, and so are signals no claim's matcher takes.
- Payloads SHALL be stored out-of-line (`ConversationInput`), with the signal's severity beside them.
- Delivery is at-least-once. Duplicate fingerprints within cooldown are safely absorbed.

#### Scenario: Claimed source fans out per its pipeline
- **WHEN** an adapter posts a signal for a source claimed by a Ready Pipeline binding two channels
- **THEN** the resulting conversation carries thread bindings for both pipeline channels and uses the pipeline's profile

#### Scenario: Unwired source drops with a reason
- **WHEN** an adapter posts a signal for a source no Ready Pipeline claims
- **THEN** nothing is created and the response reports queued 0 with a not-wired reason

#### Scenario: Job-kind signal takes the task lane
- **WHEN** an adapter posts a signal with `kind: job`
- **THEN** the resulting input dispatches with the task/job prompt template, not the read-only investigation template

#### Scenario: Task-kind signal opens its own conversation
- **WHEN** two `kind: task` signals with different fingerprints are posted to one source declaring no `signatureLabels`
- **THEN** two conversations are created, each with a task-lane input — the second does not resume the first

#### Scenario: A task needs no chat surface
- **WHEN** a `kind: task` signal is posted carrying no `agentops.dev/channel` label
- **THEN** it is accepted, and the conversation binds the claiming Pipeline's channels

#### Scenario: Unknown source rejected
- **WHEN** `/signal/inbound` names a SignalSource that does not exist
- **THEN** the manager responds 404 and creates nothing

#### Scenario: A severity outside the vocabulary is rejected
- **WHEN** a batch carries a signal with `severity: high`
- **THEN** the manager responds 400 naming `critical`, `error`, `warning` and `info`, and creates nothing

#### Scenario: A rated signal is stored rated
- **WHEN** a signal with `severity: error` is admitted
- **THEN** its ConversationInput records `error`, and the conversation it opened records `error` in its provenance
