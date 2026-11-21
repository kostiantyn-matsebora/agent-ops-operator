## 1. Measure and spike before any code

- [ ] 1.1 Measure the share of runs carrying more than one input, from `runs[].inputs` in the run history of the live install, and record the verdict in `design.md` D7. Verify: the number is written and D7 is either confirmed or reopened with the user.
- [ ] 1.2 Spike on a real NATS server: can the input family keep records with no age limit, and does the convoy queue controller accept a second per-name family. Verify: a throwaway test passes, and the answer is written in `design.md` D1 and D2.
- [ ] 1.3 Spike a runtime pod consuming its own queue. Verify: the evidence for refusing it is written in `design.md` D8.
- [ ] 1.4 Check that `Peek` can read a convoy queue's waiting messages. Verify: a test proves it, or `design.md` lists the console read as its own task.

## 2. The input family in the bus

- [ ] 2.1 Declare `in.conversation` in the inventory with no time to live, an unbounded retry that never drops, and a lease longer than the runtime's run limit. Verify: an inventory test asserts each of the three.
- [ ] 2.2 Keep the family's records in a bucket with no age limit. Verify: a test leaves an input past every other queue's retention and finds it waiting.
- [ ] 2.3 Add `Purge(queue)` to the `Bus` interface and both backends. Verify: the conformance suite scenario passes on `memory` and `nats`, and a purge records no success.
- [ ] 2.4 Make a failed delivery with a delay redeliver the same input without ending it. Verify: a conformance scenario fails an input past the other queues' attempt limit and finds it still waiting.
- [ ] 2.5 Declare in the `memory` backend that inputs do not survive a restart, and refuse it above one replica as today. Verify: a test asserts the refusal and the docs line.

## 3. The manager on the queue

- [ ] 3.1 Submit every input through the queue, with the input id as the command id, in `chat/router.go`, `chat/coordinate.go` and `signalroute/conversation.go`. Verify: a unit test per caller finds one waiting input and no entry on the record.
- [ ] 3.2 Make `GET /work` claim the conversation's own queue and drop the 2 second record poll. Verify: an envtest shows an idle pod receives a new input within one poll, and a second claim returns nothing while one is inflight.
- [ ] 3.3 Keep `Inflight` as the current run with the task uid and lease token, and settle in `POST /work/done`. Verify: a retried report settles once and records one run.
- [ ] 3.4 Fail with a delay on an unavailable context and when the pod is gone. Verify: an envtest holds an input through a context outage and runs it after.
- [ ] 3.5 Derive the phase and the capacity check from the queue's statistics and the current run. Verify: the existing capacity and phase tests pass unchanged.
- [ ] 3.6 Purge the queue on delete and release the payload blobs from the waiting inputs. Verify: an envtest deletes a conversation with two waiting inputs and finds neither the queue nor the blobs.
- [ ] 3.7 Serve queued inputs through `/state/conversations` from the queue. Verify: the state read test lists the waiting inputs.

## 4. The record and the migration

- [ ] 4.1 Read a legacy input from a record and move it to the queue by a leader sweep on a timer, idempotent by input id, then clear it. Verify: a test moves two legacy inputs once each, and an input appended after the first sweep is moved by the next.
- [ ] 4.2 Mark `Inputs` and `ProcessedInputIDs` deprecated in the Go record, the CRD and the `kube` mapping. Verify: CRD regeneration is clean and the `kube` mapping tests pass.
- [ ] 4.3 Remove the input-list reads from the 13 files that had them. Verify: a grep for the fields finds only the migration code and the deprecated field.

## 5. Unit tests

- [ ] 5.1 Run `go build ./... && go vet ./... && go test ./...` in `platform/manager` and fix every failure the change caused. Verify: exit 0 on the whole module.
- [ ] 5.2 Run the full envtest suite with `KUBEBUILDER_ASSETS`. Verify: exit 0.
- [ ] 5.3 Run `go test -race` over `internal/commandbus/...`, `internal/httpapi/...` and `internal/controller/...`. Verify: no data race reported by this change.
- [ ] 5.4 Run the console's `vitest` and `npm run test:e2e` if the work queue view changed. Verify: both pass, or a line says the view did not change.

## 6. System tests

- [ ] 6.1 Add a system lane that restarts the bus and the manager with inputs waiting and finds every input delivered once. Verify: the lane passes in `platform/manager/test/system/`.
- [ ] 6.2 Add a system lane that rolls the manager while a replica of the earlier release still appends to the record, and finds every input run once. Verify: the lane passes.
- [ ] 6.3 Add a system lane with a run longer than the ack wait of the other queues. Verify: the input is not delivered a second time.
- [ ] 6.4 Run the pack on a cluster and the dispatched `system-smoke.yml`. Verify: both green, and the result is recorded.

## 7. Documentation

- [ ] 7.1 Update the reference docs named in the proposal's Impact: `concepts.md`, `contracts.md`, `contracts/runtime.md`, `configuration.md`, `console.md`, `testing.md`, and regenerate `crds/conversation.md` and `crds/conversation-input.md` with `docs-generate.py`. Verify: `docs-generate.py --check` passes and a grep of those pages finds no claim that inputs live on the record.
- [ ] 7.2 Update `.claude/rules/invariants.md`, `terminology.md` and `wiring.md` where they describe the input queue. Verify: a grep finds no stale claim.
- [ ] 7.3 Add the `docs/CHANGELOG.md` entry under `[Unreleased]` with the migration and the "N-1 is not a rollback target after the first sweep" warning. Verify: the entry names both releases.
- [ ] 7.4 Adopter site: check `introduction.md`, `getting-started.md`, `installation.md` and `docs/guides/*`, add the upgrade note to `installation.md`, and record the pages that needed no change. Verify: the note exists and the others are listed.
- [ ] 7.5 Run `publication-guard.py --counts` and `retired-vocabulary-guard.py --counts`. Verify: both 0.
