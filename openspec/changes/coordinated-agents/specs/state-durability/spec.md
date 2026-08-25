## ADDED Requirements

### Requirement: Coordination state has a declared home

`causedBy`, the root's budget (`status.budget.agentsInvoked`, `status.budget.turns`),
`spec.escalationChannelRefs`,
a pending escalation and `closeReason` SHALL each be Kubernetes-API state on
the conversation, and the matrix SHALL name each with its restart behaviour.
The invoke-to-input path SHALL be derivable: a member run recorded as done
whose result is absent from its root's inputs is an append still owed.

#### Scenario: A restart between done and append
- **WHEN** the manager restarts after a member's run is recorded and before the root's input is written
- **THEN** reconciliation appends it exactly once
