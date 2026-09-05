---
paths:
  - "signals/**"
  - "platform/manager/internal/ingest/**"
  - "platform/manager/internal/integration/charttemplate_test.go"
  - "chart/charts/kubernetes/**"
  - "chart/charts/home-assistant/**"
---

## The rule vocabulary the event and log adapters share

**`rules` and `route` are BORROWED NAMES**, and what they may mean is fixed by
where they were borrowed from. Both `signal-k8s-events` and `signal-ha` use
them; only the first has the time axis.

### `for:` is Prometheus, `group_wait` is Alertmanager, and they are NOT the same thing

`signal-k8s-events` config is deliberately two halves:

| Half | Borrowed from | Says |
|---|---|---|
| `rules` | Prometheus | what counts as a problem, and how long it must hold |
| `route` | Alertmanager | inhibition |

**Alertmanager's `group_wait` batches a group before its FIRST notification.**
`for:` does not exist in Alertmanager at all. Spelling dwell as `group_wait`
would be an Alertmanager term meaning something Alertmanager does not mean.

Two further rules the defaults depend on, both pinned in
`internal/integration/charttemplate_test.go`:

- **Reasons describing a COMPLETED event must carry `for: 0`** — `OOMKilling`,
  `SystemOOM`, `BackoffLimitExceeded`, `DeadlineExceeded`. A dwell finds the
  healthy replacement and erases the incident.
- **The LAST rule must be a catch-all with a dwell, never a drop**, so an
  unanticipated reason is verified rather than discarded.

**`Evicted` is the exception, and is DROPPED** as of chart 5.9.0. It used to sit
in the past-tense set.

An eviction is reported from both ends already and per POD from neither:

- **Kubelet evictions are caused by node pressure**, which tier 3 reports at
  `for: 0` as ONE node-level signal rather than one per displaced pod.
- **API-initiated evictions are drains** — routine, and UNATTENDED wherever a
  reboot manager runs.
- **The case worth acting on is a pod that does not come back**, which arrives
  as `FailedScheduling` with a dwell to confirm it.

**The drop is therefore only defensible while BOTH substitutes survive**, so the
test pins node pressure at `for: 0` and `FailedScheduling`'s presence TOGETHER
with the drop. Re-tuning one of them must not silently leave eviction unreported
from every direction at once.

**TIER 3 IS `kind="Node"`, AND THAT QUALIFIER IS WHAT MAKES "ONE node-level
signal" TRUE.** `NodeHasDiskPressure` / `NodeHasMemoryPressure` genuinely are
reported once, against the Node object, by the kubelet — but `NodeNotReady` is
NOT: the node lifecycle controller stamps it on every POD scheduled on the
affected node, so without the qualifier one node blip fired once per
DaemonSet, at `for: 0`, with no chance to notice the node coming back in the
next few seconds. A pod-level `NodeNotReady` now falls to the catch-all, whose
existing dwell and pod-health re-check is what lets a self-healing reboot go
unreported without any new mechanism — see the fourth axis below for what
happens when the node genuinely stays down.

### The fourth suppression axis: node STATE (drain awareness)

**Beside `rules` (`for:`), inhibition and the time axis, there is a NODE-STATE
axis**: `route.drainingNodes` (`suppress`, the default, or `report`),
`route.drainingNodeMatchers`, `route.drainingNodeBound` (default `1h`). While a
node is cordoned or maintenance-tainted, every event on its objects — and on
the Node itself — is suppressed for exactly as long as the drain lasts, no
matter which reason it carries. This is what makes a rolling reboot manager
(kured is the canonical case) open ZERO conversations regardless of how many
DaemonSets have pods on the rebooting node.

- **INHIBITION WAS TRIED FIRST, AND MEASURED TO FAIL.** A cordon is a STATE,
  not a cause EVENT: the pod-level consequences arrive within the same second
  as the node's own event, so "cause must be seen first" is a race inhibition
  loses, and `activeTTL` (10 minutes) is shorter than some drains actually run.
  Reading the state directly needs no race and no guessed timeout — it starts
  when the cordon does and ends when the uncordon does, exactly.
- **EVALUATED BEFORE INHIBITION AND THE DWELL QUEUE**, so the two axes never
  interact and nothing occupies the queue for an event that will not emit.
- **THE PREDICATE IS THE MANAGER'S, COPIED VERBATIM** —
  `signals/k8s-events/drain.go`'s `nodeDraining` mirrors
  `platform/manager/internal/controller/drain.go`'s `nodeUnschedulable`: a node
  is draining when `spec.unschedulable` is true, or tainted `NoSchedule` /
  `NoExecute` with a key OUTSIDE the condition-taint set (`not-ready`,
  `unreachable`, the pressure taints, `network-unavailable`,
  `out-of-service`). An unwell node is not a draining node — see
  `invariants.md`'s "CONDITION TAINTS ARE NOT DRAINS" for why that distinction
  matters on the manager side too.
- **THE TAINT SET IS PINNED LITERALLY, TWICE** — `signals/k8s-events/drain_test.go`'s
  `TestConditionTaintSet` and `platform/manager/internal/controller/drain_test.go`'s
  `TestConditionTaintSetMirroredBySignalK8sEvents` — because the two copies
  cannot import each other (modules here are standard-library only) and a key
  added to one without the other means the adapter and the manager disagree
  about what counts as deliberate.
- **A DRAIN NOBODY ENDS IS REPORTED, ONCE.** Past `drainingNodeBound`, the
  adapter posts ONE `kind: Node` signal (reason `NodeDrainExceeded`,
  fingerprinted on the node AND the drain's start time so a second forgotten
  cordon on the same node is not silenced as a repeat) and releases
  suppression on that node until it drains again. This is the one way the
  feature could otherwise hide an incident — a cordon nobody remembers to lift.
- **NEEDS THE `nodes` GRANT, cluster-wide only** — nodes are cluster-scoped,
  so a namespaced install (`rbac.clusterWide: false`) has no equivalent, and
  the axis degrades to OFF rather than failing the source: every source notes
  it once on its Ready condition, without `Ready=False`.

**The TIME axis** (`route.timeIntervals` + `route.muteTimeIntervals`) is
Alertmanager vocabulary too, borrowed field-for-field.

A scheduled outage is the one thing the other three axes cannot express: `for:`
verifies a condition the outage genuinely satisfies, inhibition needs a cause
event a power cut never produces, and no label carries the time of day.

- **Mute is evaluated at EMIT** — after the dwell, before the emit cap — and
  that ordering IS the safety property. A problem outliving the window still
  emits once it closes, and a muted burst never spends the emit budget.
- **`location` defaults to UTC but must be NAMED**, because a UTC-pinned window
  drifts an hour at each DST change. `_ "time/tzdata"` is imported in `mute.go`
  — distroless carries no zoneinfo, so without it every valid zone is rejected.
- **A window with no `matchers` deafens the source outright**, which is why the
  shipped example narrows.
- **Muting reports itself on the source's Ready condition** — `Ready=True`,
  reason `Muted`, then `MuteEnded` with the count. A muted lane and an idle lane
  are otherwise indistinguishable.

### Event grouping is by workload

`[namespace, workload]`, resolved through OWNER REFERENCES (Pod → ReplicaSet →
Deployment) and never by parsing a pod name — that breaks on StatefulSets
(`api-0`), DaemonSets and bare pods.

Pod names are unique per replica and regenerated every rollout, so the old
`[namespace, kind, name]` default made conversations scale with pods × rollouts
and the 7-day window reuse could never fire.
