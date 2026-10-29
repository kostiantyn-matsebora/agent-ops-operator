## 1. The contract — backend-developer

- [ ] 1.1 Create `platform/manager/internal/state/` with the domain records (conversation control record, run, input payload, signal) and the `Store` interface per design D-2: `Get`, `Create`, `Update` with CAS, `Delete`, `List` by the declared index set, `Watch` with a resume point, `AppendRun`, `ListRuns`, `PutBlob`/`GetBlob`/`DeleteBlob`, `expiresAt` on a record. Verify `go vet ./...` passes and the package has no import of `k8s.io/` or `sigs.k8s.io/`.
- [ ] 1.2 Define the conflict and not-found error values and the opaque `Revision` type. Verify a unit test distinguishes conflict from any other error with `errors.Is`.
- [ ] 1.3 Add the static check: a test in `internal/state/` that walks every manager package and fails on a Kubernetes client call whose argument is a Conversation, ConversationInput or Signal outside `internal/state/kube`. Verify it fails on the current tree (before section 4) and names the package.

## 2. The `kube` backend — backend-developer

- [ ] 2.1 Implement `internal/state/kube` mapping records to the existing `v1alpha1` types, byte-for-byte: `Get` via the apiserver, `Update` via the status subresource with `resourceVersion` as the revision, `Create`, `Delete`. Verify an envtest round-trips a conversation and the stored object's spec and status equal a fixture written by today's code path.
- [ ] 2.2 Implement `List` over the existing field indexes (`index.go`), one index per declared lookup. Verify envtest finds a conversation by `initiationId`, by `pipelineRef`+phase and by phase.
- [ ] 2.3 Implement `Watch` over the informer, delivering `(key, revision, record)` and resuming from a kept `resourceVersion`. Verify envtest: events for a create, two updates and a delete arrive in order, and a consumer resuming from the first update's revision sees the second update and the delete.
- [ ] 2.4 Implement `AppendRun` as a CAS update of `status.runs[]` and `ListRuns` in order (design D-5). Verify envtest appends N runs and reads them back in order.
- [ ] 2.5 Implement blobs as `ConversationInput` objects. Verify envtest puts, gets and deletes a payload and a deleted key reads absent.
- [ ] 2.6 Implement `expiresAt` as a stored field with the backend's own sweep. Verify envtest: a record past its expiry is omitted by `Get` and `List`, and a delete event follows on `Watch`.

## 3. The conformance suite and bench — testing-specialist

- [ ] 3.1 Create `internal/state/conformance` with `Run(t, newStore)`: one scenario per requirement of `specs/state-store/spec.md` — CAS race, stale write refused, every declared index, feed order, feed resume with no gap, stale snapshot caught by CAS, expiry invisible then deleted, runs in order, blob lifecycle, scope isolation, two-write tolerance. Verify every scenario names the spec requirement it covers in its test name.
- [ ] 3.2 Add `Bench(b, newStore)` reporting bytes written per run, allocations per write and resident records after a listing. Verify `go test -bench . -benchmem ./internal/state/...` prints all three for the `kube` backend.
- [ ] 3.3 Wire the `kube` backend into the suite under envtest. Verify `go test ./internal/state/...` runs every scenario green with `KUBEBUILDER_ASSETS` set.

## 4. The manager on the store — backend-developer

- [ ] 4.1 Move the Conversation reconciler to `Store`: reads from the fed snapshot, writes through `Update` with CAS, `pruneProcessed` through blobs. Replace `For(&Conversation{})` with `WatchesRawSource(source.Channel(...))` fed by `Watch` (design D-6), keeping `Owns(&corev1.Pod{})` and `MapRuntimePodToPending`. Verify the controller's existing envtest suite passes unchanged.
- [ ] 4.2 Add explicit pod and ConfigMap deletion on close and on delete (design D-7), keeping `SetControllerReference` under `kube`. Verify an envtest — which runs no garbage collector — observes the pod and ConfigMap gone after a close.
- [ ] 4.3 Move `internal/httpapi` (server, signals, workreport, channelread, contextreport, contextreset, coordinate) to `Store`, replacing `APIReader` reads with `Get`. Verify the httpapi test suite passes unchanged.
- [ ] 4.4 Move `internal/chat` (router, ops, delivery, coordinate, pipelines), `internal/dispatch`, `internal/namewords`, `internal/control-plane` and the remaining controllers that touch a Conversation (signaladapter, signalsource, pipeline, coordinator, channel, channeladapter, drain, mediation) to `Store`. Verify the static check from 1.3 passes on the whole tree.
- [ ] 4.5 Wire `cmd/manager/main.go`: read `STATE_BACKEND`, construct the `kube` store, refuse an unknown name at startup with the name in the error. Verify the manager starts with the variable unset and exits non-zero on `STATE_BACKEND=nosuch`.

## 5. Chart selection — deployment-engineer

- [ ] 5.1 Add `stateStore.backend` to `chart/values.yaml`, default `kube`, with the comment stating the one-backend rule and that `kube` renders no bundle. Verify `helm template` renders with the default.
- [ ] 5.2 Add `agentops.stateStoreGuard` to `_helpers.tpl` beside `defaultRuntimeGuard`: fail the render naming the value on an unknown backend, on a selected backend whose bundle is disabled, or on two backend bundles enabled. Verify a chart render test for each of the three failures and for the default passing.
- [ ] 5.3 Render `STATE_BACKEND` into the manager Deployment from the selector. Verify the chart render test pins the env var NAME and value.

## 6. Unit tests

- [ ] 6.1 `cd platform/manager && KUBEBUILDER_ASSETS=... go test ./...` green in the worktree, including the conformance suite, the static check and the reconciler's event-sequence test.
- [ ] 6.2 `cd chart && go test ./...` green in the worktree for the guard and the env var.
- [ ] 6.3 `go test -count=1 -coverpkg=./... -coverprofile=coverage.out ./...` in `platform/manager`: `internal/state/` covered by the suite, no package below its previous coverage.

## 7. System tests

- [ ] 7.1 Run the system pack unchanged against this worktree's manager image and chart: locally per `build-test.md`, or dispatched with `gh workflow run system-smoke.yml --ref change/state-store`. Verify every lane passes without a test edited — this is the zero-behaviour-change acceptance criterion from the spec.

## 8. Documentation

### Reference docs

- [ ] 8.1 `docs/concepts.md`: the state store as a declared home, the restart-resilience matrix gains its backend column and a `kube` row saying "the Kubernetes API's own guarantee".
- [ ] 8.2 `docs/testing.md`: the store conformance tier, what it decides, how the bench is read.
- [ ] 8.3 `docs/configuration.md`: `stateStore.backend`, the one-backend rule, `kube` the default.
- [ ] 8.4 `docs/CHANGELOG.md`: the new values key, no upgrade step, no CRD change.
- [ ] 8.5 `.claude/rules/invariants.md`: "else Kubernetes API" becomes "else the state store", the runtime-pod row names explicit release. `.claude/rules/structure.md`: `internal/state/` in the manager's map.
- [ ] 8.6 Re-run `python3 .github/scripts/docs-generate.py` and `--check` for the new chart value. Verify `--check` passes.

### Adopter site

- [ ] 8.7 `docs/configuration.md` is the adopter-facing page for the key — confirm the entry reads for an adopter choosing nothing (the default needs no action).
- [ ] 8.8 Confirm `docs/installation.md`, `docs/getting-started.md`, the landing page and `README.md` need no change: a default install is identical. Record the verdict here.
