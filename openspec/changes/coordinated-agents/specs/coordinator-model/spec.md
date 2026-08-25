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

`spec.agents[]` SHALL be a list of `{name, capabilityRef, description}`. The
Coordinator SHALL be able to invoke exactly the AgentCapabilities this list names and no
other object of any kind. `description` SHALL be required and non-empty; it
lives on the entry, so two Coordinators may describe one AgentCapability differently.

#### Scenario: An entry without a description is refused
- **WHEN** an `agents[]` entry omits `description`
- **THEN** the API server rejects the manifest

#### Scenario: Invoking outside the list fails
- **WHEN** the coordinating agent asks to invoke an AgentCapability its Coordinator does not list
- **THEN** the request is refused naming the Coordinator, and no conversation is created

### Requirement: Readiness names every member that is not

`Ready` SHALL be False, naming each of them, when any listed `capabilityRef` does not
resolve or resolves to an AgentCapability whose own `Ready` is False. A Coordinator that
is not Ready SHALL claim nothing.

#### Scenario: A dangling member
- **WHEN** an `agents[]` entry names an AgentCapability that does not exist
- **THEN** the Coordinator's `Ready` is False with the entry's name in its message
- **AND** signals on its sources are not routed to it

### Requirement: Limits and escalation channels are snapshotted onto the root

`spec.limits` SHALL carry `maxAgents`, `maxTurns` and `deadline`, each optional
with a chart-documented default. The values, and the Coordinator's
`channelRefs`, SHALL be snapshotted onto the root conversation at creation, so
editing the Coordinator does not change the budget or the escalation surfaces
of an incident already in flight.

#### Scenario: A limit edit does not reach a running incident
- **WHEN** a Coordinator's `maxAgents` is lowered while one of its incidents is open
- **THEN** that incident keeps the value it was created with

### Requirement: Deleting a Coordinator cascades nothing

Deleting a Coordinator SHALL leave every conversation it started exactly as it
is — open roots keep running on their snapshot and end by budget or by hand —
as deleting a Pipeline does. No finalizer and no ownerRef ties a conversation
to its Coordinator.

#### Scenario: Coordinator deleted mid-incident
- **WHEN** a Coordinator is deleted while one of its roots has open members
- **THEN** the root and its members are unchanged, a later `escalate` opens threads on the snapshotted channels, and the budget still closes it
