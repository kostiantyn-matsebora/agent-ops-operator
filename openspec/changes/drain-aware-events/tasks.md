Every build and test below runs INSIDE the worktree (`docker exec -w "$PWD"`
from `../agent-ops-worktrees/drain-aware-events`), and every deploy uses
`--state-values-set chartPath=` naming this worktree's `chart/` — the defaults
resolve master and report success against it.

## 1. Node view (design decisions 2, 3, 4)

- [ ] 1.1 `signals/k8s-events/drain.go`: `nodeDraining(unschedulable, taints)`
      copied from the manager's `nodeUnschedulable` with the condition-taint
      set; `drain_test.go` pins the set literally, and a matching assertion
      is added to `platform/manager/internal/controller/drain_test.go`.
      `go test ./...` green in both modules in the container.
- [ ] 1.2 Node cache: list/watch `nodes` decoding name, `spec.unschedulable`,
      `spec.taints`; 410 relist; `podcache_test.go`-style test with a fake
      API server that a cordon flips the cached state.
- [ ] 1.3 Missing node access: detect 403 on the initial list, run with drain
      awareness off, add the one-line note to every source's Ready reason
      without `Ready=False`; test both branches.

## 2. Suppression (design decisions 1, 5)

- [ ] 2.1 Pre-queue check before `inhibit.Inhibited` in `main.go`: an event on
      a draining node (pod via cache, or the Node itself) is suppressed and
      counted; `route.drainingNodes` (`suppress` default | `report`) and
      `route.drainingNodeMatchers` parsed in `rules.go` with validation
      errors on the source; tests for suppress, report, matcher narrowing,
      unknown node, and evaluation order against an inhibit rule.
- [ ] 2.2 Bound: `route.drainingNodeBound` (default `1h`); a node draining
      past it posts ONE `kind: Node` signal, reason `NodeDrainExceeded`,
      fingerprint on node + drain start; suppression released until the node
      stops draining; test emits once across repeated ticks and re-arms.
- [ ] 2.3 Ready reporting: draining nodes and suppressed count while active,
      the total once on release, alongside the existing mute reason logic;
      test the transitions.

## 3. Chart and rules

- [ ] 3.1 `chart/charts/kubernetes/templates/events.yaml`: `nodes`
      `list`/`watch` in the ClusterRole only; `values.yaml` documents
      `route.drainingNodes`, `drainingNodeMatchers`, `drainingNodeBound`;
      `helm template` both RBAC modes; `serviceaccount-guard.py` passes.
- [ ] 3.2 Tier 3 gains `kind="Node"` in the shipped rules; update the
      `charttemplate_test.go` pin (node pressure at `for: 0`, now Node-kind)
      and add a scenario that a pod-level `NodeNotReady` reaches the
      catch-all; envtest green in the container.
- [ ] 3.3 `.claude/rules/signal-rules.md`: the fourth axis, the evaluation
      order, the shared taint set and where it is pinned twice.
- [ ] 3.4 Deploy the WORKTREE chart to the live install; `kubectl cordon` a
      node, restart a pod on it, observe the source condition naming the
      node and no conversation; uncordon and observe the total reported.
      Record verdicts, not node names.
- [ ] 3.5 `python3 .github/scripts/publication-guard.py` and
      `retired-vocabulary-guard.py` pass; record the verdict only.

## 4. Documentation

### 4.1 Reference docs

- [ ] 4.1.1 `docs/integrations/kubernetes.md`: re-run
      `python3 .github/scripts/docs-generate.py` for the renders table (the
      grant changed); `--check` passes.
- [ ] 4.1.2 `docs/security.md`: the events adapter's cluster-wide read grant
      now includes nodes, what that exposes (spec, no status kept), and that
      a namespaced install has none; re-run `python3 docs/diagrams/threat-model.py`
      only if a flow crossed a boundary (it did not — state that).
- [ ] 4.1.3 `docs/CHANGELOG.md`: the new default, the new grant, the tier-3
      kind change and what an install that relied on pod-level
      `NodeNotReady` at `for: 0` should expect.

### 4.2 Adopter site

- [ ] 4.2.1 `docs/integrations/kubernetes.md` prose: the reboot-manager
      scenario, the three `route` values, the bound and what its signal
      looks like, the `rbac.clusterWide: false` limitation.
- [ ] 4.2.2 The landing page and `docs/introduction.md`: if either counts the
      suppression axes (dwell, inhibition, time), add node state; otherwise
      state that nothing there changed.
