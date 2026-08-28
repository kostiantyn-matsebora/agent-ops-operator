## ADDED Requirements

### Requirement: Warm capacity is wiring, declared on the Pipeline
`Pipeline.spec.warm` SHALL declare how many runtime pods the manager keeps
ready for this route ahead of any signal. It is wiring because everything a
ready pod bakes in — identity, tools, MCP servers, the profile's repository and
its credential, persistence — is this Pipeline's snapshot, and none of it can
be supplied to a pod after creation. A reservation SHALL carry the snapshot
exactly as a signal-created conversation does, and a later Pipeline edit SHALL
NOT re-wire an existing reservation; the surplus or stale reservations are
replaced, never patched.

#### Scenario: A reservation carries the snapshot
- **WHEN** a Pipeline naming a service account and two MCP configs declares `warm: 1`
- **THEN** its reservation's pod runs as that account with those servers compiled, identical to a signal-created conversation's pod

#### Scenario: Editing the Pipeline replaces, never re-wires
- **WHEN** a Pipeline holding a reservation changes its `serviceAccountName`
- **THEN** the existing reservation is deleted and a new one created with the new snapshot
