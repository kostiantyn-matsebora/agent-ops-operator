## 1. Accept step — backend-developer

- [x] 1.1 Split `handleSignalInbound` in `platform/manager/internal/httpapi/publish.go` so the request does authentication, source scope, body shape, the unknown-source `404`, the "signal received" receipt and the attribute filters, then stops. Verify with a handler test that no `Signal` is written by the request.
- [x] 1.2 Enqueue one `ingest-signal` task per admitted signal on `in.signal`, keyed by source and `initiationId`, through the accept helpers in `accept.go`. Verify the task carries the signal body and the source name.
- [x] 1.3 Reply `202` with `{invocationId, taskIds}` and keep `queued` (= `len(taskIds)`) and `reason`. Verify an unwired source is accepted and a fully filtered batch replies `202` with empty `taskIds`, `queued: 0` and `reason: filtered`.

## 2. Consumer — backend-developer

- [x] 2.1 Register the `in.signal` consumer pool in `cmd/manager` beside the `conv` pool, sized by the existing consumer concurrency. Verify the pool starts and reports ready only with the bus up.
- [x] 2.2 Move `answerCommands` and `publishOne` into the `ingest-signal` handler, so the consumer answers an in-place chat command and creates the `Signal`. Verify a chat `/close` signal is answered once and stores nothing.
- [x] 2.3 Make the create idempotent under redelivery: a `signalId` that landed is deduplicated, and a signal without one takes its name from the task id. Verify a redelivered task creates one `Signal`.
- [x] 2.4 Declare no new retry values: use the inventory entry already declared for `in.signal` (5 attempts, 1s to 30s with jitter). Verify a handler that always fails goes `dead` and appears in `/state/dead-letters`.
- [x] 2.5 Add the `ingest-signal` verb to the command-bus conformance suite. Verify `go test -tags conformance ./test/conformance/` passes.

## 3. Unit tests — testing-specialist

- [ ] 3.1 Update the roughly 15 cases in `internal/httpapi/publish_test.go` and `signals_test.go` that assert `200` or a `Signal` on return: status-only cases expect `202` with `taskIds`, and cases that need a `Signal` run the consumer first.
- [ ] 3.2 Move the tests of `answerCommands` and `publishOne` to the consumer's package and add a redelivery case, an unwired-source case and a fully-filtered-batch case.
- [ ] 3.3 Run `go build ./... && go vet ./... && go test ./...` in `platform/manager`, with `KUBEBUILDER_ASSETS` set so the envtest suite runs. In every `signals/*` module, run `go test ./...` to confirm no adapter changed behaviour.

## 4. System tests — testing-specialist

- [ ] 4.1 Add a lane to `platform/manager/test/system/`: post a signal through the real endpoint, observe the task reach `succeeded` and the `Signal` appear, and repeat with a manager restart between accept and consume. Run it against THIS working copy's chart and images, not the default branch's.
- [ ] 4.2 Add a lane for a handler that really fails: backoff, then a dead-letter record, with the adapter's `202` unchanged.
- [ ] 4.3 Run the pack and record the verdict. A cluster-less remote session dispatches the cluster tier and also proves the same outcomes in the envtest and conformance tiers, never leaving a lane unexercised.

## 5. Documentation — backend-developer

### Reference docs

- [x] 5.1 `docs/contracts.md` and `docs/contracts/signal-adapter.md`: the `202` reply, `taskIds`, the new meaning of `queued`, and spec version `2.4.0` (re-check that `2.3.0` is still current — the contract page had moved to `2.3.1`, a wording-only patch. The six per-adapter pages were still at `2.3.0`, matching the design's assumption).
- [x] 5.1a Bump the claimed specification version to `2.4.0` in every shipped signal adapter that states one, and in the page's enumeration of them.
- [x] 5.1b Run the signal-adapter-contract conformance walk for every shipped signal adapter after the `2.4.0` bump, per "A specification change carries conformance", and record the verdict (`go test -tags conformance -run 'TestSignal' ./test/conformance/`: 9/9 pass across cron, alertmanager, k8s-events, ha, pulse, telegram).
- [x] 5.2 `docs/concepts.md`: the `Signal` is created by the consumer after acceptance.
- [x] 5.3 `docs/CHANGELOG.md`: the `200` to `202` move and the note that no adapter changes.
- [x] 5.4 `async-command-bus` is already archived, so this change folds the `ingest-signal` row and the `in.signal` queue entry into `async-operations` and `command-bus` directly, as new delta specs under this change's own `specs/`, rather than deferring to this change's own archive. Checked the published `async-operations` spec for the `Wired=False` refusal design D-7 says to drop: it never existed there (only in `async-command-bus`'s own unmerged design doc), so there is nothing to drop.

### Adopter site

- [x] 5.5 The signal guides under `docs/guides/` and the pages under `docs/integrations/` that describe the inbound reply or its status.
- [x] 5.6 `docs/installation.md` and `docs/configuration.md`, where they name the queues or the consumer concurrency: neither enumerates `in.signal`/`conv` or the concurrency knob individually, so neither needed a change.
- [x] 5.7 Re-run `python3 .github/scripts/docs-generate.py --check` and the publication and retired-vocabulary guards, and record the verdicts. All three pass: 70 generated files up to date, publication guard clean, retired-vocabulary guard clean over 144 files.
