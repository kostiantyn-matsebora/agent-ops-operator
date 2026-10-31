## 1. The contract — backend-developer

- [x] 1.1 Create `platform/manager/internal/state/` with the domain records (conversation control record, run, input payload — Signal joins with `signal-persistence`) and the `Store` interface per design D-2: `Get`, `Create`, `Update` with CAS, `Delete`, `List` by the declared index set, `Watch` with a resume point, `AppendRun`, `ListRuns`, `PutBlob`/`GetBlob`/`DeleteBlob`, `expiresAt` on a record. Verify `go vet ./...` passes and the package has no import of `k8s.io/` or `sigs.k8s.io/`.
- [x] 1.2 Define the conflict and not-found error values and the opaque `Revision` type. Verify a unit test distinguishes conflict from any other error with `errors.Is`. Also added `ErrAlreadyExists`, distinct from `ErrConflict`, for `Create` on an existing key — the spec calls this out as its own case.
- [ ] 1.3 Add the static check: a test in `internal/state/` that walks every manager package and fails on a Kubernetes client call whose argument is a Conversation or a ConversationInput outside `internal/state/kube`. Verify it fails on the current tree (before section 4) and names the package. **Deferred to section 4.** This check is designed to fail until section 4's call-site migration lands. Adding it before any call site has moved would fail `go test ./...` for the whole module. Add it in the same change that does section 4.

## 2. The `kube` backend — backend-developer

- [x] 2.1 Implement `internal/state/kube` mapping records to the existing `v1alpha1` types, byte-for-byte: `Get` via the apiserver, `Update` via the status subresource with `resourceVersion` as the revision, `Create`, `Delete`. Verify an envtest round-trips a conversation and the stored object's spec and status equal a fixture written by today's code path.
- [x] 2.2 Implement `List` over the existing field indexes (`index.go`), one index per declared lookup, and a test asserting the backend serves every index the contract declares — the list in the test is the contract's own, so a later index added without a `kube` implementation fails here. Verify envtest finds a conversation by `initiationId`, by `pipelineRef`+phase and by phase. Implemented as a full `client.List` filtered in Go, not a field indexer/cache. This backend depends on `client.WithWatch` alone, by design, with no `cache.Cache` to register an indexer on. Fine at today's scale — the contract does not mandate the mechanism.
- [x] 2.3 Implement `Watch` over the informer, delivering `(key, revision, record)` and resuming from a kept `resourceVersion`. Verify envtest: events for a create, two updates and a delete arrive in order, and a consumer resuming from the first update's revision sees the second update and the delete. Implemented directly over the apiserver's own watch via `client.WithWatch`, not a cache-backed informer, for the same "no cache" reason as 2.2. A real finding surfaced here: `Update`/`Create` each land as two apiserver calls, the main object then the status subresource, so the raw watch stream can carry a spec-only intermediate notification before the matching status write lands. This is forwarded as-is, trusting the raw payload — the contract's "duplicates permitted, no gap" wording already covers it, and a consumer CAS-ing off an intermediate revision is refused exactly like any other stale write.
- [x] 2.4 Implement `AppendRun` as an internal read-append-CAS loop on `status.runs[]`, retried on conflict, taking no revision from the caller, and `ListRuns` in order (design D-5). Verify envtest appends N runs from two goroutines at once and reads all N back in order with no conflict surfaced.
- [x] 2.5 Implement blobs as `ConversationInput` objects. Verify envtest puts, gets and deletes a payload and a deleted key reads absent.
- [x] 2.6 Implement `expiresAt` as a stored field with the backend's own sweep. Verify envtest: a record past its expiry is omitted by `Get` and by every `List`, and a delete event follows on `Watch`. Stored as an annotation, not a CRD field, since there is no CRD schema change in this change. `Store.Run` starts the sweep. It is not auto-started by `NewStore`, since section 4 — the only thing that would call it in production — hasn't landed yet.

## 3. The conformance suite and bench — testing-specialist

- [x] 3.1 Create `internal/state/conformance` with `Run(t, newStore)`: one scenario per requirement of `specs/state-store/spec.md` — CAS race, stale write refused, every declared index, feed order, feed resume with no gap, stale snapshot caught by CAS, expiry invisible then deleted, runs in order, concurrent appends with no conflict, blob lifecycle, scope isolation, two-write tolerance. Verify every scenario names the spec requirement it covers in its test name.
- [x] 3.2 Add `Bench(b, newStore)` reporting bytes written per run, allocations per write and resident records after a listing. Verify `go test -bench . -benchmem ./internal/state/...` prints all three for the `kube` backend.
- [x] 3.3 Wire the `kube` backend into the suite under envtest. Verify `go test ./internal/state/...` runs every scenario green with `KUBEBUILDER_ASSETS` set. Wiring this up surfaced the 2.3 finding above. The suite's feed scenarios were adjusted to look for a write's expected final state among what a backend actually produces, rather than assuming the very first notification already is it, which is what let the real finding surface instead of being masked.

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
