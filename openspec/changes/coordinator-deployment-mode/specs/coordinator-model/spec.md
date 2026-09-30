## Purpose

This change depends on `coordinated-agents` landing first. The
`coordinator-model` spec modified here is defined there and is not yet
archived.

The `Coordinator` CRD is the wiring for a COMPOSITION: what feeds a coordinating agent, where it escalates to people, and the typed list of agents it may invoke.

## MODIFIED Requirements

### Requirement: A Coordinator claims sources and names its escalation channels

A `Coordinator` SHALL carry `signalSourceRefs`, claimed exactly as a Pipeline
claims — shareable, fanned out, counted in `Wired` — and `channelRefs`, which
are the surfaces it ESCALATES to and nothing else.

It SHALL carry the six capability fields for the coordinating agent itself,
inline or by `capabilityRef` with the same exclusivity a Pipeline has.

The CHART MAY render a `Coordinator` that claims every enabled bundle's
signal source directly, in the same `signalSourceRefs` list — this is an
ordinary use of the existing claiming mechanism, not a new field or a new
claiming rule. See `wiring-mode` for when the chart does so.

#### Scenario: A Coordinator and a Pipeline share a source
- **WHEN** a Pipeline and a Coordinator both list one source and a signal is admitted there
- **THEN** two conversations open, one per claimant, and the source's `Wired` count is two

#### Scenario: Escalation channels open no thread at admission
- **WHEN** a signal opens a Coordinator's conversation
- **THEN** no thread is created on any of its `channelRefs`

#### Scenario: One Coordinator claims several bundles' sources

- **WHEN** the chart renders a Coordinator under coordinator mode with two
  bundles enabled
- **THEN** that Coordinator's `signalSourceRefs` lists both bundles' sources,
  exactly as any Coordinator claiming several sources does
