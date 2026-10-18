# signal-adapter-contract — delta

## ADDED Requirements

### Requirement: A missing correlation id is rejected, never synthesized

The manager SHALL NOT mint, derive or default an `initiationId` for any inbound signal. A signal arriving without one SHALL be rejected with `4xx` at `POST /signal/inbound`, the response naming the missing field.

A `1.x` adapter during rollout gets the same answer — a loud contract violation, never a silently merged conversation.

The specification's COMPAT section SHALL state this rule: there is no compatibility shim, because a manager-synthesized id would re-mint grouping policy in the manager, which this contract forbids.

Replacing `fingerprint` with `initiationId` is a MAJOR move. The specification version SHALL be `2.0.0`, and its changelog SHALL record the replacement under that version.

#### Scenario: A 1.x adapter is refused loudly

- **WHEN** an adapter posts a signal carrying the retired `fingerprint` and no `initiationId`
- **THEN** the manager responds `4xx` naming `initiationId`, creates nothing, and synthesizes no id

#### Scenario: The contract names its own break

- **WHEN** a reader opens the published specification at `2.0.0`
- **THEN** the changelog records the `fingerprint` → `initiationId` replacement under `2.0.0`, and the COMPAT section states that a missing id is rejected rather than repaired

### Requirement: The id is the sender's, never the manager's

Every signal adapter MUST supply its own `initiationId`, one rule for every lane, in one of two ways:

| Way | Means |
|---|---|
| minted | where the sender's aggregation happens — one id per "one problem" as the sender defines it, the same recurring problem reusing the same id |
| propagated | a round trip handed back a manager-issued `conversationId` — the sender or external system carries it, and the adapter passes it on VERBATIM, no re-derivation, no transformation |

The manager never determines an id and MUST treat every `initiationId` opaquely, whatever its provenance: pure equality, no parsing, no branch on the lane.

Chat is an instance of this rule, not a special lane. A fresh human request is its own aggregation unit (a fresh mint), and a reply in a bound thread is the propagation case — the thread carries the manager-issued id back. `chat-signal-origination` states both concretely.

#### Scenario: A chat reply routes by the propagated id

- **WHEN** a `chat` signal carries as `initiationId` the `conversationId` the manager stamped on an open conversation's thread binding
- **THEN** equality routing appends it to that conversation, through the same rule every other signal routes by

#### Scenario: An external system's round-trip response lands on its conversation

- **WHEN** an adapter forwards its external system's response under an id that system attached, one that originated as a `conversationId` carried out to it
- **THEN** the adapter propagates the id unchanged and equality routing appends the response to that open conversation, with no new addressing mode and no manager-side parsing

#### Scenario: The grouping decision is never the manager's

- **WHEN** any signal is posted
- **THEN** its id was supplied sender-side — minted from the sender's own aggregation or propagated verbatim from a round trip — and the manager only compares it for equality

## MODIFIED Requirements

### Requirement: Normalized signals enter through one inbound endpoint
The manager SHALL expose `POST /signal/inbound` (non-leader-gated) accepting `{source, signals: [{initiationId, labels, title?, payload?, kind?}]}` where `kind` is `alert` (default), `job`, `task`, or `chat`.

`initiationId` is the sender-minted correlation id and is REQUIRED on every signal. "These N signals are one problem" is decided wherever the id was minted — the sender or its adapter — never by the manager.

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

### Requirement: The specification is a versioned reference page

The signal adapter specification SHALL live at `docs/contracts/signal-adapter.md`
as a published reference page (front matter, permalink, a Specifications
nav entry). The page is moved there by
`runtime-specification`, and this change makes it a versioned specification.

The same change SHALL verify that `.claude/rules/documentation.md`'s routing
table has a row naming this page (the existing "Runtime/channel/signal
contract" row, `docs/contracts/<contract>.md`, does) and point
`docs/contracts.md` at it.

It SHALL carry a semver version, starting at `1.0.0`, and its own changelog
recording every version with what changed in it.

It SHALL state the manager's provisions and the adapter's obligations as two
named sections, each exhaustive for its party.

The manager's provisions SHALL include at least:

| Provision | Content |
|---|---|
| one inbound endpoint | `POST /signal/inbound` accepts every normalized signal, and it is the only way work originates from outside a chat surface |
| type-scoped derived-token auth | the master adapter token or a per-`SignalAdapter` derived token scoped to the adapter's name, with a derivation context distinct from channel adapters |
| id-equality routing and cooldown | an open conversation whose `conversationId` equals the signal's `initiationId` for the claiming pipeline receives a recurrence, else a new conversation adopts the id — and cooldown keys on `initiationId`, both manager-side |
| no Kubernetes access required | source listing, cursor state and status reporting reach the adapter through the contract's own endpoints |

The adapter's obligations SHALL include at least: normalize its transport into
signals and push them, mint `initiationId` where its aggregation already
happens and mint it deterministically so at-least-once re-sends repeat the
same id, and never emit a signal about agent-ops' own machinery, per the
self-exclusion invariant.

#### Scenario: An adapter author needs the contract

- **WHEN** an author sets out to build a signal adapter
- **THEN** `docs/contracts/signal-adapter.md` states every obligation of both
  parties
- **AND** the author needs no shipped adapter's source to build against it

#### Scenario: A normative statement changes

- **WHEN** a normative statement in the specification is added, changed or
  removed
- **THEN** the version moves as the compatibility rules direct, and the
  changelog records the change under the new version
