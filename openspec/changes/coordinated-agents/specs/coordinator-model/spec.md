## Purpose
The `Coordinator` CRD is the wiring for a COMPOSITION: what feeds a coordinating agent, where it escalates to people, and the typed list of agents it may invoke.

## ADDED Requirements

### Requirement: A Coordinator claims sources and names its escalation channels

A `Coordinator` SHALL carry `signalSourceRefs`, claimed exactly as a Pipeline
claims — shareable, fanned out, counted in `Wired` — and `channelRefs`, which
are the surfaces it ESCALATES to and nothing else. It SHALL carry the six
capability fields for the coordinating agent itself, inline or by `agentRef`
with the same exclusivity a Pipeline has.

#### Scenario: A Coordinator and a Pipeline share a source
- **WHEN** a Pipeline and a Coordinator both list one source and a signal is admitted there
- **THEN** two conversations open, one per claimant, and the source's `Wired` count is two

#### Scenario: Escalation channels open no thread at admission
- **WHEN** a signal opens a Coordinator's conversation
- **THEN** no thread is created on any of its `channelRefs`

### Requirement: The agents list is the whole outbound reach

`spec.agents[]` SHALL be a list of `{name, agentRef, description}`. The
Coordinator SHALL be able to invoke exactly the Agents this list names and no
other object of any kind. `description` SHALL be required and non-empty; it
lives on the entry, so two Coordinators may describe one Agent differently.

#### Scenario: An entry without a description is refused
- **WHEN** an `agents[]` entry omits `description`
- **THEN** the API server rejects the manifest

#### Scenario: Invoking outside the list fails
- **WHEN** the coordinating agent asks to invoke an Agent its Coordinator does not list
- **THEN** the request is refused naming the Coordinator, and no conversation is created

### Requirement: Readiness names every member that is not

`Ready` SHALL be False, naming each of them, when any listed `agentRef` does not
resolve or resolves to an Agent whose own `Ready` is False. A Coordinator that
is not Ready SHALL claim nothing.

#### Scenario: A dangling member
- **WHEN** an `agents[]` entry names an Agent that does not exist
- **THEN** the Coordinator's `Ready` is False with the entry's name in its message
- **AND** signals on its sources are not routed to it

### Requirement: Limits are declared on the Coordinator and snapshotted onto the root

`spec.limits` SHALL carry `maxAgents`, `maxTurns` and `deadline`, each optional
with a chart-documented default. The values SHALL be snapshotted onto the root
conversation at creation, so editing the Coordinator does not change the budget
of an incident already in flight.

#### Scenario: A limit edit does not reach a running incident
- **WHEN** a Coordinator's `maxAgents` is lowered while one of its incidents is open
- **THEN** that incident keeps the value it was created with
