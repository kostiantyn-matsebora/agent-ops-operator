## MODIFIED Requirements

### Requirement: A matched event must still be true after its dwell window
An event matched by a rule with `for > 0` SHALL be held and re-checked against the live involved object before emitting. The adapter SHALL drop the event when the object no longer exists, and drop it when the object exists and is healthy. Only an object that is still unhealthy at the end of the window SHALL produce a signal.

Verification SHALL proceed as a three-rung ladder:

1. **The kind has a health predicate** — evaluate it. **Pod** is the only kind that SHALL carry one: phase, `Ready` condition, and container waiting reasons (`CrashLoopBackOff`, `ImagePullBackOff`, `ErrImagePull`, `CreateContainerConfigError` and siblings). A container merely being created SHALL NOT count as unhealthy.

   Node, Job and PersistentVolumeClaim deliberately do NOT carry predicates, and fall to rung 2. Giving them predicates would mean watching three more resources — three more external RBAC grants — for a path the shipped defaults never take: the reasons that concern those kinds (`NodeNotReady`, node pressure conditions, `BackoffLimitExceeded`, `DeadlineExceeded`, `VolumeFailedDelete`) all carry `for: 0` and so are never verified at all. Rung 2 is sound for them, since a controller with a live problem keeps re-emitting.
2. **The object exists but has no health predicate** — emit only if the event was **still recurring as the window closed**; drop if it went silent before then. A controller with a live problem keeps re-emitting; a resolved one stops. Recurrence earlier in the window SHALL NOT count on its own: a burst that retries for thirty seconds and then heals has recurred, and is exactly the transient this rung exists to drop.

   "As the window closed" SHALL mean the last arrival fell within the **closing part** of the window actually waited — its final third, with a floor of thirty seconds — derived from the dwell rather than configured, so a rule shortened by `escalateAfterObjects` or by a stricter later rule keeps the same proportion.
3. **Existence cannot be determined** — **emit**, failing open. Silently dropping an object the adapter cannot evaluate at all would convert an unknown into a nonexistent problem, which is the failure this capability exists to prevent.

Rung 2 SHALL NOT be an existence check. An object with no health predicate that still exists is not evidence of an ongoing problem — an autoscaler whose metric lookup flapped once still exists, and treating existence as confirmation would emit on every transient failure of every uninspectable kind.

An emitted signal's evidence SHALL state when the last event arrived relative to the window's close, so a reader can see why it was believed.

`for: 0` SHALL emit immediately with no re-check.

A rule MAY declare `escalateAfterObjects`. When a pending entry accumulates at least that many distinct involved objects sharing a workload, the adapter SHALL **shorten** the dwell — never eliminate it. The shortened window SHALL remain long enough for an ordinary slow start to complete, and SHALL have a floor of at least one minute.

The premise a long dwell rests on — that one object misbehaving is churn — stops holding when several do at once, so waiting the full window to report a total outage is wrong. But breadth alone does not distinguish an outage from a rollout, because **a rollout also makes every replica unready at once**. Emitting on breadth alone therefore fires on every ordinary deployment with a startup delay.

#### Scenario: Breadth reports faster than the full dwell
- **WHEN** a rule declares a 10-minute dwell with `escalateAfterObjects: 3` and three pods of one workload are still unhealthy several minutes in
- **THEN** the signal is emitted well before the ten minutes elapse

#### Scenario: A slow start is not mistaken for an outage
- **WHEN** three replicas of one workload take 45 seconds to become Ready and emit unhealthiness warnings throughout
- **THEN** no signal is emitted, because the shortened window is still longer than the start took

#### Scenario: A healthy rollout produces no signal
- **WHEN** a Deployment rolls out, the terminating pod emits `Unhealthy` and the starting pod emits `Unhealthy` and `FailedScheduling`, and within the dwell window the old pod is gone and the new pod is Ready
- **THEN** no signal is emitted for any of those events

#### Scenario: A broken rollout still fires
- **WHEN** the same events occur but the new pod is still not Ready at the end of the dwell window
- **THEN** exactly one signal is emitted for the workload

#### Scenario: Urgent conditions are not delayed
- **WHEN** a rule matching `NodeNotReady` declares `for: 0`
- **THEN** the signal is emitted on first sight with no liveness re-check

#### Scenario: An unevaluable kind fails open
- **WHEN** a matched event's involved object is of a kind with no health predicate and its existence cannot be determined
- **THEN** the adapter emits the signal rather than dropping it

#### Scenario: A transient failure on an uninspectable kind is dropped
- **WHEN** an autoscaler emits a metric-lookup warning once, the autoscaler still exists at the end of the dwell, and no further such event occurred
- **THEN** no signal is emitted, because existence alone is not confirmation

#### Scenario: A persistent failure on an uninspectable kind is emitted
- **WHEN** the same autoscaler keeps emitting that warning throughout the dwell window
- **THEN** a signal is emitted

#### Scenario: A burst that healed before the window closed is churn
- **WHEN** a kind with no health predicate emits the same warning six times over forty seconds, then nothing more, under a rule with a three-minute dwell
- **THEN** no signal is emitted, because the last arrival fell outside the closing minute of the window

#### Scenario: A controller still retrying at the close is reported
- **WHEN** a kind with no health predicate keeps emitting the same warning every few seconds through the whole three-minute dwell
- **THEN** one signal is emitted at the deadline, carrying the whole burst's evidence and the time of its last arrival

#### Scenario: The closing window follows a shortened dwell
- **WHEN** a rule's dwell is shortened to one minute by escalation or by a stricter rule arriving later
- **THEN** the closing window is thirty seconds — the floor — and not a third of the original dwell
