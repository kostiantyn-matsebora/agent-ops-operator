## ADDED Requirements

### Requirement: A Coordinator is a claimant like a Pipeline

Wherever the claimants of a source are enumerated — fan-out, `Wired`, the
unwired drop, the bare-chat choice list — a Ready Coordinator listing that
source SHALL count as one claimant beside Ready Pipelines. A bare chat message
on a source one Coordinator alone claims SHALL route to it.

#### Scenario: Coordinator among the choices
- **WHEN** a bare chat message arrives on a source a Pipeline and a Coordinator both claim
- **THEN** the choice list names both, each in its addressed `/<name> <task>` form

### Requirement: A Coordinator is addressable by name from a wired surface

`/<coordinator> <task>` typed on a surface a chat source feeds SHALL open a
root conversation for that Coordinator exactly as `/<pipeline> <task>` opens
one for a Pipeline — a plain lookup by name across both kinds, no claim check,
no Ready check. The ORIGIN SURFACE is bound at creation because a person is
waiting on it; that is escalation at origin, and the Coordinator's other
`channelRefs` bind only when it escalates. Pipelines and Coordinators share
one name space for addressing, and a Coordinator named after a manager command
is unreachable by it.

#### Scenario: A person addresses a coordinator
- **WHEN** `/incident-coordinator investigate api latency` is typed on a wired surface
- **THEN** a root conversation opens for that Coordinator with the surface bound and the text as its first input
- **AND** no other channel of the Coordinator is bound until it escalates
