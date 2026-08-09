## MODIFIED Requirements

### Requirement: Normalized signals enter through one inbound endpoint
The manager SHALL expose `POST /signal/inbound` (non-leader-gated) accepting `{source, signals: [{fingerprint, labels, title?, payload?, kind?}]}` where `kind` is `alert` (default), `job`, `task`, or `chat`. For each signal the manager SHALL apply the source's grouping policy (cooldown by fingerprint, signature from `labels` × `signatureLabels`, window reuse, recurrence-on-session) through its single ingest core — the manager hosts no signal transports of its own; every signal reaches it through this endpoint, which SHALL be the ONLY way work originates from outside a chat surface. `kind: job` SHALL route as a job-lane input (task-style prompt) carrying the source name as its job name. `kind: task` SHALL route as a task-lane input carrying NO job name and NO recurrence-on-session, so each posted task is its own request rather than news about a standing one; unlike `kind: chat` it SHALL NOT require a channel label, because replies reach the claiming Pipeline's channels. `title` SHALL override the derived conversation title. Conversation binding requires the source's Ready `Pipeline` claim (the pipeline's channels + profile); signals for an unwired source are dropped with an explicit reason in the response. Payloads SHALL be stored out-of-line (`ConversationInput`). Delivery is at-least-once; duplicate fingerprints within cooldown are safely absorbed.

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
- **THEN** two conversations are created, each with a task-lane input and no job name — the second does not resume the first

#### Scenario: A task needs no chat surface
- **WHEN** a `kind: task` signal is posted carrying no `agentops.dev/channel` label
- **THEN** it is accepted, and the conversation binds the claiming Pipeline's channels

#### Scenario: Unknown source rejected
- **WHEN** `/signal/inbound` names a SignalSource that does not exist
- **THEN** the manager responds 404 and creates nothing

## ADDED Requirements

### Requirement: Programmatic origination has no endpoint of its own
The manager SHALL expose no HTTP route that creates a Conversation from a directly named `Pipeline`. A caller that wants to start work SHALL post a signal to a `SignalSource` a Ready Pipeline claims, so that which agent answers, on which channels, with which capabilities is decided by declared wiring rather than chosen by the caller.

#### Scenario: The task endpoint is gone
- **WHEN** a client posts to `/task` with any body
- **THEN** the manager responds 404 — the route does not exist, and no compatibility shim answers it

#### Scenario: A caller cannot pick a pipeline
- **WHEN** a caller wants a specific Pipeline to answer
- **THEN** it posts to a source that Pipeline claims; naming the Pipeline itself is not an available addressing form
