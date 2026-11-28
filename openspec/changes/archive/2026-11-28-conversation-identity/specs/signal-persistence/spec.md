## MODIFIED Requirements

### Requirement: The consumer makes every routing decision

Processing SHALL be the consumer's alone, in this order, reading the
`Signal` it holds:

1. A chat signal addressing a `/<pipeline> <task>` or `/<coordinator>
   <task>` takes that target, looked up by name, as its ONE claimant — no
   claim check, no `Ready` check, no cooldown.
2. Otherwise the claimants are the source's Ready `Pipeline`s and
   `Coordinator`s, read at the first processing attempt. None → `Dropped`,
   reason `unclaimed`. A chat signal with two or more → `Dropped`, reason
   `ambiguous`.
3. Cooldown is consulted and spent, on the `SignalSource` → a suppressed
   signal ends `Dropped`, reason `cooldown`.
4. A chat signal at a full pending backlog → `Dropped`, reason `capacity`.
5. The claimants are snapshotted and each is routed.

A chat signal whose `agentops.dev/channel` names no existing `Channel` ends
`Dropped`, reason `unaddressable` — there is nowhere to answer.

A chat signal ending `Dropped` for `unclaimed`, `ambiguous` or `capacity`
SHALL be told on its originating surface. The consumer enqueues the notice on
the existing in-memory channel-operation queue, as the inbound call did
before. That is fire-and-forget: a lost notice loses no fact, because the
`Signal` records the `Dropped` reason.

Each claimant SHALL be routed idempotently by equality reuse:

- the match is an open conversation the claimant itself originated whose
  `correlationId` equals the signal's `initiationId`
- the lookup SHALL read the API server directly, never an informer cache
- one claimant's conversation never satisfies another's lookup
- a match means the conversation exists, so the consumer appends this
  signal's payload as a recurrence and creates no second one
- a repeat of an `initiationId` whose conversation is open is therefore
  routed by equality reuse, never dropped as a duplicate
- an addressed claimant's conversation adopts the `initiationId` as its
  `correlationId`, the same as every other claimant's

The append SHALL be idempotent per `Signal`. The consumer writes the
`ConversationInput` under a name derived deterministically from the
`Signal`'s name and the claimant, and an input already existing under that
name counts as delivered. A repeated pass therefore appends nothing twice.

A machine signal (`alert`, `job`, `task`) whose claimant cannot open a
conversation for a full backlog SHALL stay `Routing`, and the consumer
requeues it with a delay.

The consumer SHALL move the source's `status.receivedTotal` and
`lastReceived` for each signal it routes, retrying on conflict.

#### Scenario: An unclaimed signal drops at consumption

- **WHEN** a consumer processes a `Signal` whose source no Ready Pipeline
  or Coordinator claims
- **THEN** the pass records `Dropped` with reason `unclaimed`, and no
  conversation is created

#### Scenario: A crash between create and the status write does not duplicate

- **WHEN** a consumer creates a claimant's conversation and the leader stops
  before the status write
- **THEN** the repeated pass's direct lookup finds the open conversation that
  claimant originated whose `correlationId` equals the `initiationId`,
  creates no second conversation, and records `Routed`

#### Scenario: A repeat of an open conversation's id is appended

- **WHEN** a second `Signal` arrives whose `initiationId` equals an open
  conversation's `correlationId` for the claimant, outside the cooldown
- **THEN** its consumer appends the payload to that conversation as a
  recurrence, creates no second conversation, and records `Routed`

#### Scenario: A repeated pass appends once

- **WHEN** a consumer appends a `Signal`'s payload to a conversation and the
  leader stops before the status write, and the new leader repeats the pass
- **THEN** the pass finds the input named for that `Signal` and claimant,
  appends nothing again, and records `Routed`

#### Scenario: One claimant's conversation never answers for another

- **WHEN** two claimants share a signal and only the first has an open
  conversation with the matching `correlationId`
- **THEN** the second claimant's attempt creates its own conversation, and
  the first's is left untouched

#### Scenario: An addressed signal has the target as its one claimant

- **WHEN** a consumer processes a chat `Signal` whose payload is
  `/k8s-ops check nodes` on a source two Pipelines serve
- **THEN** `status.claimants[]` holds `k8s-ops` alone, `Routed` to a
  conversation whose `correlationId` is the `initiationId`

#### Scenario: A full backlog makes a machine signal wait

- **WHEN** a consumer processes an alert `Signal` while the pending backlog
  is full
- **THEN** its claimant stays `Routing`, the consumer requeues it with a
  delay, and a later pass routes it once the backlog drains

#### Scenario: Concurrent consumers keep the cooldown consistent

- **WHEN** two consumers process fresh signals of one source at the same time
- **THEN** both cooldown entries are recorded, neither overwriting the
  other, and a repeated `initiationId` is suppressed exactly once
