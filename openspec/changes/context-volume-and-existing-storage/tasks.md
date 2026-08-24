## 1. The API: the volume leaves the runtime and lands on the wiring

- [ ] 1.1 DELETE `AgentRuntime.spec.home`, `HomeVolume`, `spec.context`, `ContextVolume` and the `ContextVolume()` accessor from `platform/manager/api/v1alpha1/`. No alias survives — the concept moved to another CR, so there is nothing for one to point at. Keep `spec.contextStorage`, which is the runtime's alone. Verify: `grep -rn "HomeVolume\|Spec.Home\|spec.context\b" platform/manager --include=*.go` returns nothing.
- [ ] 1.2 Add `Pipeline.spec.persistence` — `context` and `workspace`, each a binding of `claimName` XOR `volumeName` (plus `size` / `storageClassName` / `accessModes` shaping the claim the manager renders for `volumeName`). Doc comments state the precedence and that the CONVERSATION snapshots the result, following `spec.serviceAccountName`'s comment as the model. Verify: `go build ./...`.
- [ ] 1.3 Add a CEL or webhook-free validation that `claimName` and `volumeName` are mutually exclusive per binding — `+kubebuilder:validation:XValidation`, so the API server refuses it rather than a reconciler reporting it later. Verify: an envtest applying both is rejected.
- [ ] 1.4 Add the snapshot fields to `Conversation.spec` — the resolved context and workspace claim names — beside `runtimeRef` / `serviceAccountName`, marked MATERIALIZED and never hand-set. Verify: `go build ./...`.
- [ ] 1.5 Regenerate deepcopy and CRDs. Verify: `chart/crds/agentops.dev_agentruntimes.yaml` carries neither `home` nor `context`, `..._pipelines.yaml` carries `persistence`, and `..._conversations.yaml` the snapshot fields.

## 2. Resolution, and the one place the operator creates storage

- [ ] 2.1 Write the resolver: `pipeline binding -> chart release default -> ephemeral`, with the runtime in NO part of the chain. ONE function, used by both conversation creation and any display. Verify: a table test covers all six rows of the proposal's resolution table.
- [ ] 2.2 Resolve at conversation CREATION and write the snapshot. Verify: an integration test edits a Pipeline's binding mid-conversation and asserts the running conversation keeps its original claim.
- [ ] 2.3 Pipeline reconciler renders a PVC when a binding names `volumeName`, with `storageClassName: ""` so it binds rather than provisions. NO ownerRef on the Pipeline. Verify: an envtest deletes the Pipeline and asserts the claim survives.
- [ ] 2.4 Add `persistentvolumeclaims` get/list/watch/create to the manager's Role — and NOT delete. The operator may bring storage into being and must never take it away. Verify: the chart's RBAC render test pins the absent verb.
- [ ] 2.5 `runtimepod` reads the CONVERSATION's snapshot, never a Pipeline and never a runtime. Verify: `grep -rn "Pipeline" internal/runtimepod` returns nothing.

## 3. The mount path moves

- [ ] 3.1 `/data/home` -> `/data/context` and `HOME=/data/context` across `internal/runtimepod/podspec.go`, `contextsync.go`, `platform/context-sync/`, `runtimes/claude/Dockerfile` and `runtime.js`. `/data/workspace` is UNTOUCHED. Verify: `grep -rn "/data/home" .` returns nothing outside CHANGELOG history.
- [ ] 3.2 Confirm the stored layout is path-independent, as measured: a generation holds `.claude/projects/-data-workspace/...` relative to the mount, so nothing inside the volume moves. Verify: re-inspect a live claim read-only and record the VERDICT, never the paths of a real install.
- [ ] 3.3 Rename the pod volumes to `context` / `context-store`. Verify: `podspec_test.go` and `contextsync_test.go` assert the new mount path and pass.

## 4. The chart

- [ ] 4.1 `persistence.context` / `persistence.workspace`, claim `agentops-context`, `selector` beside `volumeName`, and the three-state `storageClassName` helper — a name renders that class, `-` renders an explicit `""`, empty renders no field. Verify: a render test covers all three states on both volumes.
- [ ] 4.2 `runtime.yaml` renders NO volume, and `runtime.contextPvcRef` / `homePvcRef` are deleted. Verify: the rendered `AgentRuntime` has neither key.
- [ ] 4.3 `pipelines[].persistence` in values, rendering the binding onto each Pipeline; the chart renders the claim for a `volumeName` a Pipeline it ships declares. Verify: a render test binds a chart-shipped route to a pre-created volume.
- [ ] 4.4 Both guards: the retired flat `persistence.*` keys FAIL the render naming where each moved (no cluster needed), and a claim under the retired name on upgrade FAILS naming the migration (cluster-only, `lookup`). Verify: a render test for the first, `helm upgrade --dry-run=server` for the second.
- [ ] 4.5 Follow the resolved default claim to `deployment.yaml` (`CONTEXT_PVC`), `housekeeping.yaml`, `context-probe.yaml`. Verify: `charttemplate_test.go` passes against the new names.

## 5. The rest of the tree

- [ ] 5.1 Sweep `home` where it names this volume in `platform/console/`, `platform/context-sync/` and the chart helpers. Verify: `grep -rniI "home" --include=*.go --include=*.ts --include=*.tpl . | grep -v "Home Assistant\|home-ops\|home directory"` returns nothing naming the volume.
- [ ] 5.2 Confirm the exclusions held: `/data/workspace`, `state-durability`'s "one declared home", and every Home Assistant mention in `signals/ha/`, `ha-bundle` and `docs/ha-bundle.md`. Verify: `git diff --stat` shows no change under `signals/ha/` or `chart/charts/ha-bundle/`.
- [ ] 5.3 Add the retired-vocabulary terms for `home volume`, `spec.home`, `homePvcRef`, `HOME_PVC`, `agentops-home` and the flat `persistence.*` keys — the rule says the entry lands in the SAME change that retires the name. This requires the specs synced first, and `docs/CHANGELOG.md` history must still pass. Verify: `python3 .github/scripts/retired-vocabulary-guard.py` clean.
- [ ] 5.4 Run every module. Verify: the `.github/components.sh modules` loop builds, vets and tests clean.

## 6. Live verification

- [ ] 6.1 Upgrade a throwaway release holding an `agentops-home` claim with no values change and confirm the render FAILS naming the migration.
- [ ] 6.2 Rebind a PV under the new claim name — retain it, clear its `claimRef`, let the chart's claim bind it by `volumeName` — and confirm no data moved and the claim is `Bound`.
- [ ] 6.3 A Pipeline naming a `volumeName` gets a manager-created claim; deleting that Pipeline leaves the claim standing.
- [ ] 6.4 Two Pipelines on ONE runtime with DIFFERENT context volumes both run, each conversation on its own claim. This is the requirement the whole move exists for.
- [ ] 6.5 A conversation created before a Pipeline's binding is edited keeps its original claim.

## 7. Documentation

**Reference docs**

- [ ] 7.1 `docs/CHANGELOG.md`, newest first, and FIRST of the three — for a GitOps install it is the only warning that arrives. Lead with the runtime field being DELETED rather than aliased and where the declaration moves to, then the claim rename, the mount path, and the retired values keys.
- [ ] 7.2 `docs/concepts.md` — persistence as wiring, the resolution table, the three forms of pointing at storage the chart did not create, and the mount path. Verify: no occurrence of "home volume" remains.
- [ ] 7.3 `docs/k8s-bundle.md`, `docs/console.md` — incidental mentions. Verify: `grep -rn "home volume\|homePvcRef\|HOME_PVC" docs/` returns only CHANGELOG history.
- [ ] 7.4 Re-run `python3 .github/scripts/docs-generate.py` — three CRDs and the chart values changed. Verify: the script exits clean.

**Adopter site**

- [ ] 7.5 `docs/installation.md` — the renamed keys, and the decision this change creates: pointing a volume at storage the chart did not create, in all three forms, at BOTH levels.
- [ ] 7.6 `docs/guides/pipeline.md` — a route declaring its own storage, which is the new adopter-facing capability. `docs/guides/agent-runtime.md` loses the volume.
- [ ] 7.7 `docs/getting-started.md` — the RWX troubleshooting row names `agentops-context`, plus the failed-static-bind case beside it.

**Context rules**

- [ ] 7.8 `.claude/rules/terminology.md` — the context volume joins the banned-word table, stating that `/data/workspace` is the load-bearing path and `/data/home` is GONE.
- [ ] 7.9 `.claude/rules/wiring.md` — persistence joins the wiring table beside tools, channels, runtime and identity. `.claude/rules/invariants.md` — the substrate statements, and the manager's new PVC-create power with its no-ownerRef rule.
