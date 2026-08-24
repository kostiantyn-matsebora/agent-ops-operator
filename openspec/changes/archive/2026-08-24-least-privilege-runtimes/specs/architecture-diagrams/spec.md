## MODIFIED Requirements

### Requirement: Claims on the diagram are checkable, and conditional ones are marked

Every numeric claim SHALL be verifiable from the repository, and the check SHALL
be recorded in the change that introduces it. At time of writing: `11` = files in
`chart/crds/`; `3` pluggable contracts = the channel, signal and work
contracts in `docs/contracts.md` (the activity contract is telemetry, not a
seam, so the label names the three); `0` Secrets = the manager grants itself no
`secrets` verbs in its RBAC; subcharts under `chart/charts/`.

The **Acts** rung SHALL be visually marked as conditional — it is granted by
wiring, and the shipped defaults do not grant it (`k8s-observability` on,
`k8s-admin` off, and the route's own account bound to nothing). A hero implying
autonomous remediation by default would be the one overclaim this diagram must
not make.

#### Scenario: A kind is added or removed

- **WHEN** the number of CRD kinds changes
- **THEN** the number on the diagram is updated in the same change

#### Scenario: A sceptical reader checks the privilege claim

- **WHEN** a reader looks at what the agent may do
- **THEN** *Acts* is distinguished from the other three rungs and states that
  the wiring must grant it
