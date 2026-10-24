# k8s-event-suppression delta

## MODIFIED Requirements

### Requirement: A matched event must still be true after its dwell window
An event matched by a rule with `for > 0` SHALL be held and re-checked before emitting. The adapter SHALL drop the event when its involved object is known to be gone, and drop it when the object is evaluable and healthy.

Only a problem still true at the end of the window SHALL produce a signal.

Verification SHALL proceed as a three-rung ladder:

1. **The kind has a health predicate** — evaluate it. **Pod** is the only kind that SHALL carry one: phase, `Ready` condition, and container waiting reasons (`CrashLoopBackOff`, `ImagePullBackOff`, `ErrImagePull`, `CreateContainerConfigError` and siblings). A container merely being created SHALL NOT count as unhealthy.

   Node, Job and PersistentVolumeClaim deliberately do NOT carry predicates, and fall to rung 2. Giving them predicates would mean watching three more resources — three more external RBAC grants — for a path the shipped defaults never take: the reasons that concern those kinds (`NodeNotReady`, node pressure conditions, `BackoffLimitExceeded`, `DeadlineExceeded`, `VolumeFailedDelete`) all carry `for: 0` and so are never verified at all. Rung 2 is sound for them, since a controller with a live problem keeps re-emitting.
2. **The kind has no health predicate** — emit only if the event was **still recurring as the window closed**, and drop if it went silent before then. Whether the object's existence can be determined SHALL NOT change this verdict: existence is not evidence of an ongoing problem, and undeterminable existence is not evidence of one either. Recurrence is the only honest confirmation an uninspectable kind offers — a controller with a live problem keeps re-emitting, and a resolved one stops. Recurrence earlier in the window SHALL NOT count on its own: a burst that retries for thirty seconds and then heals has recurred, and is exactly the transient this rung exists to drop.

   "As the window closed" SHALL mean the last arrival fell within the **closing part** of the window actually waited — its final third, with a floor of thirty seconds — derived from the dwell rather than configured, so a rule shortened by `escalateAfterObjects` or by a stricter later rule keeps the same proportion.
3. **The adapter's own state is unreadable** — **emit**, failing open. This rung is about the adapter's OWN view, never about a kind: when the re-check cannot consult the adapter's object cache or pending state at the close, silently dropping would convert the adapter's own outage into a clean bill of health. It mirrors the HA adapter's unreadable-log rule — an unreadable source of truth emits.

Rung 2 SHALL NOT be an existence check. An object with no health predicate that still exists is not evidence of an ongoing problem.

An autoscaler whose metric lookup flapped once still exists, and treating existence as confirmation would emit on every transient failure of every uninspectable kind.

An emitted signal's evidence SHALL state when the last event arrived relative to the window's close, so a reader can see why it was believed.

`for: 0` SHALL emit immediately with no re-check.

A rule MAY declare `escalateAfterObjects`. When a pending entry accumulates at least that many distinct involved objects sharing a workload, the adapter SHALL **shorten** the dwell — never eliminate it.

The shortened window SHALL remain long enough for an ordinary slow start to complete, and SHALL have a floor of at least one minute.

The premise a long dwell rests on — that one object misbehaving is churn — stops holding when several do at once, so waiting the full window to report a total outage is wrong.

But breadth alone does not distinguish an outage from a rollout, because **a rollout also makes every replica unready at once**. Emitting on breadth alone therefore fires on every ordinary deployment with a startup delay.

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
- **WHEN** a dwelled event's window closes while the adapter cannot read its own object cache, so nothing about the involved object can be evaluated at all
- **THEN** the adapter emits the signal rather than dropping it — its own outage is never a clean bill of health

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

### Requirement: A source's emit rate is capped and clipping is reported
The adapter SHALL cap the number of signals it emits per source per minute. When the cap is reached it SHALL stop emitting for the remainder of the window and report the clipping on that source's `Ready` condition, naming the count. Clipping SHALL NOT be silent.

The same rule SHALL cover every volume bound the adapter imposes on itself:

- A list pass that stops examining events at a per-pass batch bound SHALL report that clip on the source's `Ready` condition, naming the count.
- A bounded pass SHALL NOT advance its read position past an event it did not examine — the remainder is deferred to the next pass, never lost.

This cap is adapter-local and bounded in ambition: it turns a fast runaway into a visible condition rather than an unbounded write load. It does not replace a manager-side limit on conversation creation.

#### Scenario: A runaway becomes a visible condition
- **WHEN** matched events would produce more signals than the per-minute cap for a source
- **THEN** emission stops for that window and the source's Ready condition reports how many signals were clipped

#### Scenario: An event storm on a relist loses nothing
- **WHEN** a relist pass matches more events than the per-pass batch bound
- **THEN** the pass stops at the bound, keeps its read position at the last examined event, reports the clip, and a later pass considers the remainder

#### Scenario: Normal volume is unaffected
- **WHEN** a source emits fewer signals than its cap
- **THEN** no condition change is made and every signal is delivered

### Requirement: An active mute is visible on the source
While a mute window is active the adapter SHALL report it on that source's `Ready` condition, naming the interval and the running count of events it has muted.

The report SHALL be driven by the window's own clock, never only by event arrival. A window over a lane that goes quiet still reports itself while open.

The close SHALL be reported with the final count — zero included — even when no event arrives to trigger it.

Muting SHALL NOT be silent. A muted lane and an idle lane are indistinguishable from outside, and only one of them means the cluster is healthy.

"Why has nothing arrived since four?" SHALL be answerable from the source object rather than from adapter logs.

#### Scenario: Silence is explained
- **WHEN** a mute window is active
- **THEN** the source's Ready condition names the interval that is muting it

#### Scenario: The cost of the window is reported
- **WHEN** a mute window closes
- **THEN** the number of events it muted is reported, so an over-broad window is visible rather than inferred

#### Scenario: A quiet lane still reports its window
- **WHEN** a mute window opens and closes over a source that produces no events
- **THEN** the Ready condition reports the window while it is active and reports its close with a count of zero

## ADDED Requirements

### Requirement: The pending queue declares a post-emit refractory window

After emitting a pending entry's signal, the adapter SHALL suppress its own
re-posts of that entry — the same `initiationId` — for the emitting rule's
own dwell, floored at one minute.

This is the signal adapter contract's one named allowance, declared here as
that allowance requires. The window opens only on emit, never collapses
distinct ids, and substitutes for nothing: the manager's cooldown stays the
dedup authority, and the window only spends fewer round trips.

Without it, a fixed escalation would be re-reported every evaluation tick —
chatter the manager's cooldown would absorb one round trip at a time.

#### Scenario: Chatter inside the window is not re-posted
- **WHEN** an emitted workload's events keep matching within its refractory window
- **THEN** no further post is made for that id, and nothing the manager keeps
  differs from what its cooldown would have produced

#### Scenario: Recurrence past the window lands as a recurrence
- **WHEN** the workload's problem still produces events after the window closes
- **THEN** the adapter posts again under the same `initiationId` and the
  manager appends it to the open conversation
