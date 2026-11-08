## 1. Sequencing gate

This gate applies when implementation starts. This pull request only proposes.

- [x] 1.1 Confirm `state-store-generic-contract` is archived and rebase the branch on master. Nothing below starts before this is ticked.
  Verdict:
  - `state-store-generic-contract` is archived at `openspec/changes/archive/2026-11-08-state-store-generic-contract/` (issue #457, PR #470).
  - `change/nats-state-store-default` was branched from `origin/master` at `51d7304`, which already contains the archive commit.
- [x] 1.2 Re-read the published "Every piece of live state has one declared home" in `state-durability`. This change's delta replaces one sentence of it. Copy the published block whole, including every scenario the other change added, and change only that sentence.
  Verdict:
  - `openspec/changes/nats-state-store-default/specs/state-durability/spec.md`'s `MODIFIED Requirements` block already equals the now-published `state-durability` requirement apart from the one paragraph naming the backend (`nats` on its own volumes vs. `kube`'s), diffed by hand against `openspec/specs/state-durability/spec.md`.
  - No scenario of the published requirement is missing from the delta.
- [x] 1.3 Re-run `openspec validate nats-state-store-default`.
  Verdict: `openspec validate nats-state-store-default` reports the change valid.

## 2. The `nats` backend — backend-developer

- [ ] 2.1 Add `internal/state/nats` with the connection, the bucket bootstrap (replica count from `NATS_REPLICAS`) and the envelope codec (D-1, D-3). Verify `go vet ./internal/state/...` passes and only this package imports `nats.go`.
- [ ] 2.2 Implement `Collection[T]` over KV: `Get`, `Create`, `Update`, `Delete`, `DeleteAt` with the revision as CAS, and the contract's errors (D-2). Verify a stale write returns the conflict error, a create over a held key returns already-exists, and a delete of a record holding a finalizer leaves it readable with its deletion time set until the `Update` that empties the finalizers removes it.
- [ ] 2.3 Implement the watch-fed mirror, `List` by declared index, and `Watch` resuming strictly after a revision with a bookmark (D-2, D-4). Verify `List` blocks until the first catch-up and a list after a returned create includes it.
- [ ] 2.4 Implement `Expiring[T]`, the sweep and invisible-past-expiry reads (D-6). Verify a read between expiry and sweep reports not found, a list omits the expired record, and the feed carries its delete.
- [ ] 2.5 Implement `Appended[P, T]`: `Append`, `Grow` by CAS retry, `GetWithItems`, `ListWithItems`, `WatchWithItems` (D-5). Verify two concurrent grows both apply.
- [ ] 2.6 Implement `Blobs` over an Object Store (D-7). Verify a blob above the server's message limit reads back byte-identical.
- [ ] 2.7 Implement the one-time import from `kube` with the marker written last (D-10). Verify every import scenario of the spec: the upgrade, the interrupted import, the non-leader, the late old-replica write, the new-version write that is never overwritten, the fresh install and the untouched source.
- [ ] 2.8 Select `nats` in `cmd/manager/store.go` from `STATE_BACKEND`, read `NATS_URL`, credentials and replicas from env, and report not ready while the server is unreachable. Verify the unknown-backend and not-ready tests.

## 3. The `nats` bundle and the default — deployment-engineer

- [x] 3.1 Add `chart/charts/nats/`: StatefulSet running `nats-server -js`, headless and client Services, PVC, probes, digest-pinned multi-arch image, `nats.replicas` 3 by default or 1 as an explicit choice, cluster routes, a PodDisruptionBudget and soft anti-affinity (D-8). Verify `helm template` renders for one and three replicas and the one-replica render prints the single-point-of-failure note.
  Verdict:
  - `chart/charts/nats/` (new): `Chart.yaml`, `values.yaml` (`replicas: 3`, digest-pinned `nats:2.10-alpine`, auth block, persistence, probes, PDB), `templates/_helpers.tpl` (named templates `nats.statefulset`, `nats.services`, `nats.pdb`, `nats.credential`, `nats.networkpolicy`, `nats.routes`, `nats.credentialSecretName`). The digest `sha256:b83efabe…` was confirmed carrying both `linux/amd64` and `linux/arm64` with `docker buildx imagetools inspect nats:2.10-alpine`.
  - **Judgment call, flagged.** These objects render from a NEW parent file, `chart/templates/nats.yaml`, not natively from `chart/charts/nats/templates/`. A throwaway two-chart fixture proved a subchart's own template context cannot see a parent-scope value, not even through `$`, only `global.*`. The selector D-9 needs the bundle to follow, `stateStore.backend`, is not global. Making it global would need a wider rename this task did not ask for, and it would then contradict design.md's own migration text ("set `stateStore.backend: kube`"). So `chart/charts/nats/templates/_helpers.tpl` defines everything as named templates only, zero bare YAML, so Helm's unconditional native parse of a condition-less dependency emits nothing on its own. `chart/templates/nats.yaml` is the one call site that can see both `stateStore.backend` and `.Values.nats.*`, and it gates every object together. The `kubernetes` bundle's own "no condition, self-gate" precedent already covers logic no condition expression can state. This is the same escape hatch, one level further in, because even the self-gate cannot live in the subchart here.
  - The nats `Chart.yaml` dependency carries NO `condition:`, matching `kubernetes`'s precedent, so its named templates are always parsed and callable.
  - Cluster routes: `--routes` lists every replica's own headless-Service address, including the caller's own (nats-server ignores a self-route), built by `nats.routes`.
  - PDB: `maxUnavailable: 1`, rendered unconditionally, including at `replicas: 1`, where it is a no-op rather than a second place the replica count could drift.
  - Anti-affinity: `preferredDuringSchedulingIgnoredDuringExecution`, weight 100, `topologyKey: kubernetes.io/hostname`.
  - NOTES.txt: added a block to the PARENT's `chart/templates/NOTES.txt`, gated on `agentops.stateBackend == "nats"` and `nats.replicas == 1`, printing the single-point-of-failure warning.
  - Verified: `helm template chart --set claude.credentialsSecret.token=x --set nats.replicas=1` and the same with `--set nats.replicas=3` (default) both render cleanly (`helm template` exit 0, `helm lint` 0 failures), and the StatefulSet/PDB/Services/Secret all parse as independent, valid YAML documents (`python3 -c "yaml.safe_load_all(...)"`). The 1-replica `--routes` carries exactly one URL, the 3-replica one carries three.
  - **NOT performed:** reading the NOTES.txt warning's rendered text. `helm template` never prints `NOTES.txt` — confirmed by injecting a deliberate template error into it and seeing `helm template` fail naming a `NOTES.txt` line, proving it IS executed, just never printed — and `helm install --dry-run` (the only command that prints it) refuses with "Kubernetes cluster unreachable" in this sandbox, matching this repo's own `helmNotes` test helper's documented, pre-existing limitation. The template logic was read by eye and matches the sibling `persistence.context` single-attach warning's shape.

- [x] 3.2 Add the chart-generated credential following `call-auth.yaml`'s install-only rule, and the manager's `valueFrom` env in `deployment.yaml`. Verify the upgrade-without-cluster render mints nothing.
  Verdict:
  - **Judgment call, flagged.** This repository has no file named `call-auth.yaml` at the path the dispatch brief named (`chart/templates/call-auth.yaml`). The actual rule lives in `adapter-auth.yaml`/`control-plane-auth.yaml`/`state-read-auth.yaml` (parent) and in `chart/charts/telegram/templates/call-auth.yaml` (a differently-scoped per-surface binding secret). Followed `control-plane-auth.yaml`'s shape, the closest precedent, reusing `agentops.generatedSecretGuard`/`agentops.clusterConnected` verbatim for each of the two fields this credential carries (`user`/`password`) rather than re-deriving the gate.
  - `nats.credential` (named template, `chart/charts/nats/templates/_helpers.tpl`) resolves each of `user` and `password` in the same order: an explicit `nats.auth.*` value, then `nats.auth.existingSecret`, then generation on install or a connected upgrade only. Only `password` is ever generated (`randAlphaNum 32`). `user` defaults to the literal `nats` when nothing else supplies one, so a render that never touches `nats.auth` still needs no second Secret key to exist by convention.
  - `deployment.yaml`: `NATS_URL` is a plain `value`, not a credential (the client Service's DNS name). `NATS_USER`/`NATS_PASSWORD` are `valueFrom.secretKeyRef`. `NATS_REPLICAS` is a plain value from `.Values.nats.replicas`. The manager reads no Secret itself (invariants.md).
  - The StatefulSet's own container never carries the password as a literal. `--user $(POD_NATS_USER) --pass $(POD_NATS_PASSWORD)` relies on Kubernetes' own `$(VAR)` expansion of the container's OWN env, a kubelet mechanism, fed by `secretKeyRef`.
  - Verified: `helm template chart --set claude.credentialsSecret.token=x --is-upgrade` (no cluster) renders NO `kind: Secret` named `agentops-nats-credentials`, checked by parsing every document and listing Secret names. An explicit `nats.auth.user=alice,nats.auth.password=secret123` render base64-decodes to exactly those values. `nats.auth.existingSecret=my-own-nats-secret` renders no generated Secret, and every reference (`deployment.yaml`'s env, the StatefulSet's own env) points at the named Secret instead.

- [x] 3.3 Add the NetworkPolicy: client port from the manager only, peer port from its own pods, and the flow-table entry. Verify `TestEveryWiredFlowIsAllowed` lists it, a runtime pod is refused, the console, a channel adapter, a signal adapter and a housekeeping job are each refused on the client port, and a pod that is not a state server is refused on the peer port (D-12).
  Verdict:
  - `nats.networkpolicy` (named template): the client port (4222) admits `app.kubernetes.io/name: agentops-manager` only. The peer/cluster port (6222) admits `app.kubernetes.io/name: agentops-nats` only, its own pods. The monitor port (8222) has no `from` at all, matching the manager's own policy's documented reason — the kubelet's probe originates from the node, which no `podSelector` can name, and the port serves only `/healthz`.
  - `platform/manager/internal/integration/charttemplate_networkpolicy_test.go`, the ONE manager file this lane may touch: extended `TestEveryWiredFlowIsAllowed` with the manager→`agentops-nats` flow (no extra `--set` needed, `nats` is the default backend), and added `TestStateServerAdmitsOnlyTheManager` asserting the refusals. A runtime pod, an adapter, a signal adapter, a housekeeping job and the console's adapter-style label are each absent from the policy text entirely — the client-port rule names the manager alone and the peer-port rule names the server's own label alone, so their absence from the whole policy proves both refusals at once.
  - Verified: `go test ./internal/integration/... -run 'TestEveryWiredFlowIsAllowed|TestStateServerAdmitsOnlyTheManager|TestNetworkPolicyIsOptIn'` — 3/3 pass (`KUBEBUILDER_ASSETS=/root/.envtest/k8s/1.31.0-linux-amd64`). A full, untargeted `go test ./internal/integration/...` run (the manager's whole envtest and chart-template suite, 99s) also passes except the two tests named in 3.5's verdict, unrelated to this task.

- [x] 3.4 Add `nats` to `agentops.stateStoreBackends`, derive the bundle's `enabled` from the selector, and extend `stateStoreGuard` with the contradictions of the spec (D-9). Verify each guard scenario renders or fails as stated.
  Verdict:
  - `agentops.stateStoreBackends` now returns `["nats"]`.
  - **Judgment call, flagged**, same root cause as 3.1's. "Derive the bundle's `enabled` from the selector" cannot mean a literal default on `nats.enabled` tracking `stateStore.backend` — no Helm values mechanism recomputes one static value from another on a plain `--set` override. A YAML-anchor draft was tried and rejected for the same reason: `--set` cannot retroactively move a value across an anchor already expanded at parse time. Implemented instead as: `chart/charts/nats/values.yaml` ships NO default at all for `enabled` (the key is documented, commented out). `agentops.stateStoreGuard` uses `hasKey` to tell "hand-set" apart from "left alone." Left alone, the selector decides with no check. Hand-set, the guard fails any mismatch against what the selector needs. This satisfies every scenario in `specs/state-store/spec.md` literally, including "Pinning kube needs one value" (no second value touched, no failure) and "A bundle left on under another backend fails" (hand-set `true` while `kube` is selected fails, naming `nats.enabled`).
  - Rewrote `agentops.stateStoreGuard` generically, so it works the same way for any future second backend rather than special-casing `nats`: the unknown-backend check is unchanged, then, per known backend, a `hasKey`-gated mismatch check only.
  - Verified (`helm template chart --set claude.credentialsSecret.token=x ...`): `stateStore.backend=kube` alone renders 0 nats objects and no failure. `stateStore.backend=nope` fails naming it. `stateStore.backend=kube,nats.enabled=true` fails naming the conflict. `nats.enabled=true` alone, agreeing with the default `nats` selection, renders cleanly. `nats.enabled=false` alone, disagreeing with the default `nats` selection, fails naming the conflict.

- [x] 3.5 Make `nats` the default: set `stateStore.backend: nats` in `chart/values.yaml` and the helper default. Verify the default render includes the bundle and `backend: kube` renders none.
  Verdict:
  - `chart/values.yaml`'s `stateStore.backend` is now `nats`. `agentops.stateBackend`'s own internal fallback, used when `stateStore` is wholly absent (e.g. `--set stateStore=null`), is now `nats` too.
  - `chart/Chart.yaml` gained the `nats` dependency entry (no `condition:`, see 3.1).
  - Verified: the default `helm template` carries `STATE_BACKEND` = `"nats"` and renders the StatefulSet/Services/PDB/Secret/(gated)NetworkPolicy. `--set stateStore.backend=kube` carries `STATE_BACKEND` = `"kube"`, renders none of them, and omits `NATS_URL`/`NATS_USER`/`NATS_PASSWORD`/`NATS_REPLICAS` from the manager Deployment entirely.
  - **Known, flagged breakage outside this lane.** `platform/manager/internal/integration/charttemplate_test.go` carries two pre-existing tests pinned to the OLD default: `TestStateBackendEnvIsRenderedFromTheSelector` ("the default is kube") and `TestStateStoreNullFallsBackToTheKubeDefault`. Both now fail, a direct, unavoidable consequence of this task's own instruction to flip the default. Confirmed by running the full `internal/integration` suite: exactly these two tests fail and nothing else does — not `TestStateStoreGuard`, which exercises the same guard without assuming a default. This file is outside this dispatch's lane, since only the NetworkPolicy test file was authorized. These two assertions need updating to expect `nats` by whoever owns that file next (task 4, unit tests, or the orchestrating session).

- [ ] 3.6 Record the three gates of D-11 as this change's acceptance: the conformance verdict, the bench numbers against `kube` and the pack verdict. Verify all three pass. A failure is fixed in the backend, never answered by restoring the old default.
  Verdict: PENDING on the backend-developer lane (`internal/state/nats/`, tasks 2.1–2.8) and the system lane (task 5), neither of which had conformance/bench/pack output available at the time this chart lane ran. Left UNTICKED rather than fabricating a number — the orchestrating session completes this once both lanes report.

## 4. Unit tests

- [ ] 4.1 `go test ./...` in `platform/manager` under `KUBEBUILDER_ASSETS` and `TZ=Europe/Helsinki`, including `RunStores` against the in-process server and the static import check.
- [ ] 4.2 `go test -tags conformance -count=1 ./test/conformance/` and `go test -run '^$' -bench . -benchmem ./internal/state/...` for both backends.
- [ ] 4.3 Add a test for the import against an old-version writer still active on `kube`: a Signal created and a Conversation input appended after the first pass must be present after the final one, and a record the new version wrote must not be overwritten (D-10).
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
