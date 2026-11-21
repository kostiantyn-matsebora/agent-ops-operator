## 1. Inventory and spikes before any code

- [ ] 1.1 Write the action table as `docs/contracts/actions.md`: every manager route, reconciler, sweep and timer, its kind, its trigger, its auth domain and its response today. Verify: a test lists the routes the mux serves and the reconcilers registered, and fails if one is missing from the table.
- [ ] 1.2 Spike a shared Go module in this repository: a tag with a `lib/<name>/` prefix, required from a component whose image builds from its own directory, with a `go.work` for local work. Verify: a throwaway Dockerfile build of one component resolves the module, and the answer is written in `design.md` D2.
- [ ] 1.3 List every other Go component's actions and the commands and queries it sends to the manager. Verify: the list is written in `design.md` D8, and each component is named in task 6.1.
- [ ] 1.4 Decide with the user whether the library includes the client side, and whether the processors take middleware. Verify: `design.md` D8 and the open questions are updated.
- [ ] 1.5 Settle the package names and the call shape, given that Go has no generic methods. Verify: `design.md` shows the final shape.

## 2. The library, with the Bus moved in unchanged

- [ ] 2.1 Create the library as two modules, the stdlib-only core and the NATS backend, and add `lib/` to `.github/components.sh` as modules that are not images. Verify: `components.sh modules` lists them, `components.sh images` does not, and the core's dependency list is empty.
- [ ] 2.2 Move the `Bus` interface, the consumer pool, the `memory` backend and the conformance suite into the core, and the Watermill publisher, subscriber, convoy queue controller and records into the NATS backend. Verify: no code changes beyond imports, and every test in both modules passes.
- [ ] 2.3 Point the manager at the library. Verify: the manager builds and its full suite passes, including envtest.
- [ ] 2.4 Run the conformance suite on `memory` and on `nats`, and the three-node cluster test, from the library. Verify: the results match the results before the move.
- [ ] 2.5 Define the base command, query and the two responses in the core, with `Kind`, `Key`, `CommandID` and `ErrorCode` reused as they are. Verify: unit tests build each contract, a response with both a result and an error is refused, a page request carries a limit and an offset, and an error code outside the closed set of seven is refused.
- [ ] 2.6 Release the library with a tag and make the manager require that version. Verify: an image build of the manager resolves it with no `go.work`.

## 3. The synchronous scenario

- [ ] 3.1 Add the sync mode to the routing, and a registry that holds a command type's handler and mode. Verify: a unit test registers one command per mode and each is delivered its own way.
- [ ] 3.2 Add the sync send: run the handler in the calling process and return its result or refusal. Verify: tests for the result, the refusal, a run with the bus transport stopped, and a command sent twice with the same id, whose handler runs twice.
- [ ] 3.3 Make the mode part of the registration with no override by the caller. Verify: a test shows there is no call that changes the mode.
- [ ] 3.4 Prove the asynchronous modes are unchanged. Verify: the existing convoy and hand-over tests and the conformance suite pass without edits.

## 4. The query processor

- [ ] 4.1 Add the query registry and `Execute`, in the core and apart from the Bus. Verify: a unit test registers a query and gets its data.
- [ ] 4.2 Make a query handler unable to reach the command processor. Verify: a compile-time or test check fails when a query handler tries.
- [ ] 4.3 Verify a query leaves no task. Verify: a test executes a query and finds nothing on the bus.

## 5. The manager's routes, one family at a time

- [ ] 5.1 Move one sync command end to end to prove the shape. Verify: its recorded response from 1.1 is unchanged.
- [ ] 5.2 Move the sync small writes: channel and signal state and status, mark read, `POST /activity`. Verify: the recorded responses are unchanged and they run with the bus stopped.
- [ ] 5.3 Move the already-async commands: reopen, delete, reset-context, `POST /channel/inbound`, `POST /signal/inbound`. Verify: the recorded responses are unchanged.
- [ ] 5.4 Move the coordinator and runtime commands: `invoke`, close, escalate, `/work/done`, `/work/context`. Verify: the recorded responses are unchanged, including a corrective input from `/work/done`.
- [ ] 5.5 Move the queries and name each one's read model or source of truth in the route table. Verify: the route table test fails on a query with neither.
- [ ] 5.6 Remove `acceptAndEnqueue` and the per-endpoint accept code. Verify: a grep finds neither.
- [ ] 5.7 Move `GET /work` and the ack and done ends of `/channel/ops` to commands, keeping the long poll in the HTTP trigger. Verify: the recorded responses are unchanged and a poll still waits up to 30 seconds.
- [ ] 5.8 Move the sweeps and timers to commands behind their triggers. Verify: each sweep's tests pass unchanged.
- [ ] 5.9 Move the nine reconcilers to commands behind their `Reconcile`, one kind at a time, with the requeue mapped from the result and leader election still gating the trigger. Verify: each reconciler's envtest passes unchanged and a non-leader sends no command.

## 6. The other components

- [ ] 6.1 For each Go component, route its own actions through the library, and, if 1.4 says so, replace its hand-written `manager.go` with the library's sender. Verify: each component's own tests pass and its image still builds from its own directory.
- [ ] 6.2 Add a check that fails when a Go module listed by `.github/components.sh modules` does not use the library. Verify: the check passes on the repo and fails on a module that drops the import.

- [ ] 6.3 Add a check that fails when a component imports another component's `internal` package. Verify: it passes on the repo and fails on a test import of one.

## 7. Unit tests

- [ ] 7.1 Run `go build ./... && go vet ./... && go test ./...` in every module touched, the library's two and every adopting component. Verify: exit 0 in each.
- [ ] 7.2 Run the manager's envtest suite with `KUBEBUILDER_ASSETS`. Verify: exit 0.
- [ ] 7.3 Run `go test -race` over the library, `internal/httpapi/...` and `internal/controller/...`. Verify: no race reported by this change.
- [ ] 7.4 Check that a stdlib-only component still lists no third-party dependency. Verify: its `go.mod` has no `require` of a third-party module.

## 8. System tests

- [ ] 8.1 A lane that stops the bus, asserts a sync command still returns its result and an async one refuses. Verify: the lane passes in `platform/manager/test/system/`.
- [ ] 8.2 A lane that runs every moved command family against a real cluster and compares each response with the recorded one. Verify: the lane passes.
- [ ] 8.3 Build every adopting component's image from its own directory. Verify: every image builds, and `.github/components.sh images` is unchanged by `lib/`.
- [ ] 8.4 Run the pack and the dispatched `system-smoke.yml`. Verify: both green, result recorded.

## 9. Documentation

- [ ] 9.1 Update the reference docs named in the proposal's Impact: `contracts.md`, `concepts.md`, `testing.md`. Verify: `docs-generate.py --check` passes and a grep finds no claim that an endpoint carries its own accept code.
- [ ] 9.2 Update `.claude/rules/structure.md` for the `lib/` group and the module rule, and `invariants.md` for the converted-verb row. Verify: a grep finds no stale claim.
- [ ] 9.3 Add the `docs/CHANGELOG.md` entry under `[Unreleased]`. Verify: it names the library, the sync mode and each family moved.
- [ ] 9.4 Adopter site: check `introduction.md`, `getting-started.md`, `installation.md` and `docs/guides/*`, and record the pages that needed no change. Verify: the list exists.
- [ ] 9.5 Run `publication-guard.py --counts` and `retired-vocabulary-guard.py --counts`. Verify: both 0.
