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

It SHALL carry the seven capability fields for the coordinating agent
itself — including `runtimeClassName` — inline or by `capabilityRef`,
with the same exclusivity a Pipeline has.

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
