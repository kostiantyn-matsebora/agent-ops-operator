## 1. Sequencing gate

- [ ] 1.1 Confirm `state-store-generic-contract` is archived and the branch is rebased on master. Add a MODIFIED `state-durability` delta for "Every piece of live state has one declared home", replacing the "default install keeps the same two homes" sentence, and re-run `openspec validate nats-state-store-default`. Nothing below starts before this is ticked.

## 2. The `nats` backend — backend-developer

- [ ] 2.1 Add `internal/state/nats` with the connection, the bucket bootstrap (replica count from `NATS_REPLICAS`) and the envelope codec (D-1, D-3). Verify `go vet ./internal/state/...` passes and only this package imports `nats.go`.
- [ ] 2.2 Implement `Collection[T]` over KV: `Get`, `Create`, `Update`, `Delete`, `DeleteAt` with the revision as CAS, and the contract's errors (D-2). Verify a stale write returns the conflict error and a create over a held key returns already-exists.
- [ ] 2.3 Implement the watch-fed mirror, `List` by declared index, and `Watch` resuming strictly after a revision with a bookmark (D-2, D-4). Verify `List` blocks until the first catch-up and a list after a returned create includes it.
- [ ] 2.4 Implement `Expiring[T]`, the sweep and invisible-past-expiry reads (D-6). Verify a read between expiry and sweep reports not found.
- [ ] 2.5 Implement `Appended[P, T]`: `Append`, `Grow` by CAS retry, `GetWithItems`, `ListWithItems`, `WatchWithItems` (D-5). Verify two concurrent grows both apply.
- [ ] 2.6 Implement `Blobs` over an Object Store (D-7). Verify a blob above the server's message limit reads back byte-identical.
- [ ] 2.7 Implement the one-time import from `kube` with the marker written last (D-10). Verify the three import scenarios of the spec, including a kill mid-import, and that the source objects are untouched.
- [ ] 2.8 Select `nats` in `cmd/manager/store.go` from `STATE_BACKEND`, read `NATS_URL`, credentials and replicas from env, and report not ready while the server is unreachable. Verify the unknown-backend and not-ready tests.

## 3. The `nats` bundle and the default — deployment-engineer

- [ ] 3.1 Add `chart/charts/nats/`: StatefulSet running `nats-server -js`, headless and client Services, PVC, probes, digest-pinned multi-arch image, `nats.replicas` 3 by default or 1 as an explicit choice, cluster routes, a PodDisruptionBudget and soft anti-affinity (D-8). Verify `helm template` renders for one and three replicas and the one-replica render prints the single-point-of-failure note.
- [ ] 3.2 Add the chart-generated credential following `call-auth.yaml`'s install-only rule, and the manager's `valueFrom` env in `deployment.yaml`. Verify the upgrade-without-cluster render mints nothing.
- [ ] 3.3 Add the NetworkPolicy: client port from the manager only, peer port from its own pods, and the flow-table entry. Verify `TestEveryWiredFlowIsAllowed` lists it and a runtime pod is refused.
- [ ] 3.4 Add `nats` to `agentops.stateStoreBackends`, derive the bundle's `enabled` from the selector, and extend `stateStoreGuard` with the contradictions of the spec (D-9). Verify each guard scenario renders or fails as stated.
- [ ] 3.5 Make `nats` the default: set `stateStore.backend: nats` in `chart/values.yaml` and the helper default. Verify the default render includes the bundle and `backend: kube` renders none.
- [ ] 3.6 Record the three gates of D-11 as this change's acceptance: the conformance verdict, the bench numbers against `kube` and the pack verdict. Verify all three pass. A failure is fixed in the backend, never answered by restoring the old default.

## 4. Unit tests

- [ ] 4.1 `go test ./...` in `platform/manager` under `KUBEBUILDER_ASSETS` and `TZ=Europe/Helsinki`, including `RunStores` against the in-process server and the static import check.
- [ ] 4.2 `go test -tags conformance -count=1 ./test/conformance/` and `go test -run '^$' -bench . -benchmem ./internal/state/...` for both backends.
- [ ] 4.3 Add a test for the import against a writer still active on `kube`, and decide the guard D-10 and the risk table name. Record the decision.
- [ ] 4.4 The chart's render tests for the bundle, the guard, the credential and the policy, with helm present.
- [ ] 4.5 The retired-vocabulary and publication guards pass.

## 5. System tests

Run every item against THIS WORKING COPY's chart (`--state-values-set chartPath=…`), never master's. A remote session records the cluster runs as not performed and dispatches `system-smoke.yml` and `system-full.yml` with `--ref change/<name>`.

- [ ] 5.1 Run the whole existing pack unchanged against a default install, now `nats` with three servers. Record the verdict per lane. A lane that needs an edit is a finding against the backend, not the test.
- [ ] 5.2 Add `platform/manager/test/system/state_nats_test.go` with a lane that deletes one state server pod mid-conversation and verifies the conversation completes, no input is lost and the manager stays ready.
- [ ] 5.3 Add a lane that stops two of three servers, verifies work is held and `/state/*` reports the outage, restores them, and verifies every record written before is readable and held work resumes.
- [ ] 5.4 Add a lane that drains a node holding two servers and verifies the PodDisruptionBudget holds the second eviction and writes keep succeeding. On a one-node cluster the lane asserts the budget object and records the drain as not applicable.
- [ ] 5.5 Add a lane that installs on `kube`, opens a conversation, a signal and a cooldown window, upgrades to the default, and verifies all three through `/state/*`, the idempotent re-import on a manager restart, and the custom resources byte-identical.
- [ ] 5.6 Add a lane for a rolling server upgrade (image change) that keeps two servers up and verifies no write fails.
- [ ] 5.7 Add the `kube` pin to CI: a smoke run with `stateStore.backend=kube` in `system.yml`, so the fallback is exercised and not only rendered. Verify the run passes and renders no state server.
- [ ] 5.8 Add the in-process bench and the system numbers for resident memory and write latency on `nats` to the task's verdict, beside the D-11 gate.

- [ ] 5.9 Add an `e2e-live` spec that creates a conversation through the console, deletes one state server pod, and verifies the page keeps rendering, the thread and its unread state survive and a new message still arrives after the server returns.
- [ ] 5.10 Add an `e2e-live` spec that runs after the `kube` to `nats` upgrade lane and verifies the pre-upgrade conversation, its runs and its unread state render. Seed it through `kubectl` and the manager's HTTP surface like the existing specs, never a second way to arrange state.
- [ ] 5.11 Run the existing 21 specs against the default install unchanged and record the verdict. Run them against THIS WORKING COPY's install with `E2E_LIVE_*` pointed at it. Include `npm run screenshots` only if a console view changed, and state in the task that none did.
- [ ] 5.12 Verify `system-full.yml` runs the new specs in its existing browser pass, with no new job.

## 6. Documentation

### 6.1 Reference docs

- [ ] 6.1.1 `docs/concepts.md` and `docs/contracts.md`: where state lives and what backs `/state/*`.
- [ ] 6.1.2 `docs/configuration.md`: `stateStore.backend`, `nats.*`. Add the bundle page and `docs/security.md` (the new trust boundary and control).
- [ ] 6.1.3 `docs/testing.md`: the nats tier. `docs/CHANGELOG.md`: the breaking default, the pin and the rollback loss.
- [ ] 6.1.4 `.claude/rules/structure.md`, `invariants.md` (state homes) and `gotchas.md`.
- [ ] 6.1.5 Re-run `python3 .github/scripts/docs-generate.py` for the changed chart values.

### 6.2 Adopter site

- [ ] 6.2.1 `docs/installation.md`: the new workload, the volume, uninstall residue and how to delete old CRs after an upgrade.
- [ ] 6.2.2 `docs/getting-started.md`, `docs/introduction.md` and the landing page: remove "state lives in custom resources".
- [ ] 6.2.3 `README.md`: the component list, within the 215-line budget.
