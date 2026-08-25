## ADDED Requirements

### Requirement: A Coordinator is a claimant like a Pipeline

Wherever the claimants of a source are enumerated — fan-out, `Wired`, the
unwired drop, the bare-chat choice list — a Ready Coordinator listing that
source SHALL count as one claimant beside Ready Pipelines. A bare chat message
on a source one Coordinator alone claims SHALL route to it.

#### Scenario: Coordinator among the choices
- **WHEN** a bare chat message arrives on a source a Pipeline and a Coordinator both claim
- **THEN** the choice list names both
