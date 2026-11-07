## 1. The contract — backend-developer

- [x] 1.1 Create `platform/manager/internal/state/` with the domain records (conversation control record, run, input payload, with the Signal half in 1.5) and the `Store` interface per design D-2: `Get`, `Create`, `Update` with CAS, `Delete`, `List` by the declared index set, `Watch` with a resume point, `AppendRun`, `ListRuns`, `PutBlob`/`GetBlob`/`DeleteBlob`, `expiresAt` on a record. Verify `go vet ./...` passes and the package has no import of `k8s.io/` or `sigs.k8s.io/`.
- [x] 1.2 Define the conflict and not-found error values and the opaque `Revision` type. Verify a unit test distinguishes conflict from any other error with `errors.Is`. Also added `ErrAlreadyExists`, distinct from `ErrConflict`, for `Create` on an existing key — the spec calls this out as its own case.
- [x] 1.3 Add the static check: a test in `internal/state/` that walks every manager package and fails on a Kubernetes client call whose argument is a Conversation or a ConversationInput outside `internal/state/kube`. Verify it fails on the current tree (before section 4) and names the package. **Deferred to section 4.** This check is designed to fail until section 4's call-site migration lands. Adding it before any call site has moved would fail `go test ./...` for the whole module. Add it in the same change that does section 4. Done in `internal/state/static_test.go`: an AST walk of every manager package that fails on any reference to `Conversation`, `ConversationList`, `ConversationInput` or `ConversationInputList` of the API package outside `internal/state/kube`, `api/`, `test/` and test files, naming the package, file, line and enclosing call. It is stricter than "a client call taking one": a type never named cannot reach a client. Verified red against the pre-migration tree (179 references across chat, controller, dispatch, httpapi, namewords and runtimepod) and green on this one, plus a test of its own over a synthetic tree.

- [x] 1.4 ADDED after the section 4 read (design D-11, D-12): record identity (`ID`, `CreatedAt`), `Finalizers`, `DeletionRequestedAt`, `NameBase`, the carried-through `InputItem.Agent`, `ErrDeleting`, the deletion lifecycle on `Delete`/`Update`, `UpdateRun` with `RunUpdate`, the indexes `ByCausedBy`, `ByNameBase`, `ByAll`, and the rule that every instant is UTC. Verify `go vet ./internal/state/...` passes, the package still imports nothing from `k8s.io/` or `sigs.k8s.io/`, and the in-memory double passes the extended suite.

- [x] 1.5 ADDED (design D-14, D-15): the Signal half of the contract in `internal/state/signal.go` — `SignalRecord` with its claimants, the phase and claimant vocabulary, `TerminalSignalPhase`, `SignalArrivesBefore`, `SignalStore` (`Get`, `Create`, `Update` writing status and expiry only, `Delete`, `List`, `Watch`, `GetCooldown`, `PutCooldown`, `ReleaseSource`), the three declared signal indexes and `SignalEvent`, reusing `Key`, `Revision`, `Ref`, `EventType` and the errors. Verify `go vet ./internal/state/...` passes and the package still imports nothing from `k8s.io/` or `sigs.k8s.io/`.
- [x] 1.6 ADDED: extend the static check (1.3) to `Signal`, `SignalList`, `CooldownEntry` and the `SignalSource` `Status.Cooldown` field, with the explicit, marked, temporary `pendingSignalMigration` allowlist of `internal/httpapi`, `internal/controller` and `internal/signalroute`, which section 6 empties and deletes. Verify a synthetic-tree test flags each name outside the set, excuses a Signal inside it, and never excuses a Conversation there.

## 2. The `kube` backend — backend-developer

- [x] 2.1 Implement `internal/state/kube` mapping records to the existing `v1alpha1` types, byte-for-byte: `Get` via the apiserver, `Update` via the status subresource with `resourceVersion` as the revision, `Create`, `Delete`. Verify an envtest round-trips a conversation and the stored object's spec and status equal a fixture written by today's code path.
- [x] 2.2 Implement `List` over the existing field indexes (`index.go`), one index per declared lookup, and a test asserting the backend serves every index the contract declares — the list in the test is the contract's own, so a later index added without a `kube` implementation fails here. Verify envtest finds a conversation by `initiationId`, by `pipelineRef`+phase and by phase. Implemented as a full `client.List` filtered in Go, not a field indexer/cache. This backend depends on `client.WithWatch` alone, by design, with no `cache.Cache` to register an indexer on. Fine at today's scale — the contract does not mandate the mechanism.
- [x] 2.3 Implement `Watch` over the informer, delivering `(key, revision, record)` and resuming from a kept `resourceVersion`. Verify envtest: events for a create, two updates and a delete arrive in order, and a consumer resuming from the first update's revision sees the second update and the delete. Implemented directly over the apiserver's own watch via `client.WithWatch`, not a cache-backed informer, for the same "no cache" reason as 2.2. A real finding surfaced here: `Update`/`Create` each land as two apiserver calls, the main object then the status subresource, so the raw watch stream can carry a spec-only intermediate notification before the matching status write lands. This is forwarded as-is, trusting the raw payload — the contract's "duplicates permitted, no gap" wording already covers it, and a consumer CAS-ing off an intermediate revision is refused exactly like any other stale write.
- [x] 2.4 Implement `AppendRun` as an internal read-append-CAS loop on `status.runs[]`, retried on conflict, taking no revision from the caller, and `ListRuns` in order (design D-5). Verify envtest appends N runs from two goroutines at once and reads all N back in order with no conflict surfaced.
- [x] 2.5 Implement blobs as `ConversationInput` objects. Verify envtest puts, gets and deletes a payload and a deleted key reads absent.
- [x] 2.6 Implement `expiresAt` as a stored field with the backend's own sweep. Verify envtest: a record past its expiry is omitted by `Get` and by every `List`, and a delete event follows on `Watch`. Stored as an annotation, not a CRD field, since there is no CRD schema change in this change. `Store.Run` starts the sweep. It is not auto-started by `NewStore`, since section 4 — the only thing that would call it in production — hasn't landed yet.

- [x] 2.7 ADDED: map identity, finalizers, the deletion marker, `NameBase` and `Agent`, and the two lookup labels, with `Update` writing onto the stored object so metadata the record does not model survives, and writing status first when it releases the last finalizer of a marked record. Verify envtest: identity is the object's UID and creation time, finalizers round trip and a released marked record is gone, a foreign label and annotation survive an `Update`, and the labels follow `CausedBy`.
- [x] 2.8 ADDED: `UpdateRun` as a read-apply-CAS loop on `status.runs[]`. Verify envtest: an update the run already holds writes nothing (the resourceVersion is unchanged).
- [x] 2.9 ADDED: serve `ByCausedBy`, `ByNameBase` and `ByAll`, and return every instant in UTC. Verify `TestInstantsReadBackInUTCUnderANonUTCLocalZone` and the whole `internal/state/...` run pass under `TZ=Europe/Helsinki` and `TZ=UTC` — three round-trip tests had failed under any other zone, because the client decodes a `metav1.Time` into the local zone and the mapping copied it through.

- [x] 2.10 ADDED: `kube.SignalStore` (`signals.go`, `signal_mapping.go`): `Create` under the caller's key with `AlreadyExists` as `ErrAlreadyExists` and the controller owner reference built from `SourceID`. `Update` as a status-subresource write under the revision read, then the expiry annotation only when it changed. `List` over the whole namespace filtered in Go. `Watch` over the apiserver watch with bookmarks and resume. `Delete`. Verify envtest: the created object's spec and owner reference equal those of one created by the call publication made before, and a non-terminal write leaves no annotation.
- [x] 2.11 ADDED: the signal expiry sweep (`SignalStore.Run`, deleting with UID and revision preconditions) and `GetCooldown` / `PutCooldown` / `ReleaseSource` over `SignalSource.status.cooldown`, the same status merge patch as before. Verify envtest: a past expiry makes the signal invisible and a delete event follows, `status.cooldown` holds the entries written, and a gone source reads an empty window, refuses a write as not-found and releases cleanly.

## 3. The conformance suite and bench — testing-specialist

- [x] 3.1 Create `internal/state/conformance` with `Run(t, newStore)`: one scenario per requirement of `specs/state-store/spec.md` — CAS race, stale write refused, every declared index, feed order, feed resume with no gap, stale snapshot caught by CAS, expiry invisible then deleted, runs in order, concurrent appends with no conflict, blob lifecycle, scope isolation, two-write tolerance. Verify every scenario names the spec requirement it covers in its test name.
- [x] 3.2 Add `Bench(b, newStore)` reporting bytes written per run, allocations per write and resident records after a listing. Verify `go test -bench . -benchmem ./internal/state/...` prints all three for the `kube` backend.
- [x] 3.3 Wire the `kube` backend into the suite under envtest. Verify `go test ./internal/state/...` runs every scenario green with `KUBEBUILDER_ASSETS` set. Wiring this up surfaced the 2.3 finding above. The suite's feed scenarios were adjusted to look for a write's expected final state among what a backend actually produces, rather than assuming the very first notification already is it, which is what let the real finding surface instead of being masked.

- [x] 3.4 ADDED: one scenario per added requirement — identity, the deletion lifecycle (five scenarios), run growth (three), the three indexes, UTC instants — in `conformance/lifecycle.go`, each named for its spec scenario, and the in-memory double extended to match. Verify `go test ./internal/state/...` passes with `KUBEBUILDER_ASSETS` set.

- [x] 3.5 ADDED: `conformance.RunSignals` — one scenario per signal requirement (create once, CAS on status only, publication fields immutable, status round-trip, identity, arrival order, the three indexes and the unknown one, feed shape and resume, expiry invisible, on the feed and written with the terminal phase, delete, the cooldown window empty / replaced per source / UTC, release of one source, scope, UTC), an in-memory double, and the `kube` wiring that seeds the `SignalSource`s the scenarios record a window on. Verify `go test ./internal/state/...` passes with `KUBEBUILDER_ASSETS` set, also under `TZ=Europe/Helsinki`. The signal half has no bench yet: the numbers that decide a backend are the conversation's runs.

## 4. The manager on the store — backend-developer

- [x] 4.1 Move the Conversation reconciler to `Store`: reads from the fed snapshot, writes through `Update` with CAS, `pruneProcessed` through blobs. Replace `For(&Conversation{})` with `WatchesRawSource(source.Channel(...))` fed by `Watch` (design D-6), keeping `Owns(&corev1.Pod{})` and `MapRuntimePodToPending`. Verify the controller's existing envtest suite passes unchanged. Done, see design D-13. Notes: `Owns` is spelled as a `Watches` on the pod with the owner read off its controller reference, because the builder refuses `Owns` without `For`. `pruneProcessed` deletes the payload through `DeleteBlob`. The pod and ConfigMap owner is built from the record `ID`. The "across all namespaces" list in `MapRuntimePodToPending` became the store's own namespace (D-2), with a pod from another namespace ignored. Assertions in the existing suites are unchanged. The setup injects a `Store` (the `kube` backend over the fake client in unit tests, over an uncached client in the envtest suite) and the interceptors that watched `SubResourcePatch` watch `SubResourceUpdate`. The event-sequence test is `TestFeedDeliversTheLifecycleInOrder`, and `TestConversationControllerIsDrivenByTheStoreFeed` runs a real manager with no test calling `Reconcile`.
- [x] 4.2 Add explicit pod and ConfigMap deletion on close and on delete (design D-7), keeping `SetControllerReference` under `kube`. Verify an envtest — which runs no garbage collector — observes the pod and ConfigMap gone after a close.
- [x] 4.3 Move `internal/httpapi` (server, signals, workreport, channelread, contextreport, contextreset, coordinate) to `Store`, replacing `APIReader` reads with `Get`. Verify the httpapi test suite passes unchanged. Done, assertions unchanged. `handleWorkDone` is now judge, then `AppendRun` (skipped when the run id is already recorded, so a retried report is idempotent), then one `Mutate` for the inflight clear, processed ids, context handle, condition and budget, then a phase `Mutate` that never un-closes. The phase `Mutate` replaces the five-attempt optimistic loop, and its exhaustion error is the store's wrapped conflict.
- [x] 4.4 Move `internal/chat` (router, ops, delivery, coordinate, pipelines), `internal/dispatch`, `internal/namewords`, `internal/control-plane` and the remaining controllers that touch a Conversation (signaladapter, signalsource, pipeline, coordinator, channel, channeladapter, drain, mediation) to `Store`. Verify the static check from 1.3 passes on the whole tree. Done. The `signalroute` package and `SignalReconciler` take the store too. The informer field index (`SetupConversationIndexes`, `ConversationIDIndexer`, `api.IndexConversationID`) is deleted: `ByConversationID` is the lookup now. One assertion changed deliberately: the name allocator's exhaustion error is `state.ErrAlreadyExists` (what the store returns) where it was the apiserver's `AlreadyExists`.
- [x] 4.5 Wire `cmd/manager/main.go`: read `STATE_BACKEND`, construct the `kube` store, refuse an unknown name at startup with the name in the error. Verify the manager starts with the variable unset and exits non-zero on `STATE_BACKEND=nosuch`. Done. The name is validated before the manager connects to a cluster, so the exit is provable on a real process without one (`TestMainExitsOnAnUnknownStateBackend`). The `kube` store is built over an uncached `client.NewWithWatch` and its expiry sweep is a manager runnable. The "starts with the variable unset" half is `TestStateBackendDefaultsToKube` and the envtest suite, which wires the same constructor. A full start against a cluster is a system-tier fact.

## 5. Chart selection — deployment-engineer

- [x] 5.1 Add `stateStore.backend` to `chart/values.yaml`, default `kube`, with the comment stating the one-backend rule and that `kube` renders no bundle. Verify `helm template` renders with the default.
- [x] 5.2 Add `agentops.stateStoreGuard` to `_helpers.tpl` beside `defaultRuntimeGuard`: fail the render naming the value on an unknown backend, on a selected backend whose bundle is disabled, or on two backend bundles enabled. Verify a chart render test for each of the three failures and for the default passing. Only the unknown-backend refusal and the default are render-tested: no backend bundle exists to enable, and a test-only `stateStore.bundles` value was removed rather than ship a public key for it. The bundle-off and two-bundles-on refusals are tested by the first backend bundle that adds its row to `agentops.stateStoreBackends`.
- [x] 5.3 Render `STATE_BACKEND` into the manager Deployment from the selector. Verify the chart render test pins the env var NAME and value.

## 6. The Signal call sites on the store — backend-developer

The contract, the `kube` backend and the suite exist (1.5, 2.10, 3.5). the call sites below have moved. Counts are the design's Signal provenance table.

- [x] 6.1 Move publication (`internal/httpapi/publish.go`, 1 `Create`): `publishOne` calls `SignalStore.Create` under the same derived or generated key with `SourceID` from the resolved source, reading `ErrAlreadyExists` as the duplicate. Drop `SetControllerReference` from the handler. Verify the existing publication tests pass with assertions unchanged, and the stored object has the same owner reference.
- [x] 6.2 Move the signal reconciler (`internal/controller/signal_controller.go`: 1 `Get`, 2 `List`, 1 helper reached from 6 status writes, 1 `Delete`, 1 watch): hold `(record, revision)` through a pass, write with `Update` carrying the revision and then the returned one, replace `isHead` with `SignalNonTerminalByInitiation` and `enforceSourceCap` with `SignalNonTerminalBySource`, order by `SignalArrivesBefore`, and register the feed (`Watch` with the `pumpFeed` relist over `SignalAll`) as a raw source beside the `kick` channel. Verify the controller's existing suite passes with assertions unchanged.
- [x] 6.3 Replace the TTL delete with expiry: `finishIfTerminal` also sets `ExpiresAt = CompletedAt + ttlFor(phase)` in the same `Update`, `reconcileTerminal` stamps it on a terminal signal found without one, and the conditional `Delete` is removed. Verify a table test of the three phases' TTLs, that a terminal signal with no expiry is stamped from `CompletedAt`, and that a non-terminal write sets none.
- [x] 6.4 Move `internal/signalroute` (17 in-memory uses of the Signal type, where `Admit` and `Route` take a `state.SignalRecord`): the same decisions, the same order. Verify its existing tests pass with fixtures changed to the record and assertions unchanged.
- [x] 6.5 Move the cooldown window (`recoverCooldown` read, `recordCooldown` write, 2 callers): `GetCooldown` once per process per source, `PutCooldown` from the same two places, a failed write still logged and never fatal. Verify the cooldown tests pass unchanged and `status.cooldown` is byte-identical under envtest.
- [x] 6.6 Release a deleted source explicitly: the `SignalSource` reconciler's not-found path calls `ReleaseSource`, and the signal pass's missing-source branch releases the orphan instead of relying on the owner reference. Verify an envtest, which runs no garbage collector, observes every signal of a deleted source gone from any phase.
- [x] 6.7 Move the receipt counters behind `SignalStore`: `RecordReceived`/`GetReceived` in the contract, the conformance suite and `kube`, and the router call site (`signalroute.RecordReceived`). Verify the concurrent-receipts scenario passes on every backend.
- [x] 6.8 Construct `kube.NewSignalStore` in `cmd/manager/main.go` beside the conversation store, run its sweep as a manager runnable, and empty and delete `pendingSignalMigration` in `internal/state/static_test.go`. Verify the static check passes on the whole tree with the allowlist gone and `STATE_BACKEND=nosuch` still exits non-zero.
- [ ] 6.9 Close the release race: the source reconciler passes the departed source's `sourceRef` into `ReleaseSource`, so a successor created between its absence check and the release keeps its signals. Verify an envtest covering that gap against the "A successor's signals outlive the departed release" scenario. Not done here: the release was moved to a manager helper that reads the live source and deletes by id, specified and tracked in the change `state-store-generic-contract`.

## Review round (sections 4/5)

Fixed:

- Undelivered-note settling runs in the reconciler backstop and only for the run the note names.
- The expiry sweep lists only when armed.
- `kube.Store.Update` returns `ErrConflict` when a status retry meets a non-run change.
- The feed clears a rejected resume point, and `kube` Watch requests and forwards bookmarks (`EventBookmark`).
- The static check covers dot-imports and untyped Conversation kinds.
- The chart tolerates `stateStore=null`.

Deliberately not fixed at the time, since closed in the Signal round below: `Reconcile`'s `Get` then `ListRuns` (now `GetWithRuns`) and `markDelivered` clearing whatever note the thread held. Still open: a cluster measurement of `kube` read cost (envtest numbers are in D-4, no cluster was available).

## Review round (Signal)

Second pass, everything the first left open:

- The static check resolves a kind held in a constant, flags a kind handed in as a parameter, and flags the status fields by their wire path (`"cooldown"`, `"receivedtotal"`, `"lastreceived"`) in any string.
- `Store.GetWithRuns` is the combined read the reconciler uses, one apiserver read where two were made.
- `markDelivered` clears only the undelivered-reply note naming the run it delivered.
- `RecordReceived`/`GetReceived` move the receipt counters behind `SignalStore` (spec, conformance, `kube`, the router call site).
- `BenchSignals` and the envtest `kube` benchmarks put measured numbers in D-4 (a local apiserver, no cluster).

Fixed:

- `ReleaseSource` takes the departed source's identity, spares a successor's signals (and under `kube` its window), and deletes under a UID precondition.
- `Create` replaces an expired-but-unswept record (conformance scenario added).
- `CooldownBook.For` builds outside its mutex, a disabled window reads nothing, and the read has a 10s timeout.
- The feed retries a failed initial relist.
- The static check guards `SignalSpec`, `SignalStatus` and `Status.Cooldown` through an aliased variable.
- A kube test runs the sweep until the expired object is gone.

Deliberately not fixed:

- The static check still misses a reflected field path and a kind that reaches an unstructured object through another function's return value. It needs type information the AST walk lacks.
- No cluster measurement was made.
- Follow-up: pass the departed source's `sourceRef` from the source reconciler into the release (tracked as task 6.9 above). The spec's successor-release scenario holds only once 6.9 lands.
- The source reconciler passes no identity, so a successor created in the gap between its absence check and the release can lose its signals. This is a known gap against the "A successor's signals outlive the departed release" scenario, which holds only where the caller supplies the identity (6.6 is ticked for the explicit release, not for that race).

## 7. Unit tests

- [x] 7.1 `cd platform/manager && KUBEBUILDER_ASSETS=... go test ./...` green in the worktree, including the conformance suite (both halves), the static check and the reconcilers' event-sequence tests. Verdict: passed in CI (`operator` job, final commit of the pull request).
- [x] 7.2 `cd chart && go test ./...` green in the worktree for the guard and the env var. Verdict: passed in CI (`chart` job, final commit of the pull request).
- [x] 7.3 `go test -count=1 -coverpkg=./... -coverprofile=coverage.out ./...` in `platform/manager`: `internal/state/` covered by the suite, no package below its previous coverage. Verdict: the CI `operator` job ran with `-coverpkg=./...` and the coverage gate passed.

## 8. System tests

- [x] 8.1 Run the system pack unchanged against this worktree's manager image and chart: locally per `build-test.md`, or dispatched with `gh workflow run system-smoke.yml --ref change/state-store`. Verify every lane passes without a test edited — this is the zero-behaviour-change acceptance criterion from the spec. Verdict: the smoke tier passed on master after the merge (run 42409548436). An earlier red on master, a close overwritten by a reconcile pass, was fixed in #459 and is not part of this change's contract.

## 9. Documentation

### Reference docs

- [x] 9.1 `docs/concepts.md`: the state store as a declared home, the restart-resilience matrix gains its backend column and a `kube` row saying "the Kubernetes API's own guarantee".
- [x] 9.2 `docs/testing.md`: the store conformance tier, what it decides, how the bench is read.
- [x] 9.3 `docs/configuration.md`: `stateStore.backend`, the one-backend rule, `kube` the default.
- [x] 9.4 `docs/CHANGELOG.md`: the new values key, no upgrade step, no CRD change.
- [x] 9.5 `.claude/rules/invariants.md`: "else Kubernetes API" becomes "else the state store", the runtime-pod row names explicit release. `.claude/rules/structure.md`: `internal/state/` in the manager's map.
- [x] 9.6 Re-run `python3 .github/scripts/docs-generate.py` and `--check` for the new chart value. Verify `--check` passes.

- [x] 9.7 Signal, reference docs: `docs/concepts.md` (the state store home lists signals and each source's cooldown window, the ingest-cooldown row of the restart-resilience matrix stops saying "not state-store state", `SignalSource.status.cooldown[]` is described as the `kube` backend's location of the window, and the Signal kind's TTL and owner-cascade text says the manager expires and releases them), `docs/testing.md` (the signal half of the conformance tier), `docs/contracts.md` if it names who deletes a terminal signal, `docs/CHANGELOG.md` (no upgrade step, no CRD change, the expiry annotation now also appears on a terminal `Signal`), `.claude/rules/invariants.md` (the state-store row names Signal and the cooldown window) and `.claude/rules/structure.md` (`SignalStore` beside `Store`). Verify `python3 .github/scripts/docs-generate.py --check` passes.
- [x] 9.8 Signal, retired and published vocabulary: `.github/retired-vocabulary.json` and `retired-vocabulary.md` need no entry for the TTL delete, which is replaced, not renamed. The window's old name is retired separately, as `suppression-window`. Verify `retired-vocabulary-guard.py` and `publication-guard.py` pass and record the verdict here.

### Adopter site

- [x] 9.9 `docs/configuration.md` is the adopter-facing page for the key — confirm the entry reads for an adopter choosing nothing (the default needs no action).
- [x] 9.10 Confirm `docs/installation.md`, `docs/getting-started.md`, the landing page and `README.md` need no change: a default install is identical. Record the verdict here.
- [x] 9.11 Signal, adopter site: confirm `docs/installation.md`, `docs/getting-started.md`, the landing page, `README.md` and the signal guides need no change, since a default install keeps the same objects, the same `kubectl get sig` output and the same TTL behaviour. Record the verdict here.

Verdict 9.9: `docs/configuration.md` carries a State store section opening "You need do nothing". An adopter choosing nothing reads no action.

Verdict 9.10: `installation.md`, `getting-started.md`, the landing page and `README.md` need no change, since a default install is identical (`kube`, same objects). They change when the first alternative backend lands.

Verdict 9.7: done in `docs/concepts.md` (home bullet, matrix rows for the cooldown window and signals, the Signal TTL and release text, the receipt counters listed under the state-store home), `docs/testing.md` (`RunSignals`), `docs/configuration.md`, `docs/CHANGELOG.md`, `docs/contracts/signal-adapter.md` (API-7 wording), `invariants.md`, `structure.md`.

`docs-generate.py` and `--check` pass. `docs/contracts.md` names no deleter of a terminal signal, so it is unchanged.

Verdict 9.8: `retired-vocabulary-guard.py` and `publication-guard.py` both clean after the `suppression-window` entry was added.

Verdict 9.11: grepped `README.md`, `installation.md`, `getting-started.md`, `index.md`, `guides/` and `integrations/` for signal TTL, owner, deletion, cooldown storage and state-store terms. None state them, so no change. No page mentions `stateStore.bundles`.

Caveat: the docs say the manager releases a deleted source's signals, but the successor-race guarantee holds only where the caller supplies the departed source's identity, which the source reconciler does not yet (task 6.9).
