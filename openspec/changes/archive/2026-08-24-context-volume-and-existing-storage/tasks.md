## 1. The API: the volume leaves the runtime and lands on the wiring

- [x] 1.1 DELETE `AgentRuntime.spec.home`, `HomeVolume`, `spec.context`, `ContextVolume` and the `ContextVolume()` accessor from `platform/manager/api/v1alpha1/`. No alias survives — the concept moved to another CR, so there is nothing for one to point at. Keep `spec.contextStorage`, which is the runtime's alone. Verify: `grep -rn "HomeVolume\|Spec.Home\|spec.context\b" platform/manager --include=*.go` returns nothing.
- [x] 1.2 Add `Pipeline.spec.persistence` — `context` and `workspace`, each a binding of `claimName` XOR `volumeName` (plus `size` / `storageClassName` / `accessModes` shaping the claim the manager renders for `volumeName`). Doc comments state the precedence and that the CONVERSATION snapshots the result, following `spec.serviceAccountName`'s comment as the model. Verify: `go build ./...`.
- [x] 1.3 Add a CEL or webhook-free validation that `claimName` and `volumeName` are mutually exclusive per binding — `+kubebuilder:validation:XValidation`, so the API server refuses it rather than a reconciler reporting it later. Verify: an envtest applying both is rejected.
- [x] 1.4 Add the snapshot fields to `Conversation.spec` — the resolved context and workspace claim names — beside `runtimeRef` / `serviceAccountName`, marked MATERIALIZED and never hand-set. Verify: `go build ./...`.
- [x] 1.5 Regenerate deepcopy and CRDs. Verify: `chart/crds/agentops.dev_agentruntimes.yaml` carries neither `home` nor `context`, `..._pipelines.yaml` carries `persistence`, and `..._conversations.yaml` the snapshot fields.

## 2. Resolution, and the one place the operator creates storage

- [x] 2.1 Write the resolver: `pipeline binding -> chart release default -> ephemeral`, with the runtime in NO part of the chain. ONE function, used by both conversation creation and any display. Verify: a table test covers all six rows of the proposal's resolution table.
- [x] 2.2 Resolve at conversation CREATION and write the snapshot. Verify: an integration test edits a Pipeline's binding mid-conversation and asserts the running conversation keeps its original claim.
- [x] 2.3 Pipeline reconciler renders a PVC when a binding names `volumeName`, with `storageClassName: ""` so it binds rather than provisions. NO ownerRef on the Pipeline. Verify: an envtest deletes the Pipeline and asserts the claim survives.
- [x] 2.4 Add `persistentvolumeclaims` get/list/watch/create to the manager's Role — and NOT delete. The operator may bring storage into being and must never take it away. Verify: the chart's RBAC render test pins the absent verb.
- [x] 2.5 `runtimepod` reads the CONVERSATION's snapshot, never a Pipeline and never a runtime. Verify: `grep -rn "Pipeline" internal/runtimepod` returns nothing.

## 3. The mount path moves

- [x] 3.1 `/data/home` -> `/data/context` and `HOME=/data/context` across `internal/runtimepod/podspec.go`, `contextsync.go`, `platform/context-sync/`, `runtimes/claude/Dockerfile` and `runtime.js`. `/data/workspace` is UNTOUCHED. Verify: `grep -rn "/data/home" .` returns nothing outside CHANGELOG history.
- [x] 3.2 Confirm the stored layout is path-independent, as measured: a generation holds `.claude/projects/-data-workspace/...` relative to the mount, so nothing inside the volume moves. Verify: re-inspect a live claim read-only and record the VERDICT, never the paths of a real install.
  - **VERDICT: CONFIRMED.** The live claim was mounted READ-ONLY at a path that is neither the old nor the new one, and every stored generation resolved exactly as it does at its own mount. The transcript directory is named `-data-workspace` — the WORKING directory — and it is the only such directory in the volume. Nothing inside is named for `$HOME`, so nothing inside moves when `$HOME` does.
- [x] 3.3 Rename the pod volumes to `context` / `context-store`. Verify: `podspec_test.go` and `contextsync_test.go` assert the new mount path and pass.

## 4. The chart

- [x] 4.1 `persistence.context` / `persistence.workspace`, claim `agentops-context`, `selector` beside `volumeName`, and the three-state `storageClassName` helper — a name renders that class, `-` renders an explicit `""`, empty renders no field. Verify: a render test covers all three states on both volumes.
- [x] 4.2 `runtime.yaml` renders NO volume, and `runtime.contextPvcRef` / `homePvcRef` are deleted. Verify: the rendered `AgentRuntime` has neither key.
- [x] 4.3 `pipelines[].persistence` in values, rendering the binding onto each Pipeline; the chart renders the claim for a `volumeName` a Pipeline it ships declares. Verify: a render test binds a chart-shipped route to a pre-created volume.
- [x] 4.4 Both guards: the retired flat `persistence.*` keys FAIL the render naming where each moved (no cluster needed), and a claim under the retired name on upgrade FAILS naming the migration (cluster-only, `lookup`). Verify: a render test for the first, `helm upgrade --dry-run=server` for the second.
- [x] 4.5 Follow the resolved default claim to `deployment.yaml` (`CONTEXT_PVC`), `housekeeping.yaml`, `context-probe.yaml`. Verify: `charttemplate_test.go` passes against the new names.

## 5. The rest of the tree

- [x] 5.1 Sweep `home` where it names this volume in `platform/console/`, `platform/context-sync/` and the chart helpers. Verify: `grep -rniI "home" --include=*.go --include=*.ts --include=*.tpl . | grep -v "Home Assistant\|home-ops\|home directory"` returns nothing naming the volume.
- [x] 5.2 Confirm the exclusions held: `/data/workspace`, `state-durability`'s "one declared home", and every Home Assistant mention in `signals/ha/`, `ha-bundle` and `docs/ha-bundle.md`. Verify: `git diff --stat` shows no change under `signals/ha/` or `chart/charts/ha-bundle/`.
- [x] 5.3 Add the retired-vocabulary terms for `home volume`, `spec.home`, `homePvcRef`, `HOME_PVC`, `agentops-home` and the flat `persistence.*` keys — the rule says the entry lands in the SAME change that retires the name. This requires the specs synced first, and `docs/CHANGELOG.md` history must still pass. Verify: `python3 .github/scripts/retired-vocabulary-guard.py` clean.
- [x] 5.4 Run every module. Verify: the `.github/components.sh modules` loop builds, vets and tests clean.

## 6. Live verification

- [x] 6.1 Upgrade a throwaway release holding an `agentops-home` claim with no values change and confirm the render FAILS naming the migration.
  - **VERDICT: the guard fires.** A throwaway release was installed rendering the claim under the retired name, then upgraded with no values change against a real cluster (`--dry-run=server`). `UPGRADE FAILED` at `pvc.yaml`, naming the retired claim, the new default, and the values to set. No claim was created.
- [x] 6.2 Rebind a PV under the new claim name — retain it, clear its `claimRef`, let the chart's claim bind it by `volumeName` — and confirm no data moved and the claim is `Bound`.
  - **VERDICT: rebinding works and moves no data.** A marker file written through the retired claim was read back through the new one after the PV was retained, its `claimRef` cleared and the new claim bound to it by `volumeName`. Claim `Bound`.
  - **AND THE DOCUMENTED FORM WAS WRONG, WHICH ONLY DOING IT FOUND.** `storageClassName: "-"` FAILS this migration: a PV that was DYNAMICALLY PROVISIONED keeps its class forever, so a claim requesting `""` is refused with `VolumeMismatch: storageClassName does not match` and sits `Pending` — indistinguishable from a missing provisioner. The claim must name the PV's OWN class. `-` is for a STATICALLY created PV that has no class, which is not what an existing install has. The guard message, `values.yaml`, the CHANGELOG and `installation.md` all say this now.
  - **A second sharp edge, same path:** a claim's spec is immutable once created, so a first attempt that got the class wrong cannot be corrected by re-running `helm upgrade` — the wrong claim must be deleted first.
- [x] 6.3 A Pipeline naming a `volumeName` gets a manager-created claim; deleting that Pipeline leaves the claim standing.
  - **CLOSED WITHOUT A LIVE RUN, on the owner's decision:** the product is closed, and the live install is disposable — wiped and reinstalled at will — so a run against it is not a gate.
  - **Verified against a REAL API SERVER**, which is what exercises the reconciler: `TestNamingAVolumeRendersAClaimThatOutlivesThePipeline` (envtest) creates the claim from the binding, asserts the explicit empty storage class and the ABSENT ownerRef, deletes the Pipeline, and asserts the claim survives.
  - **What a live run would add, and only that:** a KUBELET actually mounting the claim. The reconciler, the CRD validation and the pod spec are all exercised above.
- [x] 6.4 Two Pipelines on ONE runtime with DIFFERENT context volumes both run, each conversation on its own claim. This is the requirement the whole move exists for.
  - **CLOSED on the same decision.** Verified against a real API server by `TestTwoPipelinesOneRuntimeTwoVolumes` (envtest), which drives two signals through the real ingest path and asserts BOTH built pods carry the one runtime's image and DIFFERENT claims.
- [x] 6.5 A conversation created before a Pipeline's binding is edited keeps its original claim.
  - **CLOSED on the same decision.** Verified against a real API server by `TestEditingAPipelineDoesNotMoveARunningConversationsVolume` (envtest): the snapshot is asserted on the stored Conversation, the Pipeline is then re-wired, and the next built pod still mounts the claim the conversation was created against.

## 7. Documentation

**Reference docs**

- [x] 7.1 `docs/CHANGELOG.md`, newest first, and FIRST of the three — for a GitOps install it is the only warning that arrives. Lead with the runtime field being DELETED rather than aliased and where the declaration moves to, then the claim rename, the mount path, and the retired values keys.
- [x] 7.2 `docs/concepts.md` — persistence as wiring, the resolution table, the three forms of pointing at storage the chart did not create, and the mount path. Verify: no occurrence of "home volume" remains.
- [x] 7.3 `docs/k8s-bundle.md`, `docs/console.md` — incidental mentions. Verify: `grep -rn "home volume\|homePvcRef\|HOME_PVC" docs/` returns only CHANGELOG history.
- [x] 7.4 Re-run `python3 .github/scripts/docs-generate.py` — three CRDs and the chart values changed. Verify: the script exits clean.

**Adopter site**

- [x] 7.5 `docs/installation.md` — the renamed keys, and the decision this change creates: pointing a volume at storage the chart did not create, in all three forms, at BOTH levels.
- [x] 7.6 `docs/guides/pipeline.md` — a route declaring its own storage, which is the new adopter-facing capability. `docs/guides/agent-runtime.md` loses the volume.
- [x] 7.7 `docs/getting-started.md` — the RWX troubleshooting row names `agentops-context`, plus the failed-static-bind case beside it.

**Context rules**

- [x] 7.8 `.claude/rules/terminology.md` — the context volume joins the banned-word table, stating that `/data/workspace` is the load-bearing path and `/data/home` is GONE.
- [x] 7.9 `.claude/rules/wiring.md` — persistence joins the wiring table beside tools, channels, runtime and identity. `.claude/rules/invariants.md` — the substrate statements, and the manager's new PVC-create power with its no-ownerRef rule.
