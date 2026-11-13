## REMOVED Requirements

### Requirement: A channel op's claim is written only by the leader
**Reason**: A channel op is a message on the bus's per-channel queue, handed over by an acknowledgement. No claim is written on the Conversation and no replica has special standing.

**Migration**: Read the replacement requirement below. An adapter now acknowledges each operation it reads.

### Requirement: A claim expires if its holder goes silent
**Reason**: No lease is placed on a message and nothing is redelivered after a hand-over. A delivery TTL, started by the acknowledgement, replaces the claim lease, and a failure is final.

**Migration**: Read the replacement requirement below. The old `reclaimAfterSeconds` field is replaced by the `deliveryTtlSeconds` of the acknowledgement reply.

### Requirement: A non-leader proxies a poll it cannot safely answer
**Reason**: Channel ops live in the command bus's per-channel queues, which every replica reads.

- Every replica serves the read, the acknowledgement and the report.
- The `503` rejection has no cause left.

**Migration**: An adapter that retries on an empty poll needs no change. It
never sees the rejection again.

## MODIFIED Requirements

### Requirement: A non-leader proxies every route through one middleware

A non-leader manager replica SHALL reverse-proxy every route to the leader
except `/healthz` and the routes served from shared state, which any replica
answers itself. No handler proxies on its own account.

The routes served by any replica are:

- `GET /channel/ops`, `GET /channel/ops/stream`, `POST /channel/ops/{id}/ack` and
  `POST /channel/ops/{id}/done`
- every route under `/state/`, among them `GET /state/tasks`, `GET /state/tasks/{id}`,
  `GET /state/tasks/stream` and `GET /state/dead-letters`

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
- **WHEN** a read on `/channel/ops` or a read of `/state/tasks` reaches a
  non-leader replica
- **THEN** that replica answers it from the command bus and forwards nothing

## ADDED Requirements

### Requirement: A channel op is handed over by an acknowledgement, never leased

A channel op is a message on the bus's `out.<channel>` queue of its Channel.

An adapter SHALL take it in three steps, on any manager replica: a read that changes nothing, an acknowledgement that hands it over, and a report.

The manager SHALL place no claim and no lease on a message and SHALL NOT write the Conversation's `status.threads[].claim`. A value found there is ignored.

The read is `GET /channel/ops?adapter=&contract=&wait=`. The acknowledgement is
`POST /channel/ops/{id}/ack?adapter=`, and the report is
`POST /channel/ops/{id}/done` with an optional `threadId` or `error`.

```
GET  /channel/ops?adapter=telegram&contract=2&wait=25
200  {"id":"send:conv-1:tg-ops:run-7","channel":"tg-ops","kind":"send","threadId":"42","message":{...}}

POST /channel/ops/send:conv-1:tg-ops:run-7/ack?adapter=telegram
200  {"ok":true,"deliveryTtlSeconds":600,"consumed":false}

POST /channel/ops/send:conv-1:tg-ops:run-7/done
200  {"ok":true}
```

The acknowledgement SHALL answer `404` for a message that is gone and `409` for
one that is held but not being offered. It SHALL be idempotent, on any replica.

#### Scenario: A read changes nothing
- **WHEN** an adapter reads a message and stops before acknowledging it
- **THEN** the next read, on any replica, offers the same message, and no delivery had begun

#### Scenario: The leader has no special standing
- **WHEN** the replica holding the leader-election `Lease` serves a read or an acknowledgement
- **THEN** it behaves exactly as any other replica

#### Scenario: Any replica serves each step
- **WHEN** an adapter reads on one replica, acknowledges on a second and reports on a third
- **THEN** every step succeeds and no request is forwarded

#### Scenario: A repeated acknowledgement hands over once
- **WHEN** an adapter acknowledges the same message twice, or two replicas receive the acknowledgement at once
- **THEN** both answer the same hand-over and the manager hands the message over once

#### Scenario: A gone or unoffered message is named
- **WHEN** an adapter acknowledges a message that was already consumed, or one still behind an unfinished message of its conversation
- **THEN** the reply is `404` for the first and `409` for the second

#### Scenario: A claim is no longer written on the Conversation
- **WHEN** an op is read and acknowledged
- **THEN** the Conversation's `status.threads[].claim` is unchanged

### Requirement: An acknowledged op with no report fails at its delivery TTL

From its acknowledgement a message SHALL be the adapter's: the manager SHALL NOT send it again.

The acknowledgement starts the message's delivery TTL, declared per message kind (stable 10 minutes, ephemeral 2 minutes, topic operations 5 minutes, `delete-conversation` 5 minutes) and set by the manager's `CHANNEL_DELIVERY_TTL_STABLE`, `_EPHEMERAL`, `_TOPIC` and `_DELETE_CONVERSATION`.

An adapter SHALL report the message done, or failed with an error, after delivering it to its surface with its own retries.

A message with no report when its TTL passes SHALL be failed by the manager as a backstop, whether or not the adapter is alive.

A failure, reported or by timeout, SHALL be final: the manager SHALL NOT redeliver it, and what is still owed is re-derived from the conversation only after the kind's park cool-off.

The chart value `claimStalenessSeconds` SHALL be inert, kept only so an existing
install's values still render. A leader failover SHALL NOT wait for any delivery
TTL: the message lives in the bus, which a failover does not touch.

#### Scenario: A slow delivery is left alone
- **WHEN** an adapter acknowledged a message less than its delivery TTL ago and has not yet reported
- **THEN** the manager does not send it again

#### Scenario: No report fails the message
- **WHEN** the delivery TTL of an acknowledged message passes and no report arrived
- **THEN** the manager fails it, never redelivers it, and re-derives what is owed only after the cool-off

#### Scenario: A reported failure is final
- **WHEN** an adapter reports an acknowledged message failed
- **THEN** the manager records the failure and does not redeliver it

#### Scenario: A stale claimStalenessSeconds value has no effect
- **WHEN** an existing install's values still set `claimStalenessSeconds`
- **THEN** the value renders without error and no delivery changes

#### Scenario: A dead adapter's message does not block the rest
- **WHEN** an adapter acknowledges a message and goes away
- **THEN** that conversation's later messages wait until it fails at its TTL, and every other conversation's messages keep flowing

### Requirement: A thread entry is fanned out once, in causal order
Every thread entry (an input, a run reply, an escalation, a notice) SHALL be enqueued from one function to the `out.<channel>` queue of each channel bound to the conversation, except the surface it entered on.

That surface is the channel whose adapter the entry arrived through, and it is excepted unless its adapter declares `echoesOwnMessages: false`.

The order in which entries are enqueued SHALL be the order in which they happened. No other code path SHALL enqueue a thread entry. The reconciler MAY call the same function as a backstop.

#### Scenario: An answer precedes the hand-back its turn caused
- **WHEN** a run finishes without a move and the manager appends its corrective input
- **THEN** every bound channel's queue holds the run's reply before the corrective input

#### Scenario: Entries reach each bound channel once
- **WHEN** an entry is recorded for a conversation bound to two channels
- **THEN** each channel's queue holds it once, except the surface it entered on

### Requirement: A channel's queue is independent of every other channel
Each Channel SHALL have its own `out.<channel>` queue of the command bus, ordered by conversation as that capability specifies.

An adapter SHALL be offered the messages of every channel that names it, and a channel that is unavailable SHALL NOT delay any other channel, of the same adapter or of any other.

A message never consumed SHALL be retained 7 days by default and then removed, and one consumed SHALL be removed. The retention is the bus's own and bounds every channel queue alike.

#### Scenario: An unavailable channel does not hold back its sibling
- **WHEN** one of two Channels of the same adapter type stops acknowledging
- **THEN** the other continues to receive its entries in order

#### Scenario: An unavailable channel does not hold back another adapter's
- **WHEN** one channel stops acknowledging while a Channel of a different adapter has messages waiting
- **THEN** the other adapter receives its messages without waiting

### Requirement: The console acknowledges on receipt and the manager deletes the message
The adapters named by `CHANNEL_RECEIPT_ADAPTERS`, the console by default, are the declared exception to the delivery TTL. Such an adapter's acknowledgement SHALL be only its receipt of the message from the manager, not the push on to browsers.

It SHALL carry no delivery TTL and no report. The manager SHALL run the delivery side effects at the acknowledgement, delete the message from the queue, and answer `consumed: true`.

The mechanism SHALL be generic and configured, never coded: no manager or bus code SHALL name an adapter, and an adapter outside the set SHALL get the full contract.

A receipt adapter MAY read over `GET /channel/ops/stream`, which pushes each unacknowledged message once per connection as an `op` event and only reads. After a reconnect it SHALL offer exactly the messages never acknowledged.

The stream SHALL carry no cursor and no `Last-Event-ID`. An adapter outside the receipt set asking for the stream SHALL be answered `404` and polls instead.

The manager SHALL assign a receipt adapter's thread id when it enqueues `ensure-topic`, so no result travels back.

#### Scenario: A dropped connection loses nothing
- **WHEN** the console's stream drops while messages are produced and it reconnects
- **THEN** it is pushed every message it never acknowledged, in queue order, without a refresh

#### Scenario: A delivered message is not pushed again
- **WHEN** the console acknowledged a message
- **THEN** the manager deleted it and no later connection is pushed it

#### Scenario: A dead connection strands nothing
- **WHEN** a connection is gone without closing and a new one opens
- **THEN** the new connection is pushed every unacknowledged message, and no message waits on a lease

#### Scenario: Another adapter has no stream
- **WHEN** an adapter outside the receipt set requests `GET /channel/ops/stream`
- **THEN** it is answered `404` and reads `GET /channel/ops` instead

#### Scenario: A made-up adapter can be made receipt-only by configuration
- **WHEN** an adapter whose name no code mentions is added to `CHANNEL_RECEIPT_ADAPTERS`
- **THEN** its acknowledgement consumes the message with no manager or bus code change
