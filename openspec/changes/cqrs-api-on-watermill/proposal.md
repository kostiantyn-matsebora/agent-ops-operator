## Why

The manager's HTTP layer mixes reading and writing inside one handler. `handleWorkDone` reads the record, judges a Coordinator turn, records the run, clears the inflight run, then moves the phase in a second write. `tryDispatch` reads, decides and writes in one function. A reader cannot tell what is a command and what is a query. Each endpoint also carries its own accept-and-run-later code (`acceptAndEnqueue`) and its own timeout and logging.

Six verbs were converted by hand. The rest were not. Nothing gives a handler a common place for the concerns every route needs.

.NET has a known shape for this: **Brighter**, a command processor, and **Darker**, a query processor. Go has no equivalent. Watermill covers the asynchronous command and event plumbing. It has no query side, and its command bus cannot run a handler in the caller's call. See the design.

## What Changes

- **A command processor.** `Send` runs a command's handler in-process and returns its result or its refusal. `Post` puts the command on the existing `Bus` and returns "accepted". Which one a command uses is part of its registration, not the caller's choice.
- **A query processor.** `Execute` runs a query's handler and returns data. It changes no state.
- **One handler registry and one pipeline.** A handler is a typed function. Middleware wraps it: recovery, timeout, logging.
- **Every route calls a processor.** The HTTP handler parses the request, calls `Send`, `Post` or `Execute`, and writes the response. Handlers move out of the HTTP package.
- **Events record what happened.** A command handler publishes an event after its write, through Watermill's event bus.
- **The HTTP surface and its auth domains stay.** Where a response changes, the design says so.

This is not event sourcing, and it is not Brighter's outbox or inbox. The state store stays the write model.

**Not breaking.** A sync command keeps its immediate answer. An async command keeps its "accepted". No route changes its contract unless the per-route table says so.

## Open questions

The design settles each. None is guessed here.

- **Scope of "all".** `GET /work` and `GET /channel/ops` are claims with a side effect. They may be exempt.
- **The shape of the API in Go.** Go has no generic methods, so the calls are package functions over a processor.
- **Where handlers live.** Out of `internal/httpapi`, into the packages that own the domain.
- **Async handlers and the consumer pool.** An async command's handler must run in the pool and keep its task record.
- **Queries that read live objects.** Read models, or named exceptions.

## Capabilities

### New Capabilities

- `api-cqrs`: every manager route is a command, a query or a claim. A command runs in a handler through the command processor, a query through the query processor, and both through one pipeline.

### Modified Capabilities

None. Sync stays sync, and accepted stays accepted, so the `async-operations` requirements still hold as written.

## Impact

| Area | Change |
|---|---|
| `platform/manager/internal/` | a new package for the two processors, the registry and the pipeline |
| `platform/manager/internal/httpapi/` | handlers become thin. The logic moves to command and query handlers |
| `platform/manager/internal/chat/`, `internal/controller/`, `internal/signalroute/` | command handlers live with their domain |
| `platform/manager/internal/commandbus/` | `Post` submits through it. Async handlers register with the consumer pool |
| `platform/manager/internal/activity/` | consumes events |
| `docs/` reference | `contracts.md`, `concepts.md`, `testing.md`, `CHANGELOG.md` |
| `.claude/rules/` | `invariants.md` (the converted-verb row), `structure.md` |
| Adopter site | `introduction.md`, `getting-started.md`, `installation.md` and `docs/guides/*` describe no route. The documentation task checks each and records that |

**Builds on #532, merged.** The `command-bus` spec on master still describes the log engine #532 replaced. That spec change is owed separately.
