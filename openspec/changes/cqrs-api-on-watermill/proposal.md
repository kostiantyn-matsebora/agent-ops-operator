## Why

The manager already has the asynchronous half of a command processor. It is the `Bus` (#532): a command is submitted, delivered by a convoy queue, and handled with retry and task records. It runs on Watermill.

It has no synchronous half, and nothing for queries. So the HTTP layer mixes reading and writing in one handler. `handleWorkDone` reads the record, judges a Coordinator turn, records the run, clears the inflight run and moves the phase. `tryDispatch` reads, decides and writes. Each endpoint carries its own accept-and-run-later code (`acceptAndEnqueue`). A reader cannot tell a command from a query.

Brighter and Darker in .NET have this shape: a command processor with sync and async, and a query processor. Go has no equivalent. Watermill's command bus covers only the asynchronous part, and has no query side.

## What Changes

- **The Bus gets a synchronous scenario.** A command type is registered with a handler. Routing decides its mode, and "sync" joins the existing regular, convoy and hand-over modes. A sync command runs its handler in-process and returns the result or the refusal. The asynchronous scenario is **unchanged**.
- **A query processor.** It is separate. `Execute` runs a query's handler and returns data. It changes no state.
- **Both are one library**, shared by the components. It is a new Go module, not part of `platform/manager/internal`.
- **The components use it.** The manager's HTTP routes first. The other components where they have handlers.
- **Every manager route is a command, a query or a claim.** The HTTP handler parses, calls a processor and writes the response.

Nothing here changes the Bus's asynchronous behaviour, and nothing changes a route's wire contract unless the per-route table says so.

## Open questions

The design settles each. None is guessed here.

- **How components consume a shared module.** Today every component builds from its own directory and none shares code. Fourteen of the sixteen Go modules have no third-party dependency.
- **Moving the Bus out of `internal/`.** Other modules cannot import it where it is.
- **Which components adopt it.** "Every component" is taken per component, by what handlers it has.
- **The call shape in Go.** Go has no generic methods.
- **Middleware.** Whether the processors take recovery, timeout and logging.
- **Events.** Whether a command publishes an event. This is not part of this change unless asked for.
- **Claims.** `GET /work` and `GET /channel/ops` long-poll and read and change state in one call.

## Capabilities

### New Capabilities

- `cqrs-library`: a shared library with a command processor, a synchronous and an asynchronous path, and a separate query processor. It is consumed by the components as a module.
- `api-cqrs`: every manager route is a command, a query or a claim, and calls a processor.

### Modified Capabilities

- `command-bus`: the bus has a synchronous scenario, added beside its asynchronous ones.

## Impact

| Area | Change |
|---|---|
| New `lib/` module | the command processor, the query processor and the Bus, moved out of `platform/manager/internal/commandbus` |
| `platform/manager/` | imports the library. HTTP handlers become thin and the logic moves to command and query handlers |
| Other Go components | adopt the library where they have handlers. The first task lists them |
| `.github/components.sh` | `lib/` is a module that is not an image, like `test/` |
| Module releases | the library needs its own version and tag |
| `docs/` reference | `contracts.md`, `concepts.md`, `testing.md`, `CHANGELOG.md` |
| `.claude/rules/` | `structure.md` (the new group), `invariants.md` (the converted-verb row) |
| Adopter site | `introduction.md`, `getting-started.md`, `installation.md` and `docs/guides/*` describe no route. The documentation task checks each and records that |

**Builds on #532, merged.** The `command-bus` spec on master still describes the log engine #532 replaced. That spec change is owed separately.
