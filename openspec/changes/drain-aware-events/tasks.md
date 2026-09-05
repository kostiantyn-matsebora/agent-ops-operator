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

- [ ] 3.1 `chart/charts/kubernetes/templates/events.yaml`: `nodes`
      `list`/`watch` in the ClusterRole only; `values.yaml` documents
      `route.drainingNodes`, `drainingNodeMatchers`, `drainingNodeBound`.
- [ ] 3.2 Tier 3 gains `kind="Node"` in the shipped rules.
- [ ] 3.3 `.claude/rules/signal-rules.md`: the fourth axis, the evaluation
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
- [ ] 4.7 `chart/`: `helm template` both RBAC modes (`rbac.clusterWide: true`
      and `false`) renders the `nodes` grant only in the cluster-wide
      ClusterRole; `python3 .github/scripts/serviceaccount-guard.py` passes.
- [ ] 4.8 `platform/manager/internal/integration/charttemplate_test.go`: the
      pin for tier 3 gains `kind="Node"` (node pressure still reports at
      `for: 0`, now qualified), and a new scenario asserts a pod-level
      `NodeNotReady` event is NOT matched by tier 3 and reaches the
      undwelled catch-all instead.
- [ ] 4.9 Run every touched module's full suite green in the container per
      `.claude/rules/build-test.md`: `go build ./... && go vet ./... && go
      test ./...` in `signals/k8s-events/` and in `platform/manager/` (the
      latter with `KUBEBUILDER_ASSETS` set, for the envtest suite).
- [ ] 4.10 `python3 .github/scripts/publication-guard.py` and
      `python3 .github/scripts/retired-vocabulary-guard.py` pass; record the
      verdict only, never the matched text.

## 5. E2E tests

- [ ] 5.1 Deploy the WORKTREE chart to the live install
      (`--state-values-set chartPath=` naming this worktree's `chart/`).
      `kubectl cordon` a node, restart a pod scheduled on it, observe the
      source's Ready condition naming the draining node and that no
      conversation opens. Uncordon and observe the suppressed-count total
      reported on release. Record verdicts, not node names.
- [ ] 5.2 With `rbac.clusterWide: false`, verify the source reports drain
      awareness off (one line, `Ready=True` unaffected) and ordinary event
      reporting is unchanged.

## 6. Documentation

### 6.1 Reference docs

- [ ] 6.1.1 `docs/integrations/kubernetes.md`: re-run
      `python3 .github/scripts/docs-generate.py` for the renders table (the
      grant changed); `--check` passes.
- [ ] 6.1.2 `docs/security.md`: the events adapter's cluster-wide read grant
      now includes nodes, what that exposes (spec, no status kept), and that
      a namespaced install has none; re-run `python3 docs/diagrams/threat-model.py`
      only if a flow crossed a boundary (it did not — state that).
- [ ] 6.1.3 `docs/CHANGELOG.md`: the new default, the new grant, the tier-3
      kind change and what an install that relied on pod-level
      `NodeNotReady` at `for: 0` should expect.

### 6.2 Adopter site

- [ ] 6.2.1 `docs/integrations/kubernetes.md` prose: the reboot-manager
      scenario, the three `route` values, the bound and what its signal
      looks like, the `rbac.clusterWide: false` limitation.
- [ ] 6.2.2 The landing page and `docs/introduction.md`: if either counts the
      suppression axes (dwell, inhibition, time), add node state; otherwise
      state that nothing there changed.
