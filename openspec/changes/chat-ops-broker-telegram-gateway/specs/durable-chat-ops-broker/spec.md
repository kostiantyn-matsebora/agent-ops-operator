## MODIFIED Requirements

### Requirement: A non-leader proxies a poll it cannot safely answer

A manager replica that is not the current leader, on receiving a poll for a channel op it would need to claim, SHALL reverse-proxy the request to whichever replica the leader-election `Lease` currently names as holder, rather than answering from its own state.

The proxy SHALL forward the request and relay the response verbatim — it claims nothing, derives nothing, and reads none of its own `OpQueue`, which was never populated for ops this process did not claim.

A non-leader SHALL reject the request, distinguishably from "no work is currently available," only when the leader is unknown or unreachable from here — never by answering from its own state.

#### Scenario: A non-leader proxies to the leader

- **WHEN** an adapter's poll for a channel op is served by a manager replica that is not the current leader, and the leader is reachable
- **THEN** that replica forwards the request to the leader and relays its response, whatever it is

#### Scenario: An unreachable leader falls back to rejection

- **WHEN** a non-leader cannot resolve or reach the current leader — the `Lease` is unread, or the leader's pod cannot be dialed
- **THEN** that replica rejects the request, distinguishably from "nothing to do," and the adapter retries rather than treating the rejection as such

#### Scenario: A conforming adapter tolerates the rejection

- **WHEN** a channel adapter built against this contract receives the rejection
- **THEN** it retries exactly as it already does on an empty, timed-out poll, never surfacing the rejection as an adapter-side error

## ADDED Requirements

### Requirement: The manager's network policy admits its own replicas

The chart's `NetworkPolicy` for the manager SHALL admit ingress from pods labelled `app.kubernetes.io/name: agentops-manager` in the release's own namespace, alongside the callers it already admits, so a non-leader's proxy dial to the leader is not dropped.

The admitted port is the manager's API port `8080`, a fixed literal with no settable chart value (unlike `router.callPort`).

#### Scenario: A non-leader's proxy dial is admitted

- **WHEN** a non-leader replica dials the leader's pod on the API port with the network policy enforced
- **THEN** the connection is admitted and the proxy relays the leader's response

### Requirement: A non-leader proxies every other route through one middleware

A non-leader manager replica SHALL reverse-proxy every route except `/healthz` to the leader, `/channel/ops` and `/activity*` included. No handler proxies on its own account.

- It does so before the request reaches a handler, through one middleware wrapping the whole mux.
- A handler reached anyway SHALL fall through to its own unchanged local behavior, except `/channel/ops`.
- `/channel/ops` is excluded from that fallback. When the proxy fails it is rejected, as the requirement above states, and never answered from the non-leader's own state.
- A handler is reached anyway when the proxy failed or the replica is the leader.

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

- **WHEN** any route other than `/healthz` is served by a non-leader replica and the leader is reachable
- **THEN** that replica forwards the request and relays the leader's response

#### Scenario: A failed proxy falls through to the local handler

- **WHEN** a non-leader cannot reach the leader for a route other than `/healthz`
- **THEN** the route's own handler runs locally, keeping its own unchanged
  behavior — for `/channel/ops` that behavior is the distinguishable
  rejection the requirement above defines, never answering from local state

## REMOVED Requirements

### Requirement: A non-leader rejects rather than proxies a poll it cannot safely answer
**Reason**: renamed to "A non-leader proxies a poll it cannot safely answer". `durable-chat-ops-broker`'s own design first rejected proxying as unneeded complexity. Measured live, 503-and-redial could take many redials to land on the leader by chance, so proxying was adopted.

**Migration**: none for a conforming adapter. It already retries a 503 immediately, and now mostly never sees one.

