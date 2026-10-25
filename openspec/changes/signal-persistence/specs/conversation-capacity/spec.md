# conversation-capacity Delta

## MODIFIED Requirements

### Requirement: The pending backlog is bounded

The manager SHALL cap pending conversations at `MAX_QUEUED_CONVERSATIONS`
(chart `maxQueuedConversations`), default **50** — the FIFO-admitted cap on
`Pending` conversations is unchanged. When the backlog is full,
`/signal/inbound` SHALL NOT create a conversation.

What happens to the signal SHALL depend on its kind:

| Origin | Full-backlog behaviour |
|---|---|
| machine (`alert`, `job`, `task`) | waits as a `Pending` `Signal` CR, routed once the backlog drains — never dropped for capacity alone, though the per-source overflow bound still applies |
| person (`chat`) | declined immediately, told on the surface the message came from — a person is answered now, never left waiting silently |

#### Scenario: A machine signal waits, never dropped

- **WHEN** 50 conversations are `Pending` and another alert signal arrives
- **THEN** no conversation is created, and the signal waits in the `Pending`
  phase — capacity-wait is that phase, not a field of its own

#### Scenario: A waiting signal is routed when the backlog drains

- **WHEN** the pending-conversation count falls below the cap while
  `Signal` CRs wait
- **THEN** a waiting signal is routed through the ordinary routing path and
  its conversation is created

#### Scenario: Signal declined when the backlog is full

- **WHEN** the backlog is full and a chat signal arrives
- **THEN** no conversation is created, the signal does not wait, and its
  `Signal` records the capacity decline as a terminal drop

#### Scenario: Chat sender learns the request was declined

- **WHEN** the backlog is full and a person sends a message on a chat surface
- **THEN** a message on that surface explains the system is at capacity
