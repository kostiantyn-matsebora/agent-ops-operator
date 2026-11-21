## Context

See `proposal.md` for the why. These are the facts the decisions rest on.

| Fact | Where |
|---|---|
| The manager serves about 46 routes in four auth domains: runtime, adapter, signal and state, plus the coordinator routes | `internal/httpapi/server.go`, `coordinate.go` |
| Six verbs are already accept-and-process: `close`, `exit`, `reopen`, `delete`, `reset-context`, `escalate` | `async-operations` spec, `internal/httpapi/accept.go` |
| A reasoned table lists what stays synchronous: reads, `invoke`, the coordinator reads, `/work*`, the consumer ends of `/channel/ops`, adapter state and status, `/activity`, mark read | `async-operations` spec |
| The bus keeps task records, dead letters and statistics that `/state/tasks` serves | `internal/commandbus/nats` |
| The bus already sits on Watermill core: a publisher, a subscriber and a routing function | `internal/commandbus/nats/wm_pubsub.go` |
| Watermill 1.5.3 has `components/cqrs` (command bus, command processor, event bus, event processor, marshalers) and `components/requestreply` (a reply with a result) | the module cache |
| Every route except `/healthz`, `/state/*` and `/channel/ops*` proxies to the leader on a non-leader replica | `leaderProxyMiddleware` |

## Goals / Non-Goals

**Goals:**

- A reader can tell from a handler whether it changes state.
- No endpoint carries its own accept-and-run-later code.
- Every state change is an event something else can consume.

**Non-Goals:**

- No event sourcing. The state store stays the write model.
- No change to the wire shape of a route unless a table row says so.
- No change to the runtime, adapter or signal contracts' meaning.

## Decisions

### D1. The routes, by kind

| Kind | Routes | Proposed |
|---|---|---|
| Command, accepted | reopen, delete, reset-context, `POST /channel/inbound`, `POST /signal/inbound` | Already accepted. Move onto the command bus unchanged |
| Command, answered | `POST /work/done`, `POST /work/context`, `POST /coordinate/invoke`, `/coordinate/close`, `/coordinate/escalate` | A command with a reply. The caller still gets its answer in the call |
| Small write, read back | `PUT /channel/state/...`, `PUT /signal/state/...`, the two status routes, `POST /channel/read`, `POST /activity` | A command with a reply. No queue delay is added to a cheap write |
| Claim | `GET /work`, `GET /channel/ops`, its stream, ack and done | See D5. Possibly exempt |
| Query | `/state/*` (nine routes), `GET /channel/channels`, vocabulary, `GET /channel/state/...`, `GET /signal/sources`, `GET /signal/state/...`, `GET /activity` and its stream, `GET /status`, `GET /pipelines/{name}/resolved`, `POST /coordinate/read`, `/agents`, `/tree`, `/open-roots`, `GET /healthz` | A read model |

The table is a starting point. The first task writes the exact per-route table, with each route's response today, before any code changes.

### D2. CQRS here means two paths, not event sourcing

A command changes state and does not return state. A query returns state and changes none. The state store stays the write model. Events are notifications that something changed. They are not the record.

Rejected: event sourcing. It would replace the state store and the conversation record's history, and nothing asks for that.

### D3. Commands go through Watermill's command bus over our publisher

A command has a name. Its handler is a function of that command. The Watermill command bus publishes through the existing publisher, so a command rides the same convoy queue as today.

| Option | Verdict |
|---|---|
| **A. Watermill's command bus over the existing publisher and subscriber** | **Chosen, subject to the spike in D4** |
| B. Replace the bus's `Submit` and `Claim` with Watermill's | Rejected. It drops task records, dead letters and statistics, which `/state/tasks` serves |
| C. Keep per-endpoint accept code | Rejected. It is the problem |

The convoy key travels as message metadata, as it does now. How a command name maps onto a queue is **to verify** in the spike.

### D4. One handler runtime, not two

Watermill's command processor runs handlers on a message router. The convoy queue consumer pool runs handlers with a claim, a settle, a timeout and task records. Two runtimes would split the failure model.

The proposed answer is that the pool stays the runtime. A Watermill command handler is registered into the pool, so a handler keeps the bus's timeout, retry and records. The first task spikes whether `cqrs.CommandProcessor` can sit on the pool's subscriber and keep the task records. If it cannot, D3 option A is reopened.

### D5. Claims are not commands

`GET /work` and `GET /channel/ops` read and change state in one call, and they long-poll. A reply over a bus does not fit a 30 second wait. They are claims, which the bus already serves natively.

They stay as they are, named as claims in the spec. The ack and done ends of `/channel/ops` are commands and follow D1. This is the part of "all" that is proposed as an exception.

### D6. A synchronous command uses a reply

`invoke` must refuse at once and its reply carries the member and the budget. `/work/done` must answer with any corrective input. These are the reasons in the current table.

`requestreply` sends the command and waits for the handler's result. The caller sees the same synchronous call. The cost is that the call now depends on the bus. A bus that cannot accept refuses these calls where today they would succeed. That is a real availability change, and the ten-second chat budget already guards the same dependency elsewhere.

### D7. Queries read a read model

`/state/*` already is one: the state store and its mirror. The queries that read live objects, such as `GET /status`, `GET /pipelines/{name}/resolved` and the coordinator reads, move to a read model that a command's events keep current, or are named as exceptions that read a source of truth directly.

An exception is acceptable. A query that reads the Kubernetes API because that is the truth is still a query.

### D8. Events

A command handler publishes events through Watermill's event bus after it writes. The activity feed consumes them, and so does the `/state` stream. The events are the existing activity kinds first, so nothing new has to be invented.

An event is published after the write lands. A lost event must not leave a read model wrong for ever. The reconciler already re-derives owed work, and the design relies on that.

### D9. The leader proxy

A non-leader proxies most routes to the leader today, because a handler needed leader-only state. A command sent over the bus can be accepted by any replica. The commands then run in the pool, on any replica.

Whether `leaderProxyMiddleware` can shrink to the queries that need the leader is **to verify** after D1's table exists. It is not a goal of this change.

### D10. Migration

One route family at a time, each pinned by a test of its current response.

| Order | Family | Why first |
|---|---|---|
| 1 | The already-accepted commands | No behaviour change. Proves the bus wiring |
| 2 | Small writes | Cheap, and they exercise the reply |
| 3 | The coordinator and runtime commands | The reply carries real results |
| 4 | Queries | Last, because they change the least |

## Risks / Trade-offs

- **[Two handler runtimes]** → D4, and a spike before code.
- **[A synchronous call now depends on the bus]** → D6 states it. The system tests stop the bus and assert the refusal.
- **[Big scope]** → D10 orders it, and each family merges on its own.
- **[A route's response changes by accident]** → the first task records every response, and each family keeps a test of it.
- **[Task records are lost for commands that do not use `Submit`]** → D3 keeps them by riding the same publisher.

## Migration Plan

1. Land the per-route table and the spike results.
2. Move one family per release, behind no flag. A family is done when its tests of the old response pass on the new path.
3. Remove `acceptAndEnqueue` and the per-endpoint accept code once the last verb has moved.
4. A rollback to the previous release works at any point, because the wire contracts do not change.

## Open Questions

- Whether `GET /work` and `GET /channel/ops` are exempt (D5). This is the one that changes the meaning of "all".
- Whether the queries that read live objects get read models or stay as named exceptions (D7).
