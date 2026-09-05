Every build and test below runs INSIDE the worktree (`docker exec -w "$PWD"`
from `../agent-ops-worktrees/drain-aware-events`), and every deploy uses
`--state-values-set chartPath=` naming this worktree's `chart/` — the defaults
resolve master and report success against it.

## 1. Node view (design decisions 2, 3, 4)

- [x] 1.1 `signals/k8s-events/drain.go`: `nodeDraining(unschedulable, taints)`
      copied from the manager's `nodeUnschedulable` with the condition-taint
      set; a matching assertion is added to
      `platform/manager/internal/controller/drain_test.go` so the two taint
      sets cannot drift apart unnoticed.
- [x] 1.2 Node cache: list/watch `nodes` decoding name, `spec.unschedulable`,
      `spec.taints`; 410 relist.
- [x] 1.3 Missing node access: detect 403 on the initial list, run with drain
      awareness off, add the one-line note to every source's Ready reason
      without `Ready=False`.

## 2. Suppression (design decisions 1, 5)

- [x] 2.1 Pre-queue check before `inhibit.Inhibited` in `main.go`: an event on
      a draining node (pod via cache, or the Node itself) is suppressed and
      counted; `route.drainingNodes` (`suppress` default | `report`) and
      `route.drainingNodeMatchers` parsed in `rules.go` with validation
      errors on the source.
- [x] 2.2 Bound: `route.drainingNodeBound` (default `1h`); a node draining
      past it posts ONE `kind: Node` signal, reason `NodeDrainExceeded`,
      fingerprint on node + drain start; suppression released until the node
      stops draining and drains again.
- [x] 2.3 Ready reporting: draining nodes and suppressed count while active,
      the total once on release, alongside the existing mute reason logic.

## 3. Chart and rules

- [x] 3.1 `chart/charts/kubernetes/templates/events.yaml`: `nodes`
      `list`/`watch` in the ClusterRole only; `values.yaml` documents
      `route.drainingNodes`, `drainingNodeMatchers`, `drainingNodeBound`.
- [x] 3.2 Tier 3 gains `kind="Node"` in the shipped rules.
- [x] 3.3 `.claude/rules/signal-rules.md`: the fourth axis, the evaluation
      order, the shared taint set and where it is pinned twice.

## 4. Unit tests

- [x] 4.1 `signals/k8s-events/drain_test.go`: pins the condition-taint set
      literally (`not-ready`, `unreachable`, the pressure taints) and covers
      `nodeDraining` over `spec.unschedulable` and each taint case; matching
      assertion added in task 1.1 keeps `platform/manager/internal/controller`
      in sync.
- [x] 4.2 Node cache test, `podcache_test.go`-style, with a fake API server:
      a cordon flips the cached state; a 410 triggers relist.
- [x] 4.3 Missing-node-access test: a 403 on the initial node list leaves the
      adapter running with drain awareness off and the Ready-reason note
      present, never `Ready=False`.
- [x] 4.4 Suppression tests in `main.go`'s test file: suppress (default),
      report, matcher narrowing, an event on an unknown node (fail toward
      reporting), and evaluation order — draining-node suppression runs
      BEFORE `inhibit.Inhibited` and the two never double-count.
- [x] 4.5 Bound test: the synthetic `NodeDrainExceeded` signal emits exactly
      once across repeated ticks past `drainingNodeBound`, and re-arms after
      the node stops and starts draining again.
- [x] 4.6 Ready-condition test: the draining-nodes/suppressed-count message
      while active, and the total once on release.
- [x] 4.7 `chart/`: `helm template` both RBAC modes (`rbac.clusterWide: true`
      and `false`) renders the `nodes` grant only in the cluster-wide
      ClusterRole; `python3 .github/scripts/serviceaccount-guard.py` passes.
- [x] 4.8 `platform/manager/internal/integration/charttemplate_test.go`: the
      pin for tier 3 gains `kind="Node"` (node pressure still reports at
      `for: 0`, now qualified), and a new scenario asserts a pod-level
      `NodeNotReady` event is NOT matched by tier 3 and reaches the
      undwelled catch-all instead (the mechanism-level half of that scenario
      lives in `signals/k8s-events/rules_test.go`, since the module boundary
      keeps the rules engine out of `platform/manager`).
- [x] 4.9 Run every touched module's full suite green in the container per
      `.claude/rules/build-test.md`: `go build ./... && go vet ./... && go
      test ./...` in `signals/k8s-events/` and in `platform/manager/` (the
      latter with `KUBEBUILDER_ASSETS` set, for the envtest suite).
- [x] 4.10 `python3 .github/scripts/publication-guard.py` and
      `python3 .github/scripts/retired-vocabulary-guard.py` pass; record the
      verdict only, never the matched text. Verdicts: publication-guard clean
      (109 files); retired-vocabulary-guard clean.

## 5. E2E tests

- [x] 5.1 Superseded by an automated lane rather than a manual live-install
      check: `docs/testing.md` names the E2E tier as the chart on a real k3s
      cluster under k3d, not the live install, so that is where this belongs
      permanently. `platform/manager/test/e2e/lanes_test.go`'s
      `TestK8sEventsDrainAwareness` cordons the (single, real) e2e node,
      schedules a subject pod onto it, posts a synthetic `OOMKilling` event
      and confirms no conversation opens; uncordons, posts the same event
      again, and confirms one does. Verdict: PASS, 71s, smoke tier (gates
      every release, not just the nightly full pack). Caught one real race
      first (a synthetic event posted immediately after cordon/uncordon can
      beat the adapter's own node-watch propagation) — fixed with a 10s
      settle after each transition, not by widening any window in the code
      under test.
- [x] 5.2 With `rbac.clusterWide: false`, verify the source reports drain
      awareness off (one line, `Ready=True` unaffected) and ordinary event
      reporting is unchanged. Verdict: covered at the tier that actually
      decides it — `TestReportNodeAccessErrorDoesNotFailTheSource`
      (`signals/k8s-events/main_test.go`) simulates the 403 directly and
      asserts `Ready=True` with the note present, and
      `TestEventsAdapterRBACGrantsNodesOnlyClusterWide`
      (`platform/manager/internal/integration/charttemplate_test.go`) pins
      that the namespaced Role never renders the grant at all. A second full
      e2e cluster install with `rbac.clusterWide: false` would only be
      re-proving that Kubernetes RBAC enforces as documented, which is not
      this change's claim to verify.

## 6. Documentation

### 6.1 Reference docs

- [x] 6.1.1 `docs/integrations/kubernetes.md`: re-run
      `python3 .github/scripts/docs-generate.py` for the renders table (the
      grant changed); `--check` passes. Verdict: 49 generated file(s) up to
      date — the renders table lists object kind/name pairs, not RBAC verb
      lists, and this change adds a verb to an existing ClusterRole rather
      than a new rendered object, so nothing in that table moved.
- [x] 6.1.2 `docs/security.md`: the events adapter's cluster-wide read grant
      now includes nodes, what that exposes (spec, no status kept), and that
      a namespaced install has none; re-run `python3 docs/diagrams/threat-model.py`
      only if a flow crossed a boundary (it did not — state that). New
      "The events adapter's own read access" subsection under *Cluster
      authorization*. No boundary crossed: the adapter already crosses into
      the cluster API; this widens an existing grant rather than adding a
      new flow, so the threat-model diagrams are untouched.
- [x] 6.1.3 `docs/CHANGELOG.md`: the new default, the new grant, the tier-3
      kind change and what an install that relied on pod-level
      `NodeNotReady` at `for: 0` should expect.

### 6.2 Adopter site

- [x] 6.2.1 `docs/integrations/kubernetes.md` prose: the reboot-manager
      scenario, the three `route` values, the bound and what its signal
      looks like, the `rbac.clusterWide: false` limitation. New "stay quiet
      through a rolling reboot" section; the pre-existing nightly-mute
      section reframed for what it is actually still for (no node state
      changes at all); the "OOM kills" worked example stopped recommending
      an unqualified `NodeNotReady` matcher.
- [x] 6.2.2 The landing page and `docs/introduction.md`: if either counts the
      suppression axes (dwell, inhibition, time), add node state; otherwise
      state that nothing there changed. Verdict: neither page enumerates the
      suppression axes at all (confirmed by search), so nothing there
      changed.
