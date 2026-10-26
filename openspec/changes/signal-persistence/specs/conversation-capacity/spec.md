# conversation-capacity Delta

## MODIFIED Requirements

### Requirement: The pending backlog is bounded

The manager SHALL cap pending conversations at `MAX_QUEUED_CONVERSATIONS`
(chart `maxQueuedConversations`), default **50** — the FIFO-admitted cap on
`Pending` conversations is unchanged. When the backlog is full, the signal
CONSUMER SHALL NOT create a conversation. Publication at `/signal/inbound`
performs no capacity check and stores the signal whatever the backlog.

What happens to the signal SHALL depend on its kind:

| Origin | Full-backlog behaviour at consumption |
|---|---|
| machine (`alert`, `job`, `task`) | its claimant (the Pipeline or Coordinator the `Signal` routes to, a per-claimant entry in `status.claimants[]`) stays `Routing` and the consumer requeues it with a delay — the `Signal` waits and is routed once the backlog drains. Never dropped for capacity alone, though the per-source overflow bound and the retry deadline still apply |
| person (`chat`), unaddressed or addressed | `Dropped` with reason `capacity`, and the surface it came from is told through the channel-operation queue — a person is answered now, never left waiting silently |

#### Scenario: A machine signal waits, never dropped

- **WHEN** 50 conversations are `Pending` and a consumer processes an
  alert `Signal`
- **THEN** no conversation is created, its claimant stays `Routing`, and
  the consumer requeues it with a delay

#### Scenario: A waiting signal is routed when the backlog drains

- **WHEN** the pending-conversation count falls below the cap while
  `Signal`s wait
- **THEN** a later pass routes a waiting signal and its conversation
  is created

#### Scenario: Signal declined when the backlog is full

- **WHEN** the backlog is full and a consumer processes a chat `Signal`
- **THEN** no conversation is created, the signal does not wait, and the
  `Signal` records `Dropped` with reason `capacity`

#### Scenario: Chat sender learns the request was declined

- **WHEN** the backlog is full and a person sends a message on a chat
  surface
- **THEN** a message on that surface explains the system is at capacity,
  enqueued by the consumer, fire-and-forget — the `Signal` still records the
  reason if the notice is lost
