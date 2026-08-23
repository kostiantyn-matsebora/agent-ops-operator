---
paths:
  - "signals/**"
  - "platform/manager/internal/ingest/**"
  - "platform/manager/internal/integration/charttemplate_test.go"
  - "chart/charts/k8s-bundle/**"
  - "chart/charts/ha-bundle/**"
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
