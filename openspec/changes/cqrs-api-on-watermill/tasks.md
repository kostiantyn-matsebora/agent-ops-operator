## 1. Inventory and spikes before any code

- [ ] 1.1 Write the per-route table: every route, its kind, its auth domain, its response today and its status codes. Verify: a test lists the routes the mux serves and fails if one is missing from the table.
- [ ] 1.2 Spike `cqrs.CommandProcessor` on the convoy queue consumer pool's subscriber, keeping task records, timeouts and retries. Verify: a throwaway test passes, or `design.md` D3 and D4 are reopened with the evidence.
- [ ] 1.3 Spike `requestreply` over the bus for `invoke` and `/work/done`, including a bus that cannot accept. Verify: the result and the refusal are both observed, and the answer is written in `design.md` D6.
- [ ] 1.4 Decide with the user whether `GET /work` and `GET /channel/ops` are exempt. Verify: `design.md` D5 and its open question are updated.

## 2. The command and event buses

- [ ] 2.1 Add a Watermill command bus and event bus over the existing publisher, with a naming rule from command to queue and key. Verify: a unit test sends a command and a handler receives it in key order.
- [ ] 2.2 Register command handlers into the pool so they keep the bus's timeout and task records. Verify: a handler that exceeds the timeout fails with a retryable code and a task record.
- [ ] 2.3 Add the reply path for commands that answer with a result. Verify: a test gets the result and a refusal in the call.
- [ ] 2.4 Publish the existing activity kinds as events after a write. Verify: a test sees the event after the record shows the change.

## 3. Move the route families, one at a time

- [ ] 3.1 Move the already-accepted commands: reopen, delete, reset-context, `POST /channel/inbound`, `POST /signal/inbound`. Verify: each route's recorded response from 1.1 is unchanged.
- [ ] 3.2 Move the small writes: channel and signal state and status, mark read, `POST /activity`. Verify: the recorded responses are unchanged and a bus outage refuses instead of dropping.
- [ ] 3.3 Move the coordinator and runtime commands: `invoke`, close, escalate, `/work/done`, `/work/context`. Verify: the recorded responses are unchanged, including a corrective input from `/work/done`.
- [ ] 3.4 Name each query's read model or source of truth in the route table. Verify: the route table test fails on a query with neither.
- [ ] 3.5 Remove `acceptAndEnqueue` and the per-endpoint accept code. Verify: a grep finds neither.
- [ ] 3.6 Decide whether the leader proxy can shrink, from the table in 1.1. Verify: `design.md` D9 records the answer.

## 4. Unit tests

- [ ] 4.1 Run `go build ./... && go vet ./... && go test ./...` in `platform/manager`. Verify: exit 0.
- [ ] 4.2 Run the envtest suite with `KUBEBUILDER_ASSETS`. Verify: exit 0.
- [ ] 4.3 Run `go test -race` over `internal/commandbus/...`, `internal/httpapi/...` and `internal/controller/...`. Verify: no race reported by this change.
- [ ] 4.4 Run the conformance suite for `memory` and `nats`. Verify: both pass.

## 5. System tests

- [ ] 5.1 A lane that stops the bus and asserts commands that answer with a result refuse and commands that answer accepted refuse too. Verify: the lane passes in `platform/manager/test/system/`.
- [ ] 5.2 A lane that runs every command family against a real cluster and compares each response with the recorded one. Verify: the lane passes.
- [ ] 5.3 Run the pack and the dispatched `system-smoke.yml`. Verify: both green, result recorded.

## 6. Documentation

- [ ] 6.1 Update the reference docs named in the proposal's Impact: `contracts.md`, `contracts/runtime.md`, `contracts/channel-adapter.md`, `contracts/signal-adapter.md`, `concepts.md`, `testing.md`. Verify: `docs-generate.py --check` passes and a grep finds no claim that an endpoint carries its own accept code.
- [ ] 6.2 Update `.claude/rules/invariants.md` and `structure.md`. Verify: a grep finds no stale claim.
- [ ] 6.3 Add the `docs/CHANGELOG.md` entry under `[Unreleased]`. Verify: it names each family moved.
- [ ] 6.4 Adopter site: check `introduction.md`, `getting-started.md`, `installation.md` and `docs/guides/*`, and record the pages that needed no change. Verify: the list exists.
- [ ] 6.5 Run `publication-guard.py --counts` and `retired-vocabulary-guard.py --counts`. Verify: both 0.
