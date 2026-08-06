# signal-adapter-contract — delta

## MODIFIED Requirements

### Requirement: Normalized signals enter through one inbound endpoint
The manager SHALL expose `POST /signal/inbound` (non-leader-gated) accepting `{source, signals: [{fingerprint, labels, title?, payload?, kind?}]}` where `kind` is `alert` (default) or `job`. For each signal the manager SHALL apply the source's grouping policy (cooldown by fingerprint, signature from `labels` × `signatureLabels`, window reuse, recurrence-on-session) through the same core as the built-in webhook; `kind: job` SHALL route as a job-lane input (task-style prompt), and `title` SHALL override the derived conversation title. **Conversation binding resolves pipeline-first: a source claimed by a Ready Pipeline produces conversations bound to the Pipeline's channels with the Pipeline's profile; an unclaimed source uses its own `channelRef`/`profileRef` as before.** Payloads SHALL be stored out-of-line (`ConversationInput`) as today. Delivery is at-least-once; duplicate fingerprints within cooldown are safely absorbed.

#### Scenario: Adapter signal creates a conversation
- **WHEN** an adapter posts a signal for an unclaimed source with a fresh fingerprint and labels matching no live conversation
- **THEN** a conversation is created with the source's own profile and channel, titled from the signal, with the payload as an out-of-line alert input

#### Scenario: Claimed source fans out per its pipeline
- **WHEN** an adapter posts a signal for a source claimed by a Ready Pipeline binding two channels
- **THEN** the resulting conversation carries thread bindings for both pipeline channels and uses the pipeline's profile

#### Scenario: Job-kind signal takes the task lane
- **WHEN** an adapter posts a signal with `kind: job`
- **THEN** the resulting input dispatches with the task/job prompt template, not the read-only investigation template

#### Scenario: Unknown source rejected
- **WHEN** `/signal/inbound` names a SignalSource that does not exist
- **THEN** the manager responds 404 and creates nothing
