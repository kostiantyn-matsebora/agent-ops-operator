# coordinator-model

## Purpose
The `Coordinator` CRD is the wiring for a COMPOSITION: what feeds a coordinating agent, where it escalates to people, and the typed list of agents it may invoke.

## Requirements

### Requirement: The agents list is the whole outbound reach

`spec.agents[]` SHALL be a list of entries, each `{name, description}` plus
EITHER `capabilityRef` OR `coordinatorRef`, mutually exclusive by CEL. The
Coordinator SHALL be able to invoke exactly the objects this list names and no
other object of any kind.

- A `capabilityRef` entry invokes an AgentCapability as a plain member.
- A `coordinatorRef` entry NESTS. The invoked Coordinator's own root opens as
  the member, and the member's own `coordinatorRef` (`conversation-provenance`)
  is set to name that invoked Coordinator.
- `description` SHALL be required and non-empty, on the entry rather than the
  target, so two Coordinators may describe one differently.

#### Scenario: An entry without a description is refused
- **WHEN** an `agents[]` entry omits `description`
- **THEN** the API server rejects the manifest

#### Scenario: An entry naming both refs is refused
- **WHEN** an `agents[]` entry carries both `capabilityRef` and `coordinatorRef`
- **THEN** the API server rejects the manifest

#### Scenario: An entry naming neither ref is refused
- **WHEN** an `agents[]` entry carries neither `capabilityRef` nor `coordinatorRef`
- **THEN** the API server rejects the manifest

#### Scenario: Invoking outside the list fails
- **WHEN** the coordinating agent asks to invoke an AgentCapability or Coordinator its own Coordinator does not list
- **THEN** the request is refused naming the Coordinator, and no conversation is created

### Requirement: Readiness names every member that is not

`Ready` SHALL be False, naming each of them, when any listed `capabilityRef` or
`coordinatorRef` does not resolve, or resolves to an object whose own `Ready`
is False. A Coordinator that is not Ready SHALL claim nothing.

Resolution SHALL track the Coordinators already visited on the current path
and detect a STATIC `coordinatorRef` cycle — one reachable with no
conversation involved. A Coordinator whose own name reappears on that path
SHALL be `Ready=False` naming the cycle, rather than recursing.

This is a separate guard from the `invoke`-time one below, which walks a
live conversation's `causedBy` chain instead of the static ref graph.

#### Scenario: A dangling member
- **WHEN** an `agents[]` entry names an AgentCapability or Coordinator that does not exist
- **THEN** the Coordinator's `Ready` is False with the entry's name in its message
- **AND** signals on its sources are not routed to it

#### Scenario: Two Coordinators list each other
- **WHEN** Coordinator A's `agents[]` names a `coordinatorRef` to B, and B's own `agents[]` names one back to A
- **THEN** both A and B are `Ready=False` naming the cycle, and neither claims anything

### Requirement: Limits and escalation channels are snapshotted onto the conversation opened

The Coordinator's limits SHALL be snapshotted onto every conversation it
opens. Its `channelRefs` SHALL be snapshotted as
`spec.escalationChannelRefs` onto an UNCAUSED root only. A member never
binds it (coordination-escalation).

The field keeps its `escalation` name because it is the stored field the
snapshot rule and `escalate` already read, and renaming it would strand
every open incident. Its meaning has widened: it IS the root's bound
channel set, not only the channels an escalation opens.

The snapshot is read once, at creation, so editing the Coordinator
afterward changes neither the budget nor the channels of an incident
already in flight.

A nested Coordinator's budget snapshot is its own, independent of any
ancestor's.

#### Scenario: A limit edit does not reach a running incident
- **WHEN** a Coordinator's `maxAgents` is lowered while one of its incidents is open
- **THEN** that incident keeps the value it was created with

#### Scenario: A channel edit does not reach a running incident
- **WHEN** a Coordinator's `channelRefs` changes while one of its roots is already open
- **THEN** that root keeps the channels it was created with

#### Scenario: A nested Coordinator's budget is independent
- **WHEN** a member conversation is itself a Coordinator's root and its own `maxAgents` is reached
- **THEN** only that member and its own members close `budget-exceeded`, and its ancestors are unaffected

### Requirement: An invoke is refused when it would cycle the Coordinator graph

The manager SHALL collect the calling conversation's OWN `coordinatorRef`,
then walk its `causedBy` chain to the uncaused root collecting each
ancestor's `coordinatorRef`.

An `invoke` whose target resolves to a Coordinator already in that list
SHALL be refused as a cycle, whether the repeat is immediate or reached
through other Coordinators.

This is a live-conversation check, distinct from the static `agents[]` graph
the Readiness requirement walks.

A Coordinator BUILT self-referential — its own `agents[]` already names itself
at creation — is already `Ready=False` and never runs, so that shape never
reaches this check.

What this check catches instead is `agents[]` EDITED into a cycle after a
chain already running through it was created.

The edit turns every Coordinator on the new cycle `Ready=False` for FUTURE
admissions, but a conversation already in flight is not re-validated against
Readiness on each `invoke` — this check is what stops it completing the cycle
anyway.

Only a `coordinatorRef` entry can ever be the repeated target. A
`capabilityRef` entry names an AgentCapability, a different CRD kind that
carries no `agents[]` of its own to invoke from (`agent-capability-model`).

#### Scenario: Direct self-invoke refused
- **WHEN** a Coordinator's conversation is already running when its `agents[]` is edited to add a `coordinatorRef` entry naming that same Coordinator, and the conversation invokes it
- **THEN** the invoke is refused naming the cycle, and no conversation is created

#### Scenario: Indirect cycle refused
- **WHEN** Coordinator A invokes B, B invokes C, and C's `agents[]` is then edited to add a `coordinatorRef` back to A, which C invokes
- **THEN** the invoke is refused naming A as the repeated ancestor

### Requirement: Deleting a Coordinator cascades nothing

Deleting a Coordinator SHALL leave every conversation it started exactly as it
is — open roots keep running on their snapshot and end by budget or by hand —
as deleting a Pipeline does. No finalizer and no ownerRef ties a conversation
to its Coordinator.

#### Scenario: Coordinator deleted mid-incident
- **WHEN** a Coordinator is deleted while one of its roots has open members
- **THEN** the root and its members are unchanged, a later `escalate` opens threads on the snapshotted channels, and the budget still closes it

### Requirement: A Coordinator claims sources and names its channels

A `Coordinator` SHALL carry `signalSourceRefs`, claimed exactly as a
Pipeline claims — shareable, fanned out, counted in `Wired` — and
`channelRefs`.

`channelRefs` bind to the uncaused root at creation, the same way a
Pipeline's `channelRefs` bind to every conversation it creates
(coordination-escalation).

They are no longer escalation-only. A human may reply from the moment the
root exists. Escalating is a message posted through that already-open
channel, never what opens it.

It SHALL carry the six capability fields for the coordinating agent itself,
inline or by `capabilityRef` with the same exclusivity a Pipeline has.

#### Scenario: A Coordinator and a Pipeline share a source
- **WHEN** a Pipeline and a Coordinator both list one source and a signal is admitted there
- **THEN** two conversations open, one per claimant, and the source's `Wired` count is two

#### Scenario: A thread opens at admission, not at escalation
- **WHEN** a signal opens a Coordinator's conversation
- **THEN** a thread is created on each of its `channelRefs` at once
- **AND** a human reply there is an ordinary input before the agent ever escalates

#### Scenario: Escalation channels open no thread at admission
- **WHEN** a signal opens a Coordinator's conversation
- **THEN** threads open only on its `channelRefs`, at admission
- **AND** escalating opens no thread of its own
