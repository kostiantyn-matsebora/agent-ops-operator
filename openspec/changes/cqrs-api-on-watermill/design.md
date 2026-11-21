## Context

See `proposal.md` for the why. These are the facts the decisions rest on.

| Fact | Where |
|---|---|
| The manager serves about 46 routes in four auth domains and the coordinator routes | `internal/httpapi/server.go`, `coordinate.go` |
| Six verbs are already accept-and-process: `close`, `exit`, `reopen`, `delete`, `reset-context`, `escalate` | `async-operations` spec, `internal/httpapi/accept.go` |
| A reasoned table lists what stays synchronous: `invoke`, `/work*`, the coordinator reads, adapter state and status, `/activity`, mark read | `async-operations` spec |
| The bus keeps task records, dead letters and statistics that `/state/tasks` serves | `internal/commandbus/nats` |
| The convoy queue consumer pool runs a handler per message with a timeout and a task record | `internal/commandbus/consumer` |
| Watermill's `CommandBus.Send(ctx, cmd) error` marshals and publishes. Its error is the publish error. It never returns the handler's result or error | Watermill 1.5.3 `components/cqrs/command_bus.go` |
| Watermill's `cqrs` package has no query side. It has no handler pipeline either | same package |
| A blocking send exists only as an in-process pub/sub setting, `BlockPublishUntilSubscriberAck`, applied to every message on that publisher | `pubsub/gochannel` |
| Go has no generic methods, so a generic call is a package function | the language |

## Goals / Non-Goals

**Goals:**

- A reader can tell from a handler whether it changes state.
- Whether a command is sync or async is declared once, with the command.
- Timeout, recovery and logging live in one pipeline, not in each endpoint.

**Non-Goals:**

- No event sourcing and no outbox or inbox. The state store stays the write model.
- No change to a route's wire shape unless a table row says so.
- No change to the runtime, adapter or signal contracts' meaning.
- No new transport. The bus and Watermill stay as they are.

## Decisions

### D1. The routes, by kind

| Kind | Routes | Registered as |
|---|---|---|
| Command, async | reopen, delete, reset-context, `POST /channel/inbound`, `POST /signal/inbound` | `Post`. The reply means accepted |
| Command, sync | `POST /work/done`, `POST /work/context`, `POST /coordinate/invoke`, `/coordinate/close`, `/coordinate/escalate` | `Send`. The caller gets the result in the call |
| Command, sync, small | `PUT /channel/state/...`, `PUT /signal/state/...`, the two status routes, `POST /channel/read`, `POST /activity` | `Send` |
| Claim | `GET /work`, `GET /channel/ops`, its stream, ack and done | See D8 |
| Query | `/state/*` (nine routes), `GET /channel/channels`, vocabulary, `GET /channel/state/...`, `GET /signal/sources`, `GET /signal/state/...`, `GET /activity` and its stream, `GET /status`, `GET /pipelines/{name}/resolved`, `POST /coordinate/read`, `/agents`, `/tree`, `/open-roots`, `GET /healthz` | `Execute` |

The table is a first draft, grouped. The first task writes the exact per-route table, with each route's response today, before any code changes.

### D2. Two processors, in Brighter's and Darker's shape

| | Command processor | Query processor |
|---|---|---|
| Call | `Send` runs the handler in-process and returns its result or refusal. `Post` returns accepted | `Execute` returns data |
| Changes state | yes | no |
| Handler | one per command type | one per query type |
| Pipeline | recovery, timeout, logging | the same |

Both share one handler type, `func(ctx, Request) (Response, error)`, and one middleware shape, `func(next Handler) Handler`. The processor enforces the kind: a command is registered with the command processor, a query with the query processor.

A sketch of the shape, not the final API:

```go
processor.Register(cp, processor.Sync(handleWorkDone))
res, err := processor.Send(ctx, cp, req)   // returns the handler's result
processor.Register(cp, processor.Async(handleReopen, queue, key))
acc, err := processor.Post(ctx, cp, req)   // returns accepted
out, err := processor.Execute(ctx, qp, q)  // a query
```

### D3. Why not Watermill's command bus as the front door

| Need | Watermill `cqrs` | Our processor |
|---|---|---|
| Run a handler and return its result or error in the call | no. `Send` returns only the publish error | yes, `Send` |
| Choose sync or async per command | no. A blocking send is a setting of the whole publisher | yes, per registration |
| A query side | none | yes, `Execute` |
| A handler pipeline | none in the `cqrs` package | yes |
| Task records, dead letters, statistics | none | through `Post` onto our `Bus` |

Watermill keeps the jobs it is good at: the event bus, and the transport under our `Bus`.

Rejected: Go-MediatR. It has the pipeline idea and no transport, and it keeps package-level registries. Rejected: Event Horizon. It brings event sourcing.

### D4. A registration declares everything about a command

A command's registration holds its name, its mode, its handler and, for async, its queue, its key function and its timeout. The caller cannot override the mode. A route's kind is fixed by what the route is.

A command that must refuse at once, such as `invoke`, is registered sync. The reasons in the current "stay synchronous" table keep holding with no change to them.

### D5. The sync path touches no bus

`Send` runs in the process that received the request. It does not publish anything. A bus outage therefore does not stop a sync command. Today's behaviour is kept.

A sync handler that needs leader-only state still gets it the way it does now. The leader proxy is unchanged.

### D6. The async path rides the existing `Bus`

`Post` submits the command to the queue named in its registration, with the key function's value as the convoy key and the caller's idempotency key as the command id. The handler runs in the convoy queue consumer pool, on any replica.

The registration registers a pool handler that unmarshals the command and calls the typed handler through the pipeline. So there is one handler runtime, with the pool's timeout, retry and task records. Watermill's command processor is not used.

Whether the pool's handler API takes a typed function without change is **to verify** in the spike.

### D7. The pipeline is small

| Middleware | Does |
|---|---|
| Recovery | turns a panic into an error and a log line |
| Timeout | bounds a sync handler. An async handler already has the pool's |
| Logging | one line per command and query, with its name, key and outcome |

There is no retry in the pipeline. The bus owns retry for async commands. A sync command is not retried.

### D8. Claims are not commands

`GET /work` and `GET /channel/ops` read and change state in one call, and they long-poll up to 30 seconds. They are claims, which the bus serves natively.

They stay as they are, named as claims in the spec. The ack and done ends of `/channel/ops` are commands and follow D1. This is the part of "all" proposed as an exception.

### D9. Queries read a read model or a named source

`/state/*` already is one: the state store and its mirror. A query that reads live objects, such as `GET /status`, reads a read model that events keep current, or is named as reading a source of truth. A query that reads the Kubernetes API because that is the truth is still a query.

### D10. Events

A command handler publishes an event through Watermill's event bus after its write lands. The activity feed and the `/state` stream consume them. The events are the existing activity kinds first.

A lost event must not leave a read surface wrong for ever. The reconciler already re-derives owed work, and the design relies on that.

### D11. Migration

One route family at a time. Each family keeps a test of its current response.

| Order | Family | Why |
|---|---|---|
| 1 | The processors, the registry and the pipeline, with one command moved | Proves the shape |
| 2 | The sync small writes | Cheap, no bus involved |
| 3 | The already-async commands | They move onto `Post` unchanged |
| 4 | The coordinator and runtime commands | Real results, the reasons in the sync table |
| 5 | Queries | They change the least |

## Risks / Trade-offs

- **[Two small abstractions to own]** → they are a registry and a pipeline, not a framework. Each fits a page.
- **[A route's response changes by accident]** → the first task records every response, and each family keeps a test of it.
- **[Big scope]** → D11 orders it, and each family merges on its own.
- **[A pool handler cannot take a typed function]** → the spike in task 1.2 settles it before code.

## Migration Plan

1. Land the per-route table and the spike results.
2. Move one family per release, behind no flag. A family is done when the tests of its old response pass on the new path.
3. Remove `acceptAndEnqueue` and the per-endpoint accept code once the last verb has moved.
4. A rollback works at any point, because no wire contract changes.

## Open Questions

- Whether `GET /work` and `GET /channel/ops` are exempt (D8).
- Whether the queries that read live objects get read models or stay as named exceptions (D9).
- The package name and the final call shape (D2).
