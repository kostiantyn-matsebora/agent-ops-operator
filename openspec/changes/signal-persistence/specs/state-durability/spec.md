# state-durability (delta)

## MODIFIED Requirements

### Requirement: A restart-resilience matrix is documented and maintained
The documentation SHALL carry a matrix naming every component, the state it holds, that state's declared home, and what a restart of that component costs. Adding state to a component SHALL require adding its row.

The matrix SHALL name `status.threads[].claim` and `status.threads[].undeliveredReply`: Kubernetes-API state, surviving every restart, with a claim held by a former leader cleared on recovery.

The matrix SHALL name the `Signal` record — its spec, its claimants and its processing state (`status.attempts`, `status.consumedBy`):

- Kubernetes-API state, surviving every restart and leader change
- a leader change costs no accepted signal
- the new leader resumes every non-terminal `Signal` from the CRs, redoing only the interrupted pass idempotently, so a conversation or input that already landed is found and never written twice

#### Scenario: Guarantee is checkable
- **WHEN** an operator asks what restarting a given component loses
- **THEN** the answer is read from the documented matrix rather than inferred from code

#### Scenario: Claim and owed-reply state have rows
- **WHEN** an operator asks what a manager restart does to a claimed op or an undelivered reply
- **THEN** the matrix names both as Conversation status, surviving the restart

#### Scenario: Accepted signals have a row
- **WHEN** an operator asks what a manager restart or leader change does to a signal accepted but not yet fully routed
- **THEN** the matrix names the `Signal` record as Kubernetes-API state, resumed by the new leader, which redoes only the interrupted pass and replays no routed work
