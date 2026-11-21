## Context

See `proposal.md` for the why. These are the facts the decisions rest on.

| Fact | Where |
|---|---|
| The repo has 16 Go modules. 14 have no third-party dependency. Only `platform/manager` (73) and `runtimes/ollama` (8) have any | `go.mod` of each, from `.github/components.sh modules` |
| Each component builds from its own directory. No Dockerfile shares code from another directory, and no `go.mod` has a `replace` | the Dockerfiles and `go.mod` files |
| The Bus is `platform/manager/internal/commandbus`. A Go module cannot import another module's `internal/` | the language |
| The Bus's routing is a function from a queue name to a mode: regular, convoy or hand-over | `internal/commandbus/nats/wm_pubsub.go` |
| The convoy queue consumer pool depends on the `Bus` interface only | `internal/commandbus/consumer` |
| Watermill's `CommandBus.Send` returns only the publish error and has no query side | Watermill 1.5.3 `components/cqrs` |
| The manager serves about 46 routes in four auth domains and the coordinator routes | `internal/httpapi/server.go`, `coordinate.go` |
| A reasoned table lists what stays synchronous: `invoke`, `/work*`, the coordinator reads, adapter state and status, `/activity`, mark read | `async-operations` spec |

## Goals / Non-Goals

**Goals:**

- The Bus's asynchronous behaviour does not change.
- Sync or async is declared once, with the command, and the caller cannot override it.
- A reader can tell from a handler whether it changes state.
- The components share one library instead of one copy each.

**Non-Goals:**

- No change to the Bus's asynchronous behaviour, its task records or its backends.
- No event sourcing, and no outbox or inbox.
- No change to a route's wire shape unless a table row says so.
- No new transport.

## Decisions

### D1. The library's shape

One library, in a new `lib/` group, split by what each part needs.

| Part | Holds | Third-party dependencies |
|---|---|---|
| **Core** | the handler type, the registry, the routing, the command processor, the query processor, the `Bus` interface, the consumer pool, the `memory` backend, the conformance suite | none |
| **NATS backend** | the Watermill publisher and subscriber, the convoy queue controller and the records | Watermill and nats.go |

The two are separate Go modules. The core stays stdlib-only, so the 14 stdlib-only components can use it without gaining a dependency. Only the manager imports the NATS backend, as it is the only component that opens a bus.

Rejected: one module for both. Every component's `go.mod` would then list Watermill and nats.go, and `go mod download` would fetch them for 14 components that never use them.

### D2. How a component consumes the library

This is the first shared code in the repo, and each component builds from its own directory.

| Option | Verdict |
|---|---|
| **A. The library is a Go module with its own version, tagged `lib/<name>/vX.Y.Z`, and each component requires it** | **Chosen, subject to the spike** |
| B. Copy the library into each component by a generated step | Rejected. It is the copies we want to remove |
| C. One `go.mod` for the whole repo | Rejected. It ends the isolation every component has |

A local build in the repo uses a `go.work` file, which no Dockerfile reads. A release of the library means a tag and a bump in each consumer. That is a real cost, and the design accepts it.

Whether the Go module proxy resolves a tag with a sub-directory prefix for this repository is **to verify** in the first spike.

### D3. The Bus moves out of `internal/`, unchanged

The Bus and its pool, backends and conformance suite move into the library as they are. The first move changes no behaviour. The existing conformance suite and tests are the proof, and they pass before and after.

### D4. The synchronous scenario is a mode of the Bus

Routing already maps a message type to a mode. **Sync** joins regular, convoy and hand-over. A command type is registered with its handler, and its routing says how it is delivered.

| Mode | Delivered by | The caller gets |
|---|---|---|
| Sync | the handler, in the calling process | the handler's result or refusal |
| Regular, convoy | the queue and the consumer pool, on any replica | accepted |
| Hand-over | the queue, read by an adapter | accepted |

A sync command does not touch the bus transport. A bus outage does not stop it.

The caller cannot choose the mode. A route's kind is fixed by what the route is. `invoke` stays synchronous for the reasons already in the `async-operations` spec, and those requirements need no change.

### D5. The query processor is separate

`Execute` takes a typed query, runs its one registered handler and returns the data. It has its own registry and its own types. A query handler is given no command processor, so it cannot send a command.

It is not part of the Bus. A query is not delivered, retried or recorded.

### D6. The routes, by kind

| Kind | Routes | Registered as |
|---|---|---|
| Command, async | reopen, delete, reset-context, `POST /channel/inbound`, `POST /signal/inbound` | an async mode. The reply means accepted |
| Command, sync | `POST /work/done`, `POST /work/context`, `POST /coordinate/invoke`, `/coordinate/close`, `/coordinate/escalate` | sync |
| Command, sync, small | `PUT /channel/state/...`, `PUT /signal/state/...`, the two status routes, `POST /channel/read`, `POST /activity` | sync |
| Claim | `GET /work`, `GET /channel/ops`, its stream, ack and done | See D7 |
| Query | `/state/*` (nine routes), `GET /channel/channels`, vocabulary, `GET /channel/state/...`, `GET /signal/sources`, `GET /signal/state/...`, `GET /activity` and its stream, `GET /status`, `GET /pipelines/{name}/resolved`, `POST /coordinate/read`, `/agents`, `/tree`, `/open-roots`, `GET /healthz` | the query processor |

The table is a first draft, grouped. The first task writes the exact per-route table, with each route's response today, before any code changes.

### D7. Claims are not commands

`GET /work` and `GET /channel/ops` read and change state in one call, and they long-poll up to 30 seconds. They are the Bus's own consumer protocol, so they stay as they are, named as claims in the spec. The ack and done ends of `/channel/ops` are commands and follow D6.

### D8. Which components adopt it

The manager first. For each other Go component, the first task lists the handlers it has and whether any is a command or a query.

A component with no handler of its own has nothing to adopt, and the task records that. The Node runtimes cannot use a Go library and are out of scope.

### D9. Migration

| Order | Step | Proof |
|---|---|---|
| 1 | Create the library and move the Bus into it, unchanged | The conformance suite and every test pass before and after |
| 2 | Point the manager at the library | The manager's tests pass |
| 3 | Add the sync mode and the registry | New tests |
| 4 | Add the query processor | New tests |
| 5 | Move the manager's routes, one family at a time | Each family keeps a test of its current response |
| 6 | The other components, one at a time | Each component's own tests |

## Risks / Trade-offs

- **[A shared module is the first in the repo]** → D2 and a spike before any move.
- **[Every library change needs a tag and a bump in the consumers]** → the library changes rarely, and a `go.work` file covers local work.
- **[Moving the Bus breaks the manager's imports]** → step 1 changes no behaviour, and the full suite runs before and after.
- **[A route's response changes by accident]** → the first task records every response, and each family keeps a test of it.
- **[Adoption is forced where a component has no handlers]** → D8 lets a component record that it has nothing to adopt.

## Migration Plan

1. Land the per-route table and the spike results.
2. Move the Bus into the library with no behaviour change, then release it.
3. Add the sync mode and the query processor, then move the routes one family per release.
4. A rollback works at any step, because no wire contract changes.

## Open Questions

- Whether `GET /work` and `GET /channel/ops` stay as claims (D7).
- Whether the processors take middleware: recovery, timeout and logging.
- Whether a command publishes an event. It is not in this change unless asked for.
- The package names and the final call shape, given that Go has no generic methods.
