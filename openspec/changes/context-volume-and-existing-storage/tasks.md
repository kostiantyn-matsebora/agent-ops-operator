## 1. The API field, and its dual-read

- [ ] 1.1 Add `AgentRuntime.spec.context` in `platform/manager/api/v1alpha1/`, mirroring the existing `home` shape (`pvcRef` / `emptyDir`), and mark `spec.home` deprecated in its doc comment naming the one-release window. Verify: `go build ./...` in `platform/manager`.
- [ ] 1.2 Add the ONE read point — an accessor on the type, `context` winning, `home` honoured — following `Conversation.ContextID()`. Nothing else in the tree may read `spec.home`. Verify: `grep -rn "Spec.Home" platform/manager --include=*.go` returns only the accessor.
- [ ] 1.3 Add its table test beside `conversation_context_test.go`'s shape: new wins, old alone is honoured, both present prefers new. Verify: `go test ./api/...` passes.
- [ ] 1.4 Regenerate deepcopy and CRDs (`controller-gen object` + `crd`, per `.claude/rules/build-test.md`). Verify: `chart/files/crds/agentops.dev_agentruntimes.yaml` carries `context` and a deprecation-marked `home`, and `git diff` shows no unrelated CRD churn.

## 2. The manager

- [ ] 2.1 Rename the volume through `internal/runtimepod/podspec.go` and `contextsync.go` — the pod volume name, the builder's fields, `CONTEXT_LIVE_DIR`'s neighbour — leaving `/data/home` and `HOME=/data/home` UNCHANGED. Verify: `podspec_test.go`'s mount assertion still names `/data/home` and passes.
- [ ] 2.2 Dual-read the bootstrap env in `cmd/manager/main.go`: `CONTEXT_PVC` preferred, `HOME_PVC` honoured for one release. Verify: a unit test asserts both spellings resolve and that the new one wins.
- [ ] 2.3 Update `runtimepod` fixtures pinning `home` as an identifier, keeping every assertion on the PATH intact. Verify: `go test ./...` in `platform/manager` (with `KUBEBUILDER_ASSETS`).

## 3. The chart values

- [ ] 3.1 Restructure `chart/values.yaml`: the top-level `persistence` keys move under `persistence.context`, default claim name `agentops-context`, with `persistence.workspace` unchanged in shape. Verify: `helm template chart/` renders with no values supplied.
- [ ] 3.2 Document `storageClassName` with the three-state convention, on BOTH volumes — PORT the four comment lines from prometheus-community's values (quoted in design.md) rather than rewording them, so a reader recognises the convention. Verify: both blocks carry the identical comment and the shipped default stays `""`.
- [ ] 3.3 Add `selector` to BOTH volumes beside the existing `volumeName`, commented out by default as those charts ship it. Verify: values.yaml carries the identical pair under `persistence.context` and `persistence.workspace`.
- [ ] 3.4 Rename `runtime.homePvcRef` to `runtime.contextPvcRef`, honouring the old key for one release. Verify: both spellings render the same `AgentRuntime`.

## 4. The chart templates

- [ ] 4.1 Add a storage-class helper in `_helpers.tpl` implementing the convention once for both claims — a name renders that class, `-` renders an explicit `storageClassName: ""`, empty or unset renders no field. Verify: a render test covers all three states and asserts `-` yields the explicit empty string rather than an omitted field.
- [ ] 4.2 `pvc.yaml`: emit `selector` beside the existing `volumeName`, on both claims, through that helper. Verify: a render test binds by name and by selector.
- [ ] 4.3 Add the rename guard to `_helpers.tpl`, modelled on `agentops.generatedSecretGuard`: on upgrade, a claim under the retired name present and neither `name` nor `existingClaim` set explicitly FAILS the render printing the `existingClaim` line. Verify: `helm upgrade --dry-run=server` against a namespace holding `agentops-home` fails with that message, and passes once the value is set — `helm template` cannot pin this, per `.claude/rules/gotchas.md`.
- [ ] 4.4 Follow the resolved claim name through every consumer for BOTH volumes — `runtime.yaml`, `deployment.yaml` (the bootstrap env, now `CONTEXT_PVC`), `housekeeping.yaml`, `context-probe.yaml`. Verify: `charttemplate_test.go`'s `HOME_PVC` assertions are updated and pass against the new name.
- [ ] 4.5 Confirm `contextProbe.claimName` still defaults to the context claim alone. Verify: the existing probe render test passes unchanged in intent.
- [ ] 4.6 Add render tests for the previously-broken combination — a volume name with a defaulted storage class — asserting the convention now makes the working form expressible. Verify: `go test ./internal/integration/...` passes.

## 5. The rest of the tree

- [ ] 5.1 Rename `home` where it names THIS volume in `platform/console/`, `platform/context-sync/` and any remaining chart helper. Verify: `grep -rniI "home" --include=*.go --include=*.ts --include=*.tpl . | grep -v "Home Assistant\|/data/home\|HOME=\|declared home"` returns nothing naming the volume.
- [ ] 5.2 Confirm the exclusions held: `/data/home`, `HOME=/data/home`, `state-durability`'s "one declared home", and every Home Assistant mention in `signals/ha/`, `ha-bundle` and `docs/ha-bundle.md` are untouched. Verify: `git diff --stat` shows no change under `signals/ha/` or `chart/charts/ha-bundle/`.
- [ ] 5.3 Run every module. Verify: the `.github/components.sh modules` loop from `.claude/rules/build-test.md` builds, vets and tests clean.

## 6. Live verification

- [ ] 6.1 Upgrade a throwaway release holding an `agentops-home` claim with NO values change and confirm the render FAILS naming `existingClaim`. Verify: `helm upgrade --dry-run=server` prints the guard's message.
- [ ] 6.2 Set `persistence.context.existingClaim: agentops-home`, upgrade, and confirm nothing moved: the old claim is still bound, no new claim exists, and an existing conversation resumes with its context. Verify: `kubectl get pvc` shows one claim and a follow-up message references earlier turns.
- [ ] 6.3 Bind a pre-created PV by name on a fresh release with `storageClassName: "-"` and confirm the claim BINDS rather than provisioning. Verify: `kubectl get pvc -o jsonpath='{.spec.storageClassName}'` is empty and `.spec.volumeName` is the operator's PV.
- [ ] 6.4 Confirm an `AgentRuntime` still declaring only `spec.home` runs. Verify: apply one by hand, start a conversation, and its pod mounts the volume.

## 7. Documentation

**Reference docs**

- [ ] 7.1 `docs/concepts.md` — the context volume, its field, and the three forms of pointing at storage the chart did not create. Verify: no occurrence of "home volume" remains on the page.
- [ ] 7.2 `docs/CHANGELOG.md`, newest first, and WRITE IT FIRST of the three — for a GitOps install it is the only warning that arrives, because the guard reads the cluster and Argo renders without one. Lead with the `persistence.context.existingClaim: agentops-home` line, then the claim rename, the dual-read window and the deprecated values keys.
- [ ] 7.3 `docs/k8s-bundle.md` and `docs/console.md` — incidental mentions. Verify: `grep -rn "home volume\|homePvcRef\|HOME_PVC" docs/` returns only CHANGELOG history.
- [ ] 7.4 Re-run `python3 .github/scripts/docs-generate.py` — the CRD field and chart values changed, so `docs/cr-reference.md` and every `<!-- generated: … -->` block including `docs/guides/agent-runtime.md` are stale. Verify: the script exits clean and `git diff` shows the regenerated blocks.

**Adopter site**

- [ ] 7.5 `docs/installation.md` — replace the four `persistence.*` rows with the renamed keys, and add the decision this change creates: pointing a volume at storage the chart did not create, in all three forms. Verify: the page names all three and both volumes, and states the `-` convention.
- [ ] 7.6 `docs/getting-started.md` — the RWX troubleshooting row names `agentops-home`; rename it to `agentops-context` and add the failed-static-bind case beside it, since a `Pending` claim looks identical and the cause is not.
- [ ] 7.7 Re-read `docs/index.md` and `docs/concepts.md` as an adopter for anything the rename made incoherent. Verify: no page describes a setting called `contextStorage` against a volume called home.

**Context rules**

- [ ] 7.8 `.claude/rules/terminology.md` — the context volume joins the banned-word table beside `session`, `worker` and `wake`, stating that `/data/home` is the deliberate exception and why.
- [ ] 7.9 `.claude/rules/invariants.md` and `wiring.md` — "home volume" in the substrate-ownership statements becomes the context volume. Verify: `grep -rn "home volume" .claude/` returns nothing.
