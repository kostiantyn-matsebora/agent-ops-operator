# state-durability (delta)

## MODIFIED Requirements

### Requirement: A restart-resilience matrix is documented and maintained
The documentation SHALL carry a matrix naming every component, the state it
holds, that state's declared home, and what a restart of that component costs.
Adding state to a component SHALL require adding its row.

A row whose home is the state store SHALL say what each shipped backend does
on a restart of the backend itself, since that is a restart the Kubernetes
API never had.

The matrix SHALL name `status.threads[].claim` and
`status.threads[].undeliveredReply`: state store records, surviving every
restart, with a claim held by a former leader cleared on recovery.

The matrix SHALL name the `Signal` record — its spec, its claimants and its processing state (`status.attempts`, `status.consumedBy`):

- state store records, surviving every restart and leader change
- a leader change costs no accepted signal
- the new leader resumes every non-terminal `Signal` from the state store, redoing only the interrupted pass idempotently, so a conversation or input that already landed is found and never written twice

#### Scenario: Guarantee is checkable
- **WHEN** an operator asks what restarting a given component loses
- **THEN** the answer is read from the documented matrix rather than inferred from code

#### Scenario: Claim and owed-reply state have rows
- **WHEN** an operator asks what a manager restart does to a claimed op or an undelivered reply
- **THEN** the matrix names both as conversation records in the state store, surviving the restart

#### Scenario: A backend restart has a row
- **WHEN** an operator asks what restarting the state backend costs
- **THEN** the matrix answers per shipped backend, and the `kube` row says
  "the Kubernetes API's own guarantee"

#### Scenario: Accepted signals have a row
- **WHEN** an operator asks what a manager restart or leader change does to a signal accepted but not yet fully routed
- **THEN** the matrix names the `Signal` record as a state store record, resumed by the new leader, which redoes only the interrupted pass and replays no routed work

