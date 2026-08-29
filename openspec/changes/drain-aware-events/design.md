## Context

See proposal.md — Why. The adapter already has everything but the node view:
an object cache of trimmed fields (`podcache.go`) that knows each pod's node,
a pre-queue inhibitor (`inhibit.go`), the dwell queue, and Ready-condition
reporting used by mute. The manager already defines "deliberately draining"
(`controller/drain.go` `nodeUnschedulable`) and the reason condition taints are
excluded. Modules are standard-library only and nothing imports across
components (`structure.md`), so the predicate is COPIED and pinned.

## Goals / Non-Goals

**Goals:**
- One planned reboot sweep opens zero conversations, whatever it does to pods.
- The suppression lasts exactly the drain: no cause event, no TTL.
- A drain nobody ends is reported, once.

**Non-Goals:**
- Suppressing events on an UNWELL node. A node that is NotReady with no cordon
  is an incident; tier 3 (now Node-kind) reports it.
- Any manager-side change. Drain awareness in the manager releases runtime
  pods; this is the ingest side of the same fact and they share only the
  predicate.
- A node watch under namespaced RBAC.

## Decisions

### 1. Node STATE, not a cause event — a fourth axis, not an inhibit rule

Inhibition was tried on paper first: `sourceMatchers: kind=Node,
reason=NodeNotReady` → `targetMatchers: kind=Pod, reason=NodeNotReady`. It
fails twice on the night's timeline: the pod events (03:38:32) arrive at the
same second as the Node's, so "cause first" is a race; and `activeTTL` is
10 minutes while agent-2's cycle ran 14. A cordon is observable 30 s before
the first consequence and is retracted explicitly, so reading it removes both
problems at once. Placed BEFORE inhibition in the evaluation order so the two
never interact.

### 2. The predicate is the manager's, copied and pinned

`nodeUnschedulable` from `drain.go` verbatim: `spec.unschedulable` or a
NoSchedule/NoExecute taint not in the condition set. Copying is the rule for
cross-component code; the two tests each assert the taint set literally, so a
key added on one side fails the other's review. Kured is the canonical taint
case (`weave.works/kured` taint when configured), `kubectl cordon` the flag
case.

### 3. The node cache is a third kind in the existing cache

Same list/watch, same trimmed decode (`name`, `spec.unschedulable`,
`spec.taints`), same 410 relist. No status is kept: conditions are what the
predicate deliberately ignores. A cache miss for a pod's node means "not
suppressed" — fail toward reporting.

### 4. Absent node access is degraded, not broken

`Ready=False` is the spec's answer for missing pod access because the adapter
cannot do its job without it. Here it can; a namespaced install chose to see
no nodes. One line on each source's condition, then silence.

### 5. The bound emits a synthetic signal and releases the node

A cordon someone forgot would otherwise silence a node's workloads
indefinitely — the one way this feature could hide an incident. After
`drainingNodeBound` (1h; a Pi kernel reboot takes ~15 min, a slow drain with
PDBs maybe 30) the adapter posts one signal of `kind: Node`, reason
`NodeDrainExceeded`, fingerprint keyed on node + drain start, and stops
suppressing that node. Re-armed only when the node stops draining and drains
again, so the same forgotten cordon is reported once.

### 6. Tier 3 becomes Node-kind

Independent of drains: pod-level `NodeNotReady` at `for: 0` per workload is
wrong even with no reboot manager (a 60-second kubelet blip). With
`kind="Node"` the Node event still fires immediately; the pod-level copies
take the catch-all's 3-minute dwell and the pod re-check, which drops them
when the pod is Ready again. The `charttemplate_test.go` pin that requires
node pressure at `for: 0` is kept and gains the kind.

## Risks / Trade-offs

- [A pod that genuinely breaks DURING a drain is suppressed] → it is still
  broken after the uncordon and emits again; the drain window is bounded.
- [`drainingNodes: suppress` on by default changes what an existing install
  reports] → only on cordoned nodes, and the source condition says so;
  CHANGELOG entry.
- [Taint set drifts between manager and adapter] → both tests pin the literal
  list; `signal-rules.md` states it once.
- [Namespaced installs get nothing from this change] → stated; the tier-3
  kind fix still helps them.

## Migration Plan

Chart upgrade only; the ClusterRole gains a read verb. No CRD change. Rollback
is the previous chart. Installs with `rbac.clusterWide: false` see one new
condition line.

## Open Questions

- Whether `kubectl drain`'s eviction of a pod should count the pod's
  `Killing`/`Evicted` events as drain — `Evicted` is already dropped; no
  decision needed now.
