# durable-chat-ops-broker Specification

## Purpose
How a channel op is claimed exactly once across more than one manager
replica, how an abandoned claim is recovered, and what a replica that
cannot safely answer a poll does instead.

## Requirements

### Requirement: A channel op's claim is written only by the leader

Claiming an outbound channel op out to a polling adapter, and deciding
that a previously claimed op is abandoned, SHALL be performed only by the
manager replica currently holding the leader-election `Lease`.

The claim SHALL be recorded on the owning Conversation's `status`, keyed
per channel, as `status.threads[].claim` with the subfields `holder` (the
claiming replica's identity) and `claimedAt` (the time it was claimed).

#### Scenario: The leader claims an op

- **WHEN** the leader's reconciler finds a channel binding with no thread
  and no live claim
- **THEN** it writes a claim naming itself and the current time, then
  hands the op to the next matching poll

#### Scenario: A non-leader never writes a claim

- **WHEN** a manager replica that is not the current `Lease` holder would
  otherwise need to claim or revoke a claim
- **THEN** it performs neither write, regardless of what its own cached
  view of the Conversation shows

### Requirement: A claim expires if its holder goes silent

A claim SHALL carry a staleness bound. A claim older than that bound, on a
channel that still has no thread, SHALL be treated as abandoned.

The bound SHALL be configurable through the chart value
`claimStalenessSeconds`, rendered into the manager's configuration, never a
compiled-in constant, so an install with a slower transport can raise it.

The replica that clears an abandoned claim is always the current leader —
the same replica that is the only one ever allowed to write a claim in the
first place. Clearing one is therefore never a second, competing writer.

A leader failover SHALL NOT, on its own, require waiting for that bound.
The claim persists on the Conversation's `status`, so the new leader's
first reconcile pass finds it.

A claim whose holder is not the current leader is treated as abandoned at
once and cleared, without waiting for the staleness bound.

#### Scenario: A legitimately slow claim is left alone

- **WHEN** the current leader claimed an op less than the staleness bound
  ago, and the claiming adapter has not yet completed it
- **THEN** the leader does not reclaim or re-dispatch that op

#### Scenario: A stale claim is retried

- **WHEN** a claim is older than the staleness bound and its channel still
  has no thread
- **THEN** the current leader clears the claim and dispatches the op again

#### Scenario: A dead leader's claim is not waited out

- **WHEN** the leader that wrote a claim stops renewing its `Lease` and a
  new leader takes over
- **THEN** the new leader's reconcile pass finds a claim held by a
  former leader, clears it and dispatches the op immediately, without waiting for the
  staleness bound to elapse

### Requirement: A non-leader proxies a poll it cannot safely answer

A manager replica that is not the current leader, on receiving a poll for
a channel op it would need to claim, SHALL reverse-proxy the request to
whichever replica the leader-election `Lease` currently names as holder,
rather than answering from its own state.

The proxy SHALL forward the request and relay the response verbatim — it
claims nothing, derives nothing, and reads none of its own `OpQueue`,
which was never populated for ops this process did not claim.

A non-leader SHALL reject the request with `503`, distinguishably from
the `204` an empty poll already returns, only when the leader is unknown
or unreachable from here — never by answering from its own state.

#### Scenario: A non-leader proxies to the leader

- **WHEN** an adapter's poll for a channel op is served by a manager
  replica that is not the current leader, and the leader is reachable
- **THEN** that replica forwards the request to the leader and relays its
  response, whatever it is

#### Scenario: An unreachable leader falls back to rejection

- **WHEN** a non-leader cannot resolve or reach the current leader — the
  `Lease` is unread, or the leader's pod cannot be dialed
- **THEN** that replica rejects the request with `503`, distinguishably
  from the `204` an empty poll returns, and the adapter retries rather
  than treating the rejection as such

#### Scenario: A conforming adapter tolerates the rejection

- **WHEN** a channel adapter built against this contract receives the
  rejection
- **THEN** it retries exactly as it already does on an empty, timed-out
  poll, never surfacing the rejection as an adapter-side error

### Requirement: The manager's network policy admits its own replicas

The chart's `NetworkPolicy` for the manager SHALL admit ingress from pods
labelled `app.kubernetes.io/name: agentops-manager` in the release's own
namespace, alongside the callers it already admits, so a non-leader's
proxy dial to the leader is not dropped.

The admitted port is the manager's API port `8080`, a fixed literal with
no settable chart value (unlike `router.callPort`).

#### Scenario: A non-leader's proxy dial is admitted

- **WHEN** a non-leader replica dials the leader's pod on the API port
  with the network policy enforced
- **THEN** the connection is admitted and the proxy relays the leader's
  response

### Requirement: A non-leader proxies every other route through one middleware

A non-leader manager replica SHALL reverse-proxy every route except
`/healthz` to the leader, `/channel/ops` and `/activity*` included. No
handler proxies on its own account.

- It does so before the request reaches a handler, through one
  middleware wrapping the whole mux.
- A handler reached anyway SHALL fall through to its own unchanged local
  behavior, except `/channel/ops`.
- `/channel/ops` is excluded from that fallback. When the proxy fails it
  is rejected, as the requirement above states, and never answered from
  the non-leader's own state.
- A handler is reached anyway when the proxy failed or the replica is
  the leader.

Most routes this middleware covers read or write the Kubernetes API,
consistent on any replica, so the proxy changes no response a client
could already receive.

`/activity*` is the one exception: its state is a per-process ring buffer
with no cross-replica fan-out, not Kubernetes-API state. It is covered
for the same reason every other route is, not because it is
K8s-API-consistent too.

Proxying it reaches the leader's buffer, where reconciler-driven events
land — see `activity-telemetry`'s own requirement for its differing
fallback.

#### Scenario: A non-leader proxies an ordinary route

- **WHEN** any route other than `/healthz` is served by a non-leader
  replica and the leader is reachable
- **THEN** that replica forwards the request and relays the leader's
  response

#### Scenario: A failed proxy falls through to the local handler

- **WHEN** a non-leader cannot reach the leader for a route other than
  `/healthz`
- **THEN** the route's own handler runs locally, keeping its own
  unchanged behavior — for `/channel/ops` that behavior is the
  distinguishable rejection the requirement above defines, never
  answering from local state
