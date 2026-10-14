## 1. CRD and capability fields — api-architect

- [ ] 1.1 Add `RuntimeClassName string` to `AgentRuntimeSpec`
  (`platform/manager/api/v1alpha1/agentruntime_types.go`), beside
  `ServiceAccountName`/`NodeSelector`. Verify: `go build ./...` in
  `platform/manager`.
- [ ] 1.2 Add `RuntimeClassName string` to `AgentCapabilitySpec`
  (`platform/manager/api/v1alpha1/agentcapability_types.go`), overriding
  the runtime's own. Update `PipelineSpec`'s CEL mutual-exclusivity field
  list (`pipeline_types.go`) to include it. Verify: `go build ./...`, and
  the existing CEL unit test for the `capabilityRef`/inline exclusivity
  still passes with the new field included.
- [ ] 1.3 Add `RuntimeClassName string` to `Conversation.Spec`
  (`conversation_types.go`), the frozen snapshot field. Verify: `go build
  ./...`.
- [ ] 1.4 Regenerate deepcopy and CRD YAML for `agentruntimes`,
  `agentcapabilities`, `pipelines`, `coordinators`, `conversations` via
  `controller-gen` (`build-test.md`'s two `go run ...controller-tools...`
  commands). Verify: `git diff --stat chart/crds/` shows only the expected
  five CRDs changed, and `go vet ./...` passes.

## 2. Conversation-pod runtime-class resolution — backend-developer

- [ ] 2.1 Add `Config.RuntimeClassName string` to
  `internal/runtimepod/podspec.go`, and overlay it in `FromRuntime` when
  the `AgentRuntimeSpec` sets it. Verify: existing `FromRuntime` unit
  tests still pass, plus a new one asserting the overlay.
- [ ] 2.2 In `ResolveFor`, resolve `runtimeClassName` with the
  AgentCapability-overrides-AgentRuntime precedence, and write the
  resolved value onto `Conversation.Spec.RuntimeClassName` at creation,
  never re-resolving it for an existing conversation. Verify: a unit test
  asserting a Pipeline/Capability override wins over the runtime's own,
  and that editing the capability after creation does not change an
  existing conversation's resolved value.
- [ ] 2.3 In `Build`, set `corev1.PodSpec.RuntimeClassName` from the
  resolved `Config`, as `*string`, only when non-empty. Verify: a unit
  test asserting the field is absent when unset and present with the
  right value when set.

## 3. Component-workload chain — backend-developer

- [ ] 3.1 Add a manager-bootstrap-sourced `runtimeClassName` (env var
  alongside the existing `CONTEXT_SYNC_IMAGE`-style config) read once at
  manager startup. Verify: `go build ./...`, and the bootstrap config
  unit test covers the new var's default (unset).
- [ ] 3.2 Apply it in `ensureAdapterWorkload`
  (`internal/controller/adapterworkload.go`) to every workload it builds.
  Verify: a unit test asserting console's and a signal/channel adapter's
  built `PodSpec` both carry the configured `RuntimeClassName`, and carry
  none when unset.

## 4. Chart: runtime bundle defaults — deployment-engineer

- [ ] 4.1 Add `runtimeClassName` to `global.agentops.runtimeDefaults` in
  `chart/values.yaml`, defaulting to unset, documented beside
  `nodeSelector`. Verify: `helm lint chart/`.
- [ ] 4.2 Render it in `agentops.renderRuntime`
  (`chart/templates/_helpers.tpl`), beside the existing `nodeSelector`
  block, onto the `AgentRuntime` CR's spec. Verify: a chart render test
  asserting `runtimeClassName` appears on a rendered `AgentRuntime` when
  set, and is absent when unset.
- [ ] 4.3 Confirm `agentops.mergedRuntime`'s existing per-key overlay
  already lets `claude`/`ollama`/`copilot` bundles override it with no
  further template change. Verify: a chart render test setting a
  per-bundle override and asserting it wins over the global default.

## 5. Chart: component workload passthrough — deployment-engineer

- [ ] 5.1 Wire the bootstrap env var from task 3.1 into the manager's own
  Deployment (`chart/templates/deployment.yaml`), matching how
  `CONTEXT_SYNC_IMAGE` is already wired. Verify: `helm lint chart/`.
- [ ] 5.2 Add `global.agentops.runtimeClassName` plus a per-component
  `runtimeClassName` override to each of: `chart/templates/deployment.yaml`
  (manager), `housekeeping.yaml`, `context-probe.yaml`,
  `coordination.yaml` (mcp-aops) — mirroring each template's existing
  `nodeSelector` block. Verify: a chart render test per template asserting
  the global default applies and a per-component override wins.
- [ ] 5.3 Add the same global-plus-override pair, net new, to
  `chart/charts/kubernetes/templates/mcp-server.yaml`,
  `chart/charts/prometheus/templates/mcp-server.yaml`,
  `chart/charts/home-assistant/templates/mcp-admin-server.yaml`, and
  `chart/charts/telegram/templates/router.yaml`. Verify: a chart render
  test per template, same assertions as 5.2.

## 6. egress-proxy and context-sync gVisor spike — testing-specialist

- [ ] 6.1 Provision a real `runsc`-enabled cluster node (a VM or
  managed-cloud sandboxed node pool, outside this repo's CI), deploy this
  working copy's chart with `egressMediation.enabled` and
  `runtimeClassName: runsc` on a test runtime, and record whether
  `egress-proxy`'s iptables-NAT-plus-conntrack `SO_ORIGINAL_DST`
  interception actually intercepts and forwards traffic. Verify:
  deliverable is a written finding (pass, fail, or partial, with the
  observed behavior) committed to this change's directory, not a code
  change.
- [ ] 6.2 On the same cluster, run a conversation whose runtime has
  `contextSync.paths` configured, under `runtimeClassName: runsc`, through
  several checkpoint cycles, and confirm the rename/symlink/hardlink
  checkpoint mechanism produces a correct `current` generation. Verify:
  the same written finding document records pass/fail and any observed
  latency or hardlink anomaly.

## 7. Resolve egress-proxy's design from the spike — backend-developer

- [ ] 7.1 If task 6.1 found `egress-proxy` incompatible with `runsc`,
  document the limitation (feeds task 10's security-posture-page update)
  and add a render-time or reconcile-time warning when
  `egressMediation.enabled` and a non-default `runtimeClassName` are both
  set, rather than letting it fail silently at the agent's first outbound
  connection. Verify: a unit test asserting the warning condition is set.
- [ ] 7.2 If task 6.1 found it compatible, no code change is needed for
  this component. Verify: state this explicitly in the task's completion
  note, since an absent task and a skipped one look identical otherwise.

## 8. Unit tests

- [ ] 8.1 Run `go build ./... && go vet ./... && go test ./...` in
  `platform/manager` and confirm the new tests from sections 1-3 pass.
- [ ] 8.2 Run the chart's render-test suite and confirm the new tests
  from sections 4-5 pass.

## 9. System tests

- [ ] 9.1 In `platform/manager/test/system/cluster.go`, install
  `containerd-shim-runsc-v1` inside the k3d node container and patch
  k3s's bundled containerd config template to register the `runsc`
  handler, then define a `RuntimeClass` named `runsc` in the
  provisioned cluster. Verify: `kubectl get runtimeclass runsc` succeeds
  against a cluster the pack provisions.
- [ ] 9.2 Add the new lane from the `end-to-end-testing` delta spec: a
  component workload and a full conversation (with `context-sync` and
  `egress-proxy` sidecars where configured) both rendered with
  `runtimeClassName: runsc`, asserting `egress-proxy`'s result
  explicitly per the task 6/7 finding, isolated so its failure does not
  fail the rest of the pack. Verify: `go test -tags system -count=1
  -timeout 45m -v ./test/system/` run against THIS WORKING COPY's chart
  and images, with the new lane passing or explicitly, legibly skipped
  with its reason.

## 10. Documentation

### Reference docs
- [ ] 10.1 Update `docs/concepts.md`: the new fields on the
  AgentRuntime/AgentCapability/Pipeline/Coordinator field tables, and a
  new subsection under "Execution is wiring too" documenting
  `runtimeClassName` as a third trust/isolation dimension.
- [ ] 10.2 Update `docs/configuration.md` with the new global-plus-per-
  component chart values from sections 4-5.
- [ ] 10.3 Update `docs/security.md` with the kernel-isolation control
  entry, stating the validated scope from the task 6 finding honestly,
  including any `egress-proxy` limitation. Re-run
  `python3 .github/scripts/threat-model.py` since this strengthens a
  trust boundary.
- [ ] 10.4 Update `.claude/rules/wiring.md`'s `runtime`/`identity`
  precedence tables to add the `runtimeClassName` chain, matching
  `docs/concepts.md`'s "Resolution order, stated" section.
- [ ] 10.5 Run `python3 .github/scripts/docs-generate.py` and commit any
  regenerated block, since this change alters CRD fields and chart
  values.

### Adopter site
- [ ] 10.6 Add a new guide under `docs/guides/` walking through enabling
  a custom runtime class, gVisor included, with generated chart-value
  blocks via `docs-generate.py`.
- [ ] 10.7 Update `docs/security.md`'s adopter-facing framing with the
  gVisor-as-validated-control narrative from task 10.3's finding.
- [ ] 10.8 Add at most a one-line pointer in `README.md` to the new guide
  and security entry, keeping within its mirrors-the-landing-page budget.
