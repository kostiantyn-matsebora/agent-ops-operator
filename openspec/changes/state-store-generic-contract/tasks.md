## 1. Sequencing gate

- [x] 1.1 Confirm PR #454 (`state-store`) is merged and archived, rebase this branch on master, and re-run `openspec validate state-store-generic-contract`. Verify the `state-store` and `state-durability` deltas validate against the published specs and that every MODIFIED block still equals the published text it replaces apart from this change's edits. Nothing below starts before this is ticked. Verdict: `state-store` and `signal-persistence` are archived and branch rebased on master. Validation found three omitted published scenarios (console-topology signals-by-watch, the run concurrency scenario, the successor-outlives-release scenario) and now passes. The heavily rewritten requirements differ from their published text by design; the lightly edited ones (signal lookup, restart-resilience home, snapshots) match it to within the change's own edits.

## 2. The generic contract — backend-developer

The call sites keep their call names. The handle each holds changes type.

- [ ] 2.1 Define the capability interfaces in `platform/manager/internal/state/`: `Collection[T]` (`Get`, `Create`, `Update` with CAS, `Delete`, `List(Index[T], value)`, `Watch`), `Expiring[T]`, `Appended[P, T]` (`Append`, `List`, `Grow`, `GetWithItems`, a listing with items), `Blobs`, with `Index[T]`, `Event[T]`, `Decl[T]` (name, indexes, expiry, merge) and `Mutate[T]`. Delete `Store` and `SignalStore`. Verify `go vet ./internal/state/...` passes and the package imports nothing from `k8s.io/` or `sigs.k8s.io/`.
- [ ] 2.2 Define the `Stores` inventory (`Conversations`, `Runs`, `Signals`, `Cooldowns`, `Receipts`, `AdapterState`, `Blobs`), each field typed by exactly its capabilities, with the declarations beside it, the run merge of the grow-only edit as the `Runs` declaration, and the fourth signal index (one source, any phase). Verify a compile-time test that `Conversations` has no `Append` and no expiry, `Signals` has no `Append`, and a conversation index does not type-check against `Signals.List`.
- [ ] 2.3 Move the domain operations above the contract: the cooldown read and replace and the receipt count into `internal/signalroute` as helpers over `Cooldowns` and `Receipts` through `Mutate`, and the source release into the signal controller's package as a helper over `Signals.List` plus `Delete` under the listed revision plus the window clear. Verify their existing tests pass with assertions unchanged. Add the release rules of the spec: read the live source after listing, delete each signal under its listed revision, retry a conflict with back-off, and clear the window only when no source holds the name. Verify an envtest recreates a source under the same name mid-release and its signals survive.

## 3. The `kube` backend as one generic constructor — backend-developer

- [ ] 3.1 Replace `kube.Store` and `kube.SignalStore` with `kube.Open[T](client, Layout[T])`, `OpenExpiring`, `OpenAppended[P, T]` and `OpenBlobs`, where `Layout[T]` carries the object kind, the record mapping both ways, the status subresource rule, the labels serving each index, the expiry annotation and the owner-link hint. Verify every existing `internal/state/kube` test passes with assertions unchanged.
- [ ] 3.2 Write the layouts: conversations and runs onto `Conversation`, signals onto `Signal`, cooldown windows and receipt counters onto `SignalSource.status` by the same status merge patch, blobs onto `ConversationInput`. Verify the byte-identical envtest fixtures of the earlier change pass unchanged.
- [ ] 3.3 Write the adapter state layout: key `<surface>/<name>/<key>` onto the `agentops.dev/adapter-state-<key>` annotation of the `Channel` or `SignalSource`, the same merge patch the handlers made. Verify envtest: a value written through the collection reads back through the old handlers' code path byte-identical, and the reverse.
- [ ] 3.4 Serve the listing with items and the put event carrying them under `kube` (free from the object). Verify the two scenarios of the spec in envtest.
- [ ] 3.5 Build `Stores` once in `platform/manager/cmd/manager/main.go` from the selected backend and hand typed fields to the reconcilers, the HTTP API and chat. Verify the default-backend and unknown-backend tests of the earlier change pass.

## 4. The generic conformance suite — testing-specialist

- [ ] 4.1 Rewrite `internal/state/conformance` as `Run[T]`, `RunExpiring[T]`, `RunAppended[P, T]`, `RunBlobs` and `RunStores(t, open func() Stores)` driven by the inventory, each scenario named for its spec requirement. Carry the existing scenarios over and dissolve the kind-specific ones into fixtures. Add the merge idempotence and commutativity scenario and the "a kind cannot be left out" check. Verify `go test ./internal/state/...` passes under `KUBEBUILDER_ASSETS` and `TZ=Europe/Helsinki`.
- [ ] 4.2 Make `Bench[T]` per collection, keeping the earlier numbers comparable. Verify `go test -bench . -benchmem ./internal/state/...` prints bytes written per run, allocations per write and resident records for each field of `Stores`.
- [ ] 4.3 Reduce the `kube` backend's test file to one `RunStores` call over envtest plus its layout tests. Verify the suite fails, naming the field, when one layout is removed.

## 5. Call sites and handlers on the typed handles — backend-developer

- [ ] 5.1 Move every call site of the earlier change onto the typed fields of `Stores`. Verify every manager test suite passes with assertions unchanged and the static check passes on the whole tree.
- [ ] 5.2 Move the `/channel/state` and `/signal/state` handlers onto `Stores.AdapterState`, and release a deleted Channel's or SignalSource's keys from the owning reconciler. Add the adapter state annotation prefix to the manager's static check. Verify the handler tests pass with assertions unchanged, the adapter contracts' documented behaviour is unchanged, and the check flags the prefix in a synthetic tree.

## 6. The state read surface — api-architect, backend-developer

- [ ] 6.1 Add `spec.stateAccess` (bool, default false, documented as "grants this adapter's derived token the manager's state read surface. Silence grants nothing.") to `ChannelAdapterSpec` and `SignalAdapterSpec` in `platform/manager/api/v1alpha1/`, and regenerate deepcopy and `chart/crds/` with the two `controller-gen` commands in `build-test.md`. Verify `go build ./...` passes.
- [ ] 6.2 Serve the surface in `platform/manager/internal/httpapi/state.go`: the three reads and two feeds, SSE frames `put`/`delete`/`bookmark`/`resync` with `id:` as the resume revision, any query parameter but `since` refused by name, and the wire record of the design. Verify httpapi tests: the listing equals the store's whole set with runs, a stream resumed from a kept revision carries a put and a delete made meanwhile, an unknown `since` yields `resync` first, an unknown parameter is refused naming it, and under `kube` the served `spec` and `status` bytes equal the object's.
- [ ] 6.3 Authenticate `/state/*`: the master token, a derived adapter token whose CR declares `stateAccess`, and `STATE_READ_TOKEN` read from the environment in `cmd/manager/main.go`, never from a Secret. Refuse with 403 naming adapter and field, and 401 otherwise. Add `/state/` to `leaderProxyMiddleware`'s bypass beside `/healthz`. Verify tests: an adapter without the field is refused with the adapter named, `STATE_READ_TOKEN` opens `/state/*` and is refused on every `/channel/*` route and on `/control-plane/*`, and a non-leader serves a listing and a feed with no proxy dial.

## 7. Chart — deployment-engineer

- [ ] 7.1 Add `stateReadAuth.{secretName, existingSecret, token}` to `chart/values.yaml` and `state-read-auth.yaml` in the shape of `control-plane-auth.yaml`, failing the render when the token equals `adapterAuth.token` or `controlPlaneAuth.token`, and generating a credential on install and a connected upgrade only. Project `STATE_READ_TOKEN` through `valueFrom.secretKeyRef` into the manager Deployment. Verify render tests for the equal-token refusal and for a cluster-less upgrade minting nothing.
- [ ] 7.2 Set `stateAccess: true` on `ChannelAdapter/console`, remove `conversations` and `signals` from the console Role, remove housekeeping's Role and RoleBinding, set `automountServiceAccountToken: false` on its CronJob and give it `MANAGER_URL` and `STATE_READ_TOKEN` through `valueFrom.secretKeyRef` on the same Secret as the manager's. Admit `app.kubernetes.io/name: agentops-housekeeping` on 8080 in the NetworkPolicy. Verify the console Role test asserts the remaining kinds equal the console's `Kinds`, a housekeeping test asserts no Role, no RoleBinding, automount off and both env names pinned, and `TestEveryWiredFlowIsAllowed` covers housekeeping to the manager.

## 8. Console — frontend-developer, backend-developer

- [ ] 8.1 Add `platform/console/state.go`: list, get and stream over `MANAGER_URL` with the adapter token, `resync` triggering a relist with the existing backoff. Feed the cache from two sources through one delta vocabulary, drop `conversations` and `signals` from `Kinds`, resolve a UID on a miss with `GET /state/conversations/{name}`, and fill the object metadata from `id`, `revision`, `createdAt` and `deletionRequestedAt`. Verify the browser API contract tests keep every assertion with the two kinds fed by a fake of the state client over the same fixtures.
- [ ] 8.2 Add the console's `TestReadsNoStateFromKubernetes` (a stdlib `go/parser` walk, design D-8). Verify it is red on the tree before 8.1 and green after.

## 9. Housekeeping — backend-developer

- [ ] 9.1 Delete `platform/housekeeping/kube.go` and add `state.go` implementing the lister over `GET /state/conversations`, keeping the `runtimeContextId` read and its fallback to the retired `sessionId` and the phase-blind comment, leaving `reclaim.go` untouched. Add the module's `TestReadsNoStateFromKubernetes`. Verify `reclaim_test.go` passes unedited, the parse and dual-read test passes against a fake manager, and a failing listing exits non-zero and removes nothing.

## 10. Unit tests

- [ ] 10.1 `cd platform/manager && KUBEBUILDER_ASSETS=... go test ./...` green in the worktree, including the generic suite over every field of `Stores`, the static check with the annotation prefix and the reconcilers' event-sequence tests.
- [ ] 10.2 `cd platform/console && go test ./... && (cd ui && npm test)` and `cd platform/housekeeping && go test ./...` green, each module's `TestReadsNoStateFromKubernetes` included.
- [ ] 10.3 `cd chart && go test ./...` green for the credential, the Roles and the NetworkPolicy flow.
- [ ] 10.4 `go test -count=1 -coverpkg=./... -coverprofile=coverage.out ./...` in `platform/manager`: no package below its previous coverage.

## 11. System tests

- [ ] 11.1 Run the system pack unchanged against this worktree's images and chart, locally per `build-test.md` or dispatched with `gh workflow run system-smoke.yml --ref change/state-store-generic-contract`. Verify the console lanes pass with the console Role no longer granting `conversations` or `signals`.
- [ ] 11.2 Add one full-tier housekeeping lane in `platform/manager/test/system/`: run the job once in dry-run against the tier's install and assert retained equals the conversation count and nothing is removed. Verify it passes locally or dispatched.
- [ ] 11.3 Run `e2e-live` once against the tier's console, unchanged. Verify every spec passes.

## 12. Documentation

### Reference docs

- [ ] 12.1 `docs/contracts.md`: the state read surface section, the introspection paragraph reworded so "never proxied" is said of configuration only, and the HTTP API table row. `docs/console.md`: sources, the two feeds, the RBAC table, the values table. Verify the pages read for the console's operator.
- [ ] 12.1a `docs/contracts/channel-adapter.md` and `docs/contracts/signal-adapter.md`: the optional `spec.stateAccess` field, its default of false, and that it grants only the state read surface.
- [ ] 12.2 `docs/concepts.md`: `stateAccess` under the adapter kinds, adapter cursors under the state-store home, and the restart-resilience rows for the console and the housekeeping job naming the manager as what they read. `docs/configuration.md`: `stateReadAuth`. `docs/testing.md`: the suite is generic and driven by the inventory. `docs/CHANGELOG.md`: housekeeping needs the manager reachable and holds no RBAC, the new values key, the new CRD field.
- [ ] 12.3 `docs/security.md`: housekeeping's trust boundary moves from a Kubernetes grant to a manager credential. Re-run `threat-model.py`.
- [ ] 12.4 `.claude/rules/structure.md` (`Stores`, the capability interfaces, the generic `kube.Open`, the console and housekeeping rows) and `.claude/rules/invariants.md` (the state row names every reader). Add `SignalStore`, `GetCooldown`/`PutCooldown`, `RecordReceived`/`GetReceived` and `ReleaseSource` as contract operations to `.github/retired-vocabulary.json`, with `Stores` and the helpers as replacement. `Store` itself is not added, since the word also names the whole store in prose and the Go identifier is already guarded by the compiler and the static check.
- [ ] 12.5 Re-run `python3 .github/scripts/docs-generate.py` and `--check` for the new CRD field and chart value. Verify `--check`, `retired-vocabulary-guard.py` and `publication-guard.py` pass, and record the verdicts here.

### Adopter site

- [ ] 12.6 `docs/console.md` and `docs/console-guide.md` say the console reads state from the manager and needs no RBAC on state kinds. `docs/installation.md` if it lists the console's or housekeeping's cluster permissions. Confirm `docs/getting-started.md`, the landing page and `README.md` need no change: a default install renders the same pages. Record the verdict here.
