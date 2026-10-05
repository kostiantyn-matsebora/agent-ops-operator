## MODIFIED Requirements

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

### Requirement: `spec.escalationChannelRefs` names the root's bound channels

The Coordinator's `channelRefs` SHALL be snapshotted as
`spec.escalationChannelRefs` onto an UNCAUSED root only. A member never
binds it (coordination-escalation).

This snapshot IS the root's bound channel set — it is read once, at
creation, so editing the Coordinator afterward changes neither the budget
nor the channels of an incident already in flight.

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
