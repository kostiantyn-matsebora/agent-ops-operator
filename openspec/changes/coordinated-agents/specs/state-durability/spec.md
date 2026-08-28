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

### Requirement: A conversation carries a brief, written by its agent

`Conversation.status.brief` SHALL hold what the conversation is about — the
objects it concerns and what was asked, one or two sentences a person could
recognise it by — reported by the runtime as `brief` in `/work/done` and
recorded LATEST-WINS in the same status write that records the run. A report
without it SHALL leave the stored brief unchanged. The manager SHALL parse
nothing to obtain it and SHALL read it for no decision of its own; it exists for
readers choosing a conversation without reading its transcript. The matrix
SHALL name it: Kubernetes-API state, surviving every restart.

#### Scenario: The brief follows the conversation, not the run
- **WHEN** a conversation about an ingress in `web` gains a second run that also touches its Service
- **THEN** the agent's new brief names both, replacing the earlier one, and the earlier one is not kept

#### Scenario: A runtime that reports none costs nothing
- **WHEN** a runtime completes a run with no `brief` field
- **THEN** the stored brief is unchanged, and a conversation that never had one is described by `title` alone
