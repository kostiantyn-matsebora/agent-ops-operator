## RENAMED Requirements

- FROM: `### Requirement: A channel op's claim is written only by the leader`
- TO: `### Requirement: A channel op's claim is a lease held by the command bus`

## REMOVED Requirements

### Requirement: A non-leader proxies a poll it cannot safely answer
**Reason**: A claim lives in the command bus's log.

- Every replica serves the poll.
- Any replica completes a claim another made.
- The `503` rejection has no cause left.

**Migration**: An adapter that retries on an empty poll needs no change. It
never sees the rejection again.

## MODIFIED Requirements

### Requirement: A channel op's claim is a lease held by the command bus

Claiming an outbound channel op out to a polling adapter SHALL be performed by
whichever manager replica serves the poll. The claim SHALL be a lease recorded
in the command bus's log, not on the Conversation and not in a replica's
memory.

The lease SHALL carry a claim token and a lease-until instant, appended
conditionally on the last entry the claimant saw. A claim made by one replica SHALL be
completable by another that presents the token.

The Conversation's `status.threads[].claim` SHALL NOT be written. A value
found there is ignored.

#### Scenario: The leader claims an op

- **WHEN** the replica holding the leader-election `Lease` serves a poll while
  a channel op is waiting
- **THEN** it claims the op as a lease in the bus's log exactly as any
  other replica would, with no special standing

#### Scenario: Any replica serves the poll

- **WHEN** an adapter's poll reaches a replica that is not the leader and a
  channel op is waiting
- **THEN** that replica claims it from the bus and returns it, without
  forwarding the request

#### Scenario: A claim is completed on another replica

- **WHEN** one replica claims an op and the adapter's done call reaches
  another replica
- **THEN** the completion succeeds because the claim token matches the log

#### Scenario: A non-leader never writes a claim
- **WHEN** a replica that is not the current `Lease` holder serves a poll
- **THEN** it claims from the bus and writes nothing to the Conversation's
  `status.threads[].claim`, which no replica writes

#### Scenario: A claim is no longer written on the Conversation

- **WHEN** an op is claimed
- **THEN** the Conversation's `status.threads[].claim` is unchanged

### Requirement: A claim expires if its holder goes silent

A claim SHALL carry a lease. A lease that passes with no completion SHALL
return the op to its queue for redelivery, counted as one attempt by the
command bus.

The lease duration SHALL default to five minutes, owned by the command bus's
log. The chart value `claimStalenessSeconds` no longer feeds it and is
inert, kept only so an existing install's values still render.

The bus returns an expired lease. No replica clears it, and leadership has no
part in it.

A leader failover SHALL NOT require waiting for the lease. The lease is in the
log, which a failover does not touch, and a claim whose holder is gone
expires on its own schedule while every other op keeps flowing.

#### Scenario: A legitimately slow claim is left alone

- **WHEN** an op was claimed less than the lease ago and the adapter has not
  yet completed it
- **THEN** the op is not redelivered

#### Scenario: A stale claimStalenessSeconds value has no effect

- **WHEN** an existing install's values still set `claimStalenessSeconds`
- **THEN** the value renders without error and an op's lease is still five minutes

#### Scenario: A stale claim is retried

- **WHEN** a lease has passed and the adapter never completed the op
- **THEN** the bus returns the op to its queue, counts one attempt, and the
  next matching poll claims it

#### Scenario: A dead leader's claim is not waited out

- **WHEN** the leader that served a poll stops renewing its `Lease` and a new
  leader takes over
- **THEN** unclaimed ops are claimed by any replica without waiting, and the
  former leader's claim expires on its own schedule without blocking them

### Requirement: A non-leader proxies every route through one middleware

A non-leader manager replica SHALL reverse-proxy every route to the leader
except `/healthz` and the routes served from shared state, which any replica
answers itself. No handler proxies on its own account.

The routes served by any replica are:

- `GET /channel/ops` and `POST /channel/ops/{id}/done`
- `GET /state/tasks`, `GET /state/tasks/{id}`, `GET /state/tasks/stream` and
  `GET /state/dead-letters`

`/activity*` is not among them. It is proxied, as the paragraph below states.

The proxy runs before the request reaches a handler, through one middleware
wrapping the whole mux.

- A handler reached anyway SHALL fall through to its own unchanged local
  behavior.
- A handler is reached anyway when the proxy failed or the replica is the
  leader.

Most routes this middleware covers read or write the Kubernetes API,
consistent on any replica, so the proxy changes no response a client could
already receive.

`/activity*` is the one proxied route whose state is not Kubernetes-API state.
It is a per-process ring buffer with no cross-replica fan-out. It is covered
for the same reason every other route is.

Proxying it reaches the leader's buffer, where reconciler-driven events
land — see `activity-telemetry`'s own requirement for its differing
fallback.

#### Scenario: A non-leader proxies an ordinary route

- **WHEN** any route other than `/healthz` and the routes served from shared
  state is served by a non-leader replica and the leader is reachable
- **THEN** that replica forwards the request and relays the leader's
  response

#### Scenario: A failed proxy falls through to the local handler

- **WHEN** a non-leader cannot reach the leader for a route other than
  `/healthz`
- **THEN** the route's own handler runs locally, keeping its own unchanged
  behavior

#### Scenario: A shared-state route is never proxied

- **WHEN** a poll on `/channel/ops` or a read of `/state/tasks` reaches a
  non-leader replica
- **THEN** that replica answers it from the command bus and forwards nothing

## ADDED Requirements

### Requirement: A thread entry is fanned out once, in causal order
Every thread entry (an input, a run reply, an escalation, a notice) SHALL be enqueued from one function to the queue of each channel bound to the conversation, except the surface that already displayed it.

The order in which entries are enqueued SHALL be the order in which they happened. No other code path SHALL enqueue a thread entry.

#### Scenario: An answer precedes the hand-back its turn caused
- **WHEN** a run finishes without a move and the manager appends its corrective input
- **THEN** every bound channel's queue holds the run's reply before the corrective input

#### Scenario: Entries reach each bound channel once
- **WHEN** an entry is recorded for a conversation bound to two channels
- **THEN** each channel's queue holds it once, except the surface that displayed it

### Requirement: A channel's queue is independent of other channels
Each channel SHALL have its own ordered queue. An adapter SHALL receive the entries of every channel that names it, and a channel that is unavailable SHALL NOT delay any other.

#### Scenario: An unavailable channel does not hold back its sibling
- **WHEN** one of two Channels of the same adapter type stops acknowledging
- **THEN** the other continues to receive its entries in order

### Requirement: The console is a plain SSE consumer of its log and resumes from a cursor
The manager SHALL push the console channel's log entries over the console's single SSE stream, in log order, each carrying its command id as the event id. The console SHALL need no acknowledgement. A reconnect that names the last event received SHALL replay every later entry, and an unknown id SHALL replay from the start of the retained log. A connection SHALL claim, lease and delete nothing, so a dead or second connection strands no entry.

The console channel's log SHALL be retained for its declared window (30 seconds by default) and trimmed by age and count, not by delivery. The manager SHALL assign the console thread's id itself. The console SHALL render an entry once per command id.

#### Scenario: A dropped connection loses nothing
- **WHEN** the stream drops while entries are produced and the console reconnects with the last event it received
- **THEN** it receives every later entry in order, without a refresh and without an acknowledgement

#### Scenario: A dead connection strands nothing
- **WHEN** a connection is gone without closing and a new one opens
- **THEN** the new connection receives every entry after its cursor, and no entry waits on a lease

#### Scenario: A replay does not duplicate a rendered entry
- **WHEN** the console reconnects with a cursor older than entries it already rendered
- **THEN** each entry is rendered once

### Requirement: An adapter takes a message in three steps and owns its delivery
A channel adapter other than the console SHALL read a message without changing it, acknowledge it to hand it over, and later report it done or failed. A message that has been read and not acknowledged SHALL be offered again. A message acknowledged SHALL NOT be sent by the manager again. The delivery TTL SHALL start at the acknowledgement, and a message with no report when it passes SHALL be failed, by the adapter and, if the adapter is gone, by the manager. A failed message SHALL be final.

#### Scenario: A read message is not lost
- **WHEN** an adapter reads a message and stops before acknowledging it
- **THEN** the message is offered again and no delivery had begun

#### Scenario: An acknowledged message is never sent again
- **WHEN** an adapter acknowledged a message and is slow to report
- **THEN** the manager does not send it again, however long it takes within the TTL

#### Scenario: A message with no report fails at its TTL
- **WHEN** the TTL of an acknowledged message passes with no report
- **THEN** the message is failed and is not redelivered

