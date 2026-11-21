## Why

The manager already has the asynchronous half of a command processor. It is the `Bus` (#532): a command is submitted, delivered by a convoy queue, and handled with retry and task records. It runs on Watermill.

It has no synchronous half, and nothing for queries. So the HTTP layer mixes reading and writing in one handler. `handleWorkDone` reads the record, judges a Coordinator turn, records the run, clears the inflight run and moves the phase. `tryDispatch` reads, decides and writes. Each endpoint carries its own accept-and-run-later code (`acceptAndEnqueue`). A reader cannot tell a command from a query.

The unit is the **action**. An action is a command or a query whatever triggers it: an HTTP route, a reconcile loop, a sweep, a timer, a queue message. The trigger only builds the request and calls a processor.

Brighter and Darker in .NET have this shape: a command processor with sync and async, and a query processor. Go has no equivalent. Watermill's command bus covers only the asynchronous part, and has no query side.

## What Changes

- **The Bus gets a synchronous scenario.** A command type is registered with a handler. Routing decides its mode, and "sync" joins the existing regular, convoy and hand-over modes. A sync command runs its handler in-process and returns the result or the refusal. The asynchronous scenario is **unchanged**.
- **A query processor.** It is separate. `Execute` runs a query's handler and returns data. It changes no state.
- **The library is the application layer, and it defines the message contracts.** Beside the two processors it defines what crosses them: a base command, a base query, and the response of each. Triggers, such as HTTP and reconcile loops, sit outside it. Handlers, which hold the domain logic, sit inside each component.
- **Both are one library**, shared by the components. It is a new Go module, not part of `platform/manager/internal`.
- **The components use it.** The manager's HTTP routes first. Then every other Go component, for its own actions.
- **Every action is a command or a query, whatever triggers it.** The HTTP handler, the reconcile loop, the sweep and the timer each build a request and call a processor. The logic lives in the handler.

Nothing here changes the Bus's asynchronous behaviour, and nothing changes a route's wire contract unless the action table (`docs/contracts/actions.md`) says so.

## Out of scope

Mapping the message contracts to a port, such as one uniform HTTP envelope, is a later change. The contracts here are Go types, and the existing routes keep their wire shapes.

## Open questions

The design settles each. None is guessed here.

- **How components consume a shared module.** Today every component builds from its own directory and none shares code. Fourteen of the sixteen Go modules have no third-party dependency.
- **Moving the Bus out of `internal/`.** Other modules cannot import it where it is.
- **The client side.** Whether the library includes an HTTP sender of the same typed commands and queries, so each adapter's hand-written `manager.go` goes. Six signal adapters, the channel adapter and the console each have one.
- **The call shape in Go.** Go has no generic methods.
- **Middleware.** Whether the processors take recovery, timeout and logging.
- **Events.** Whether a command publishes an event. This is not part of this change unless asked for.

## Capabilities

### New Capabilities

- `cqrs-library`: a shared library with a command processor, a synchronous and an asynchronous path, and a separate query processor. It is consumed by the components as a module.
- `action-processing`: every action is a command or a query whatever triggers it, and its trigger only calls a processor.

### Modified Capabilities

- `command-bus`: the bus has a synchronous scenario, added beside its asynchronous ones.

## Impact

| Area | Change |
|---|---|
| New `lib/` core module | stdlib-only: the command processor, the query processor, the message contracts, the Bus interface, the consumer pool and the memory backend |
| New `lib/` backend module | the NATS and Watermill Bus backend, moved out of `platform/manager/internal/commandbus`. Only the manager imports it |
| `platform/manager/` | imports the library. HTTP handlers become thin. The nine reconcilers and the sweeps become command handlers behind their triggers |
| Other Go components | every Go component routes its own actions through the library. The first task lists them |
| The message contracts | new types in the core, reusing the Bus's `Kind`, `Key`, `CommandID` and `ErrorCode` |
| `.github/components.sh` | `lib/` is a module that is not an image, like `test/` |
| Module releases | the library needs its own version and tag |
| `docs/` reference | `contracts.md`, `concepts.md`, `testing.md`, `CHANGELOG.md` |
| `.claude/rules/` | `structure.md` (the new group), `invariants.md` (the converted-verb row) |
| Adopter site | `introduction.md`, `getting-started.md`, `installation.md` and `docs/guides/*` describe no route. The documentation task checks each and records that |

**Builds on #532, merged.** The `command-bus` spec on master still describes the log engine #532 replaced. That spec change is owed separately.
