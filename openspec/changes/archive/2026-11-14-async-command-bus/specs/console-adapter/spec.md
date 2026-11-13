## MODIFIED Requirements

### Requirement: Console serves the channel adapter contract
The console SHALL be a conforming channel adapter and the declared receipt-only one. It SHALL read ops over the manager's stream `GET /channel/ops/stream?adapter=console`, acknowledge each with `POST /channel/ops/{id}/ack` on receipt, and submit UI-typed user messages via `POST /channel/inbound`, authenticating every call with its injected per-adapter derived token. It SHALL call no `/done`, since the manager consumes each op at the acknowledgement. It SHALL tolerate an op offered more than once by rendering each op id once.

#### Scenario: Ensure-topic binds a console thread
- **WHEN** the console receives an `ensure-topic` op for a conversation
- **THEN** it uses the thread id the manager assigned (`console-<uid>`, derived from the conversation's UID), so the same conversation maps to the same thread across console restarts

#### Scenario: Redelivered op renders once
- **WHEN** a `send` op is offered twice (a failed acknowledgement, a reconnect)
- **THEN** the transcript shows the message exactly once, deduplicated by op id

#### Scenario: UI message enters through the shared router
- **WHEN** a user types a message in a joined conversation's transcript view
- **THEN** the console posts it to `/channel/inbound` with the console thread id and it is queued/acked exactly as an inbound message from any other channel

#### Scenario: A reconnect is offered only unacknowledged ops
- **WHEN** the console's stream drops and it reconnects
- **THEN** it is pushed only the ops it never acknowledged, and none that it acknowledged
