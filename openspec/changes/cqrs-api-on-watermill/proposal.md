## Why

The manager's HTTP layer mixes reading and writing inside one handler. `handleWorkDone` reads the record, judges a Coordinator turn, records the run, clears the inflight run, then moves the phase in a second write. `tryDispatch` reads, decides and writes in one function. A reader cannot tell what is a command and what is a query, and each endpoint carries its own accept-and-run-later code (`acceptAndEnqueue`).

Six verbs were converted by hand (`close`, `exit`, `reopen`, `delete`, `reset-context`, `escalate`). The rest stay as they were. The bus now runs on Watermill core (#532). Watermill 1.5.3 also ships the command and event buses (`components/cqrs`) and a reply with a result (`components/requestreply`). The project hand-builds what the library already provides.

## What Changes

- **Every route is a command or a query.** A route that changes state is a command. A route that returns state is a query. The design lists all routes and says which.
- **Commands go through Watermill's command bus.** The handler builds a typed command, sends it and answers. The work runs in a command handler.
- **A synchronous command uses a reply.** `invoke`, `/work/done` and the coordinator verbs keep their immediate answer. The caller waits for the handler's result through `requestreply`. The reasons in the current "stay synchronous" table keep holding.
- **Events record what happened.** A command handler publishes events through Watermill's event bus. The activity feed and the read surfaces consume them.
- **Queries read a read model.** The state store, its mirror and the `/state/*` surface already are one. The queries that read live objects move to read models too, or are named as exceptions.
- **The HTTP surface and its auth domains stay.** Adapters, runtimes and the console call the same paths. Where a response changes, the design says so route by route.

This is not event sourcing. The state store stays the write model.

**BREAKING:** any route that moves from an immediate result to "accepted" changes its contract. The design marks which routes can and which cannot. Routes that keep an immediate answer do not break.

## Open questions

The design settles each. None is guessed here.

- **Scope of "all".** The pull protocols (`GET /work`, `GET /channel/ops` and its ack and done) are claims with a side effect. They may be exempt.
- **One handler runtime or two.** Watermill's command processor and the convoy queue consumer pool both run handlers. Running both is a cost.
- **Task records.** The bus keeps task state, dead letters and statistics that the console reads. Watermill's command bus does not.
- **Convoy key and topic naming.** How a command's name and its key map onto a queue and a convoy queue.
- **Leader proxy.** Every non-leader replica proxies most routes to the leader today. A command bus lets any replica accept a command.

## Capabilities

### New Capabilities

- `api-cqrs`: every manager route is a command or a query, commands run in handlers behind a command bus, and queries read a read model.

### Modified Capabilities

- `async-operations`: the operations that stay synchronous to the caller run as commands with a reply.

## Impact

| Area | Change |
|---|---|
| `platform/manager/internal/httpapi/` | handlers become thin: build a command, send, answer. Queries read a read model |
| `platform/manager/internal/commandbus/` | a Watermill `cqrs` command bus and event bus over the existing publisher and subscriber |
| `platform/manager/internal/chat/`, `internal/controller/`, `internal/signalroute/` | command handlers move out of the HTTP package |
| `platform/manager/internal/activity/` | consumes events |
| `docs/` reference | `contracts.md`, `contracts/runtime.md`, `contracts/channel-adapter.md`, `contracts/signal-adapter.md`, `concepts.md`, `testing.md`, `CHANGELOG.md` |
| `.claude/rules/` | `invariants.md` (the converted-verb row and the dumb-proxy row), `structure.md` |
| Adopter site | `introduction.md`, `getting-started.md`, `installation.md` and `docs/guides/*` describe no route. The documentation task checks each and records that |

**Builds on #532, merged, and on `conversation-inputs-on-convoy-queues` (#537).** The `command-bus` spec on master still describes the log engine #532 replaced. That spec change is owed separately.
