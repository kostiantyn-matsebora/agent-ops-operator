## RENAMED Requirements

- FROM: `### Requirement: A channel op's claim is written only by the leader`
- TO: `### Requirement: A channel op's claim is a lease held by the command bus`

## REMOVED Requirements

### Requirement: A non-leader proxies a poll it cannot safely answer
**Reason**: A claim lives in the command bus's ledger.

- Every replica serves the poll.
- Any replica completes a claim another made.
- The `503` rejection has no cause left.

**Migration**: An adapter that retries on an empty poll needs no change. It
never sees the rejection again.

## MODIFIED Requirements

### Requirement: A channel op's claim is a lease held by the command bus

Claiming an outbound channel op out to a polling adapter SHALL be performed by
whichever manager replica serves the poll. The claim SHALL be a lease recorded
in the command bus's ledger, not on the Conversation and not in a replica's
memory.

The lease SHALL carry a claim token and a lease-until instant, written
conditionally on the task's revision. A claim made by one replica SHALL be
completable by another that presents the token.

The Conversation's `status.threads[].claim` SHALL NOT be written. A value
found there is ignored.

#### Scenario: The leader claims an op

- **WHEN** the replica holding the leader-election `Lease` serves a poll while
  a channel op is waiting
- **THEN** it claims the op as a lease in the bus's ledger exactly as any
  other replica would, with no special standing

#### Scenario: Any replica serves the poll

- **WHEN** an adapter's poll reaches a replica that is not the leader and a
  channel op is waiting
- **THEN** that replica claims it from the bus and returns it, without
  forwarding the request

#### Scenario: A claim is completed on another replica

- **WHEN** one replica claims an op and the adapter's done call reaches
  another replica
- **THEN** the completion succeeds because the claim token matches the ledger

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

The lease duration SHALL default to five minutes and SHALL stay configurable
through the chart value `claimStalenessSeconds`, rendered into the manager's
configuration, never a compiled-in constant, so an install with a slower
transport can raise it.

The bus returns an expired lease. No replica clears it, and leadership has no
part in it.

A leader failover SHALL NOT require waiting for the lease. The lease is in the
ledger, which a failover does not touch, and a claim whose holder is gone
expires on its own schedule while every other op keeps flowing.

#### Scenario: A legitimately slow claim is left alone

- **WHEN** an op was claimed less than the lease ago and the adapter has not
  yet completed it
- **THEN** the op is not redelivered

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

The proxy runs before the request reaches a handler, through one middleware
wrapping the whole mux.

- A handler reached anyway SHALL fall through to its own unchanged local
  behavior.
- A handler is reached anyway when the proxy failed or the replica is the
  leader.

Most routes this middleware covers read or write the Kubernetes API,
consistent on any replica, so the proxy changes no response a client could
already receive.

`/activity*` is the one exception: its state is a per-process ring buffer
with no cross-replica fan-out, not Kubernetes-API state. It is covered for the
same reason every other route is, not because it is K8s-API-consistent too.

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
