## 1. Inventory and spikes before any code

- [ ] 1.1 Write the per-route table: every route, its kind, its auth domain, its response today and its status codes. Verify: a test lists the routes the mux serves and fails if one is missing from the table.
- [ ] 1.2 Spike registering a typed handler with the convoy queue consumer pool, keeping the timeout, the retry and the task record. Verify: a throwaway test passes, and the answer is written in `design.md` D6.
- [ ] 1.3 Decide with the user whether `GET /work` and `GET /channel/ops` are exempt. Verify: `design.md` D8 and its open question are updated.
- [ ] 1.4 Settle the package name and the call shape, given that Go has no generic methods. Verify: `design.md` D2 shows the final shape.

## 2. The processors

- [ ] 2.1 Add the handler registry, the command processor with `Send` and `Post`, and the query processor with `Execute`. Verify: unit tests register a command and a query and run both.
- [ ] 2.2 Add the pipeline with recovery, timeout and logging. Verify: tests for a panic, a slow handler and the log line.
- [ ] 2.3 Make `Post` submit to the registered queue with the key function and the idempotency key, and run the handler in the pool. Verify: a test sends two commands of one key and sees order, and a repeated idempotency key creates one.
- [ ] 2.4 Make a query handler unable to reach the command processor. Verify: a compile-time or test check fails when a query handler tries.
- [ ] 2.5 Publish the existing activity kinds as events through Watermill's event bus after a write. Verify: a test sees the event after the record shows the change.

## 3. Move the route families, one at a time

- [ ] 3.1 Move one sync command end to end to prove the shape. Verify: its recorded response from 1.1 is unchanged.
- [ ] 3.2 Move the sync small writes: channel and signal state and status, mark read, `POST /activity`. Verify: the recorded responses are unchanged and they run with the bus stopped.
- [ ] 3.3 Move the already-async commands: reopen, delete, reset-context, `POST /channel/inbound`, `POST /signal/inbound`. Verify: the recorded responses are unchanged.
- [ ] 3.4 Move the coordinator and runtime commands: `invoke`, close, escalate, `/work/done`, `/work/context`. Verify: the recorded responses are unchanged, including a corrective input from `/work/done`.
- [ ] 3.5 Move the queries and name each one's read model or source of truth in the route table. Verify: the route table test fails on a query with neither.
- [ ] 3.6 Remove `acceptAndEnqueue` and the per-endpoint accept code. Verify: a grep finds neither.

## 4. Unit tests

- [ ] 4.1 Run `go build ./... && go vet ./... && go test ./...` in `platform/manager`. Verify: exit 0.
- [ ] 4.2 Run the envtest suite with `KUBEBUILDER_ASSETS`. Verify: exit 0.
- [ ] 4.3 Run `go test -race` over `internal/commandbus/...`, `internal/httpapi/...` and `internal/controller/...`. Verify: no race reported by this change.
- [ ] 4.4 Run the conformance suite for `memory` and `nats`. Verify: both pass.

## 5. System tests

- [ ] 5.1 A lane that stops the bus, asserts a synchronous command still returns its result and an asynchronous one refuses. Verify: the lane passes in `platform/manager/test/system/`.
- [ ] 5.2 A lane that runs every command family against a real cluster and compares each response with the recorded one. Verify: the lane passes.
- [ ] 5.3 Run the pack and the dispatched `system-smoke.yml`. Verify: both green, result recorded.

## 6. Documentation

- [ ] 6.1 Update the reference docs named in the proposal's Impact: `contracts.md`, `contracts/runtime.md`, `contracts/channel-adapter.md`, `contracts/signal-adapter.md`, `concepts.md`, `testing.md`. Verify: `docs-generate.py --check` passes and a grep finds no claim that an endpoint carries its own accept code.
- [ ] 6.2 Update `.claude/rules/invariants.md` and `structure.md`. Verify: a grep finds no stale claim.
- [ ] 6.3 Add the `docs/CHANGELOG.md` entry under `[Unreleased]`. Verify: it names each family moved.
- [ ] 6.4 Adopter site: check `introduction.md`, `getting-started.md`, `installation.md` and `docs/guides/*`, and record the pages that needed no change. Verify: the list exists.
- [ ] 6.5 Run `publication-guard.py --counts` and `retired-vocabulary-guard.py --counts`. Verify: both 0.
