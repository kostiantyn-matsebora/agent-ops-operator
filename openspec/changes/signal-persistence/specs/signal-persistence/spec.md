## Purpose

The `Signal` CRD (`agentops.dev/v1alpha1`) and its lifecycle: persist-then-ack
ingest, absorb by `initiationId`, the per-claimant routing workflow, the
derived phase roll-up, bounds and GC, and routing that is always resumable
from the CR alone.

## ADDED Requirements

### Requirement: A signal is persisted before it is acknowledged

The manager SHALL create a `Signal` CR for every accepted inbound signal
before writing the HTTP response. The `202` acknowledgement SHALL be sent
only after the CR exists. A signal whose CR cannot be written SHALL NOT be
acknowledged as accepted.

Each `Signal` SHALL carry an ownerRef to its `SignalSource`.

The `Signal` carries the payload only until it routes. On routing, the
payload is handed off out-of-line as a `ConversationInput` — the
conversation's own input record, never the `Signal`, is the routed copy.

#### Scenario: The CR exists before the acknowledgement

- **WHEN** an adapter posts an accepted signal to the inbound endpoint
- **THEN** a `Signal` CR owned by the named `SignalSource` exists before the
  `202` response is written

#### Scenario: A signal that cannot be persisted is not acknowledged

- **WHEN** writing the `Signal` CR fails
- **THEN** the response reports the failure instead of `202`, so the
  adapter's at-least-once re-send repeats the same `initiationId` safely

#### Scenario: Routing hands the payload off

- **WHEN** a `Signal` routes to a claimant's conversation
- **THEN** its payload lands on that conversation as a `ConversationInput`,
  through the same out-of-line storage every input uses

### Requirement: A repeat of an open signal is absorbed, never duplicated

A signal arriving whose `initiationId` equals a non-terminal `Signal` of the
same source SHALL be absorbed into that CR: the payload is appended and the
occurrence count is incremented. The manager SHALL NOT create a second CR
for it.

A terminal `Signal` is an audit record and SHALL never be reopened. A repeat
arriving after its match went terminal is a fresh signal.

#### Scenario: A repeat is absorbed into the open Signal

- **WHEN** a signal arrives whose `initiationId` equals a non-terminal
  `Signal` of the same source
- **THEN** that CR gains the appended payload and an incremented occurrence
  count, and no second `Signal` is created

#### Scenario: A terminal Signal absorbs nothing

- **WHEN** a signal arrives whose `initiationId` matches only a terminal
  `Signal` of its source
- **THEN** a new `Signal` CR is created, and the terminal one is unchanged

### Requirement: One routing path serves ingest and the reconciler

Ingest SHALL perform the inline first routing attempt of the `Signal` it
just persisted, before writing the response, so the response keeps today's synchronous
semantics — including the explicit unwired-drop reason — and the signal
adapter contract does not move.

Every later attempt SHALL be a reconciler's: retries, and resumption after a
crash or a leader change. Both SHALL go through one shared routing path.

No routing decision SHALL come from process memory that was not just read
from a `Signal` CR.

#### Scenario: The response still carries the drop reason

- **WHEN** an adapter posts a signal for a source no Ready Pipeline claims
- **THEN** the synchronous response reports the explicit not-wired reason, as
  it does today, and the `Signal` records the same outcome

#### Scenario: Retries are the reconciler's, not the adapter's

- **WHEN** routing to a claimant fails during the inline first attempt
- **THEN** the reconciler retries that claimant through the same routing
  path, with no further HTTP request involved

#### Scenario: Memory is never the record

- **WHEN** leadership moves to a replica that shared no memory with the old
  leader
- **THEN** every routing decision the new leader makes is derived from
  `Signal` CRs it reads, and none depends on the old leader's process state

### Requirement: Routing is tracked per claimant

At first routing attempt the manager SHALL snapshot the claimant set into
`status.claimants[]`: one entry per claimant, carrying the claimant's ref
(kind and name) and a state.

| State | Means |
|---|---|
| `Routing` | the initial, non-terminal state |
| `Routed` | terminal — the entry records the conversation name |
| `Failed` | terminal — the entry records the failure reason verbatim |
| `Skipped` | terminal — routing was deliberately not attempted |

The snapshot SHALL never grow or shrink after first routing attempt. A re-wire never
changes an in-flight signal's fan-out.

Retries SHALL touch only non-terminal claimants.

A retry SHALL be idempotent by equality reuse: an open conversation whose
`conversationId` equals the signal's `initiationId` for that claimant means
the earlier attempt landed, so the retry appends rather than duplicates.

#### Scenario: A re-wire does not reach an in-flight signal

- **WHEN** a Pipeline claims the source after a `Signal`'s first routing attempt
- **THEN** the `Signal`'s claimant set is unchanged, and only later
  signals fan out to the new claimant

#### Scenario: A retry leaves terminal claimants alone

- **WHEN** a `Signal` has one `Routed` claimant and one `Routing`
  claimant when the reconciler retries
- **THEN** only the `Routing` claimant is attempted, and the
  `Routed` entry and its conversation are untouched

#### Scenario: The outcome is readable per claimant

- **WHEN** a claimant reaches a terminal state
- **THEN** its entry names the conversation it routed to, or carries the failure
  reason verbatim

### Requirement: The phase is a roll-up derived from the claimants

A `Signal` SHALL report a phase recomputed from `status.claimants[]` on
every status write, never authored independently of them.

| Phase | Means |
|---|---|
| `Pending` | not yet routed, or waiting on conversation capacity |
| `Routed` | every claimant terminal, at least one `Routed` |
| `PartiallyRouted` | at least one `Routed`, at least one non-terminal |
| `Dropped` | terminal before fan-out — unclaimed source, cooldown suppression, overflow eviction, or a chat signal declined at capacity, with the reason recorded |
| `Failed` | every claimant terminal, none `Routed` |

`Failed` SHALL be surfaced as loudly as the unwired drop: an activity event
and a log line, never a silent status.

#### Scenario: Full routing rolls up to Routed

- **WHEN** a `Signal`'s last non-terminal claimant becomes `Routed`
- **THEN** the same status write reports phase `Routed`

#### Scenario: Total failure is loud

- **WHEN** every claimant of a `Signal` is terminal and none is
  `Routed`
- **THEN** the phase is `Failed`, an activity event is emitted, and the
  failure is logged

#### Scenario: A drop records why

- **WHEN** a `Signal` goes terminal before fan-out
- **THEN** its phase is `Dropped` and the recorded reason names which of
  unclaimed, cooldown, overflow or capacity ended it

### Requirement: A partially routed signal exits

Retries against a non-terminal claimant SHALL be bounded by a retry
budget or deadline. When the bound is exhausted, the manager SHALL flip that
claimant to `Failed` with the exhaustion recorded as its reason.

No `Signal` SHALL sit in a non-terminal phase indefinitely.

#### Scenario: An exhausted claimant goes terminal

- **WHEN** a `Routing` claimant exhausts its retry budget while another
  claimant is already `Routed`
- **THEN** the exhausted claimant becomes `Failed` with the reason
  recorded, and the phase rolls up to `Routed`

#### Scenario: Exhaustion with nothing routed fails the signal

- **WHEN** every claimant exhausts its retry budget and none is
  `Routed`
- **THEN** the `Signal` reports phase `Failed`, with the loud surfacing that
  phase requires

### Requirement: Live signals are bounded and terminal ones are collected

The manager SHALL cap the non-terminal `Signal` CRs per source. Absorb by
`initiationId` is the primary enforcement — a recurring problem occupies one
CR however often it fires.

When the cap is still exceeded, the manager SHALL evict the OLDEST
non-terminal `Signal`, counted and logged — degrading gracefully, as the
bounded cooldown record already does:

- a `Signal` that has not fanned out becomes `Dropped` with reason overflow
- a `Signal` that has fanned out has each non-terminal claimant flipped to
  `Failed` with reason overflow, and the phase rolls up from the claimants
  as always — `Dropped` stays a strictly pre-fan-out phase

Terminal `Signal` CRs SHALL be garbage-collected after a TTL. The TTL for
`Dropped` SHALL be shorter than the TTL for `Routed`, because the routed
record is the audit trail. `Failed` SHALL use the `Routed` TTL — a signal
nobody routed is exactly the record an operator investigates.

Deleting a `SignalSource` SHALL garbage-collect its `Signal` CRs through the
ownerRef, whatever phase each is in.

#### Scenario: Overflow evicts the oldest, loudly

- **WHEN** a fresh signal would exceed its source's cap on non-terminal
  `Signal` CRs
- **THEN** the oldest non-terminal `Signal` is evicted, counted and logged
  — `Dropped` with reason overflow, since it never fanned out

#### Scenario: Overflow after fan-out exhausts claimants, never re-drops

- **WHEN** the eviction candidate is `PartiallyRouted`, one claimant `Routed`
  and one `Routing`
- **THEN** the `Routing` claimant becomes `Failed` with reason overflow and
  the phase rolls up to `Routed`, never to `Dropped`

#### Scenario: Failed is collected on the audit TTL

- **WHEN** a `Failed` and a `Dropped` `Signal` go terminal at the same moment
- **THEN** the `Dropped` one is collected first, and the `Failed` one on the
  `Routed` TTL

#### Scenario: The audit trail outlives the drop record

- **WHEN** a `Dropped` and a `Routed` `Signal` go terminal at the same moment
- **THEN** the `Dropped` one is collected first, on its shorter TTL

#### Scenario: A deleted source takes its signals

- **WHEN** a `SignalSource` is deleted while `Signal` CRs of every phase
  reference it
- **THEN** all of them are garbage-collected through the ownerRef

### Requirement: Only the manager writes a Signal

No component other than the manager SHALL write a `Signal` — create, update,
status or delete. Every other component, the console included, SHALL hold
read-only access at most.

#### Scenario: The console reads without writing

- **WHEN** the console renders signals and their per-claimant state
- **THEN** it does so through watch and read access alone, holding no write
  verb on the kind

#### Scenario: A non-manager write is refused

- **WHEN** any component other than the manager attempts to write a `Signal`
- **THEN** the cluster's authorization refuses it, because no grant beyond
  the manager's exists

### Requirement: A new leader resumes signals without replaying routed work

On gaining leadership the manager SHALL list non-terminal `Signal` CRs and
resume each through the shared routing path. Resumption SHALL replay no
routing that already landed.

#### Scenario: A crash between create and status write does not duplicate

- **WHEN** the manager crashes after creating a claimant's conversation
  but before writing that claimant's `Routed` entry
- **THEN** the new leader's retry finds the open conversation whose
  `conversationId` equals the `initiationId` for that claimant, appends
  rather than creating a second conversation, and records `Routed`

### Requirement: Cooldown suppression is recorded, never silent

Cooldown SHALL still be consulted at routing time, and SHALL keep its key and
its persistence on the `SignalSource` — this change moves neither.

A cooldown-suppressed signal SHALL become `Dropped` with reason cooldown,
rather than vanishing without a record.

#### Scenario: A suppressed signal leaves a record

- **WHEN** a signal is routed whose `initiationId` was admitted on the same
  source inside its cooldown window
- **THEN** no conversation or input is created, and the `Signal` reports
  `Dropped` with reason cooldown
