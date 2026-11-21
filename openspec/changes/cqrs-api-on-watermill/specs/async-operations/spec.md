## MODIFIED Requirements

### Requirement: These operations stay synchronous
The following SHALL stay synchronous to the caller, for the reasons given. The
caller's call SHALL return the outcome. An operation MAY run as a command with a
reply, and SHALL NOT answer accepted for work that has not been decided.

| Operation | Why it stays synchronous |
|---|---|
| reads, streams and sessions | they return state and have no effect to defer |
| `invoke` | its reply carries the member and the budget reservation, its refusals must be immediate, and an accepted-then-failed `invoke` would still count as a move |
| the coordinator reads | they return state |
| the runtime's `/work`, `/work/done` and `/work/context` | the move-per-turn judgement and the corrective input are inside `/work/done` and need an immediate answer |
| the consumer ends of `/channel/ops`, `/channel/ops/{id}/ack`, `/channel/ops/{id}/done` and `/channel/ops/stream` | the read, the hand-over and its report are the bus's consumer |
| adapter state and status | small writes the caller reads back |
| `/activity` | telemetry with no durable effect |
| mark read and mark unread | monotonic and cheap |
| the accept step | it is what makes the rest durable |

#### Scenario: Invoke refuses at once
- **WHEN** a Coordinator's agent calls `invoke` and its budget is spent
- **THEN** the call is refused in the same reply and no member exists

#### Scenario: The runtime protocol is unchanged
- **WHEN** a runtime reports a run through `/work/done`
- **THEN** the manager answers within the call, including any corrective input
