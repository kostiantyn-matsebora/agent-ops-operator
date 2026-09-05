## Why

A reboot manager cordoning and rebooting nodes one at a time opened eight
`k8s-ops` conversations overnight on 2026-08-29, every one of which the agent
closed with "node reboot, pod is a symptom, already recovered". `NodeNotReady`
is stamped on every POD of a node the controller marks, the shipped tier-3
rule matches it at `for: 0` with no `kind` qualifier, and grouping by workload
turns one planned reboot into one conversation per DaemonSet. Inhibition
cannot answer it: it needs a cause EVENT to arrive first and expires on a
10-minute TTL, while a drain is a node STATE that begins 30 seconds before
anything goes wrong and ends exactly when the node is uncordoned.

## What Changes

- **The adapter watches nodes and knows which are DRAINING**, by the same
  predicate the manager's drain awareness uses: `spec.unschedulable`, or a
  `NoSchedule`/`NoExecute` taint outside the condition-taint set
  (`not-ready`, `unreachable`, the pressure taints). An unwell node is not a
  draining node; a cordoned or maintenance-tainted one is, however it got
  there.
- **Events on a draining node are suppressed as a matter of node state**, not
  cause-and-effect: any event whose involved object sits on that node (pods,
  through the object cache) and any event on the Node itself, for as long as
  the node is draining. Evaluated before the dwell queue, like inhibition,
  so nothing occupies the queue. Opt-out per source under `route`
  (`route.drainingNodes: suppress` default, `report` to disable), with an
  optional matcher narrowing what is suppressed.
- **A forgotten cordon becomes a signal instead of permanent silence**: a node
  draining longer than a configured bound (default 1h) emits ONE Node-kind
  signal naming it, and suppression on that node stops. Silence that never
  ends is the one way this feature could hide an incident.
- **Suppression is visible on the source** — `Ready=True` with a reason naming
  the draining nodes and the count suppressed, then the count on release —
  exactly as mute reports itself.
- **RBAC**: the events component's ClusterRole gains `nodes` `list`/`watch`.
  Nodes are cluster-scoped, so a namespaced (`rbac.clusterWide: false`)
  install has no node view; the feature is then OFF and the source says so,
  once. It is not `Ready=False`: a namespaced install chose that scope.
- **The tier-3 node-condition rule is scoped to `kind="Node"`**, so a pod-level
  `NodeNotReady` outside a drain falls to the catch-all's dwell and liveness
  re-check instead of firing per workload at `for: 0`. The bundle spec already
  says tier 3 is for "node-level conditions"; the matcher now says it too.

## Capabilities

### New Capabilities
- `k8s-drain-awareness`: the draining predicate, what is suppressed while a
  node drains, the bound and its escalation signal, visibility, and the
  degraded mode without node access.

### Modified Capabilities
- `k8s-event-suppression`: a fourth axis beside rules, inhibition and time —
  node STATE — with its place in the evaluation order stated.
- `k8s-events-signal-adapter`: the object cache also holds nodes; absent node
  access degrades to "drain awareness off" rather than `Ready=False`.
- `k8s-bundle`: the events component's grant carries `nodes` cluster-wide
  only; the shipped tier-3 rule is Node-kind.

## Impact

- `signals/k8s-events/`: a node cache beside `podcache.go`, a `drain.go`
  predicate mirroring `platform/manager/internal/controller/drain.go` (copied,
  not imported — modules are standard-library only, and both tests pin the
  same taint set), the pre-queue check beside `inhibit.go`, the bound timer
  and its synthetic signal, the Ready-condition reporting.
- `chart/charts/kubernetes/`: `nodes` in `events.yaml`'s ClusterRole, the
  `route.drainingNodes` value, the tier-3 matcher; the `charttemplate_test.go`
  pin for tier 3 moves with it.
- `.claude/rules/signal-rules.md`: the fourth axis, and the shared taint set.
- **Reference docs**: `docs/concepts.md` if it lists the suppression axes,
  `docs/integrations/kubernetes.md` (what the events component renders is
  generated — re-run `docs-generate.py`; the drain section is prose),
  `docs/security.md` (a new cluster-wide read grant: nodes), `docs/CHANGELOG.md`.
- **Adopter site**: `docs/integrations/kubernetes.md` is the adopter page for
  this bundle — the reboot-manager scenario, the value, the bound; the landing
  page's suppression claim if it counts axes; `docs/installation.md` is
  unaffected (a subchart value belongs to the bundle page).
