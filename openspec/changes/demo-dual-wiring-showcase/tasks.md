## 1. Chart implementation — deployment-engineer

- [x] 1.1 Add a `pipelines.demoShowcase` block to
  `chart/charts/kubernetes/values.yaml` (`name: k8s-observe-pipeline`,
  `description`, `icon`). Verify `helm lint chart/charts/kubernetes`
  passes in this worktree.
- [x] 1.2 Add the `kubernetes.demoShowcaseActive` helper to
  `chart/charts/kubernetes/templates/_helpers.tpl`, mirroring
  `kubernetes.wiringActive`'s nullable pattern (design.md). Verify
  `helm template chart/ --set global.demo.enabled=true --set
  global.demo.wiringShowcase=false` renders no `k8s-observe-pipeline`
  object.
- [x] 1.3 Add `chart/charts/kubernetes/templates/pipeline-showcase.yaml`,
  rendering the showcase `Pipeline` when `agentops.wiringMode` resolves
  `coordinator`, `kubernetes.demoShowcaseActive` is true,
  `kubernetes.wiringActive` is true, and `profile.enabled` is true. It
  claims `cluster-events` and the console's signal source and channel,
  reusing the observing route's profile and toolset. Verify `helm
  template chart/ --set global.demo.enabled=true` renders exactly one
  extra `Pipeline` named `k8s-observe-pipeline`, alongside the existing
  `Coordinator` and `AgentCapability` objects.
- [x] 1.4 Document `global.demo.wiringShowcase` beside `global.demo.enabled`
  in `chart/values.yaml`'s comment block. Verify `helm lint chart/`
  passes.
- [x] 1.5 Update `chart/templates/NOTES.txt` (the kubernetes subchart ships
  no `NOTES.txt` of its own — every bundle's notice lives in the parent's,
  keyed by `index .Values "kubernetes"`, same convention as the existing
  "claimed TWICE" notice) to name the showcase route and state that a
  cluster `Warning` event now opens two conversations when it renders.
  `helm template --show-only` cannot select `NOTES.txt` (confirmed: not a
  selectable template) and `helm install --dry-run` needs a reachable
  cluster (confirmed: `Kubernetes cluster unreachable` here) — so the
  literal verification command is not performable in this environment.
  Verified instead by executing the full render (a broken reference in
  `NOTES.txt` does fail `helm template`, confirmed by deliberately
  injecting one) across the `default`, `demo`, `kubernetes`, `everything`,
  `demo+wiringShowcase=false`, and `demo+wiringMode=pipelines`
  permutations — all exit 0. Reading the rendered NOTES text itself is
  cluster-only and NOT performed here.

## 2. Unit tests

- [x] 2.1 Add render-test scenarios covering four cases, then verify
  `go test ./...` in `platform/manager` passes from this worktree:
  - demo defaults render both the `Coordinator` and the showcase
    `Pipeline`
  - `global.demo.wiringShowcase: false` renders no showcase `Pipeline`
  - `global.demo.enabled: true` with `global.agentops.wiringMode:
    pipelines` renders no showcase and no `Coordinator`
  - a non-demo install renders no showcase object, whatever `wiringMode`
    is set to

  Added `TestDemoModeAlsoShowcasesAPlainPipelineUnderCoordinator` in
  `charttemplate_test.go`, covering all four. `KUBEBUILDER_ASSETS=` the
  session's envtest path (`~/.envtest` resolved to the wrong `$HOME` in
  this environment) `go test ./...` in `platform/manager` — all packages
  pass, including `internal/integration`.
- [x] 2.2 Audit existing chart render tests that assert demo mode's
  default manifest shape with no explicit `wiringMode` override, and
  update any expected object list or count for the new showcase
  `Pipeline`. Verify `go test ./...` in `platform/manager` stays green.

  Audited every bare `global.demo.enabled=true` call across
  `internal/integration/*_test.go` (`charttemplate_test.go`,
  `charttemplate_identity_test.go`, `charttemplate_prometheus_test.go`,
  `charttemplate_runtimes_test.go`). None assert a Pipeline/Coordinator
  object list or count — each checks RBAC subjects, bundle presence/absence
  by name, a guard's error text, or a PVC's access mode, none of which the
  showcase route touches. No existing test needed updating. Full
  `go test ./...` confirmed green (see 2.1).

## 3. E2E tests

- [x] 3.1 Not applicable — ticked. This change is chart-template rendering
  only. It adds no new kubelet, RBAC, informer, or pod-lifecycle behavior:
  the multi-claimant source fan-out and the chat choice-list for several
  claimants are pre-existing, already-covered cluster mechanisms this
  change does not alter.

## 4. Documentation — reference

- [x] 4.1 Add a row for `global.demo.wiringShowcase` to
  `docs/configuration.md`, beside the existing
  `global.agentops.wiringMode` row.
- [x] 4.2 Update `docs/concepts.md` if it names demo mode's wiring
  posture, to mention the showcase `Pipeline`.
- [x] 4.3 Update `.claude/rules/chart.md`'s kubernetes bundle section and
  "THE DEMO WIRES THE CONSOLE" section for the new showcase branch.
- [ ] 4.4 Re-run `python3 .github/scripts/docs-generate.py` (this change
  adds a chart value) and verify it reports no stale generated block.

## 5. Documentation — adopter site

- [x] 5.1 Update `docs/getting-started.md`: drop the
  `--set global.agentops.wiringMode=pipelines` pin and its stale
  "console does not auto-wire" callout, install with demo mode's own
  default, and walk through addressing both the coordinator and the
  showcase pipeline by name.
- [x] 5.2 Update `docs/guides/coordinate-agents.md`'s callout to reflect
  that the console source and escalation channel are now claimed for the
  chart-rendered `Coordinator` (#296), restate the remaining
  escalation-only channel behavior as a deliberate difference from a
  `Pipeline`, and point at the showcase `Pipeline` as the always-in-thread
  counterpart.
- [ ] 5.3 Re-run `python3 .github/scripts/docs-generate.py --check` and
  verify it passes.
