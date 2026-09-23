## Purpose
The `Coordinator` CRD is the wiring for a COMPOSITION: what feeds a coordinating agent, where it escalates to people, and the typed list of agents it may invoke.

## ADDED Requirements

### Requirement: A Coordinator claims sources and names its escalation channels

A `Coordinator` SHALL carry `signalSourceRefs`, claimed exactly as a Pipeline
claims — shareable, fanned out, counted in `Wired` — and `channelRefs`, which
are the surfaces it ESCALATES to and nothing else. It SHALL carry the six
capability fields for the coordinating agent itself, inline or by `capabilityRef`
with the same exclusivity a Pipeline has.

#### Scenario: A Coordinator and a Pipeline share a source
- **WHEN** a Pipeline and a Coordinator both list one source and a signal is admitted there
- **THEN** two conversations open, one per claimant, and the source's `Wired` count is two

#### Scenario: Escalation channels open no thread at admission
- **WHEN** a signal opens a Coordinator's conversation
- **THEN** no thread is created on any of its `channelRefs`

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

#### Scenario: A dangling member
- **WHEN** an `agents[]` entry names an AgentCapability or Coordinator that does not exist
- **THEN** the Coordinator's `Ready` is False with the entry's name in its message
- **AND** signals on its sources are not routed to it

### Requirement: Limits and escalation channels are snapshotted onto the conversation opened

`spec.limits` SHALL carry `maxAgents`, `maxTurns` and `deadline`, each optional
with a chart-documented default. The values SHALL be snapshotted onto the
conversation it opens at creation, whether that conversation is an uncaused
root or itself a member.

A nested Coordinator's budget snapshot is its own, independent of any
ancestor's.

The Coordinator's `channelRefs` SHALL be snapshotted as
`spec.escalationChannelRefs` onto an UNCAUSED root only. A member never binds
it (coordination-escalation).

Editing the Coordinator changes neither the budget of an incident already in
flight nor where the uncaused root would escalate.

#### Scenario: A limit edit does not reach a running incident
- **WHEN** a Coordinator's `maxAgents` is lowered while one of its incidents is open
- **THEN** that incident keeps the value it was created with

#### Scenario: A nested Coordinator's budget is independent
- **WHEN** a member conversation is itself a Coordinator's root and its own `maxAgents` is reached
- **THEN** only that member and its own members close `budget-exceeded`, and its ancestors are unaffected

### Requirement: An invoke is refused when it would cycle the Coordinator graph

The manager SHALL collect the calling conversation's OWN `coordinatorRef`,
then walk its `causedBy` chain to the uncaused root collecting each
ancestor's.

An `invoke` whose target resolves to a Coordinator already in that list
SHALL be refused as a cycle, whether the repeat is immediate or reached
through other Coordinators.

#### Scenario: Direct self-invoke refused
- **WHEN** a Coordinator's own conversation asks to invoke a Coordinator wired to itself
- **THEN** the invoke is refused naming the cycle, and no conversation is created

#### Scenario: Indirect cycle refused
- **WHEN** Coordinator A invokes B, B invokes C, and C asks to invoke A
- **THEN** the invoke is refused naming A as the repeated ancestor

### Requirement: Deleting a Coordinator cascades nothing

Deleting a Coordinator SHALL leave every conversation it started exactly as it
is — open roots keep running on their snapshot and end by budget or by hand —
as deleting a Pipeline does. No finalizer and no ownerRef ties a conversation
to its Coordinator.

#### Scenario: Coordinator deleted mid-incident
- **WHEN** a Coordinator is deleted while one of its roots has open members
- **THEN** the root and its members are unchanged, a later `escalate` opens threads on the snapshotted channels, and the budget still closes it
