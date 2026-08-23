# k8s-event-suppression Specification

## Purpose

Turning a cluster's Event stream into something worth answering.

Ordered first-match-wins rules in Alertmanager's matcher syntax, plus a TIME
axis spelled in Alertmanager's vocabulary — and the two halves that make it
honest: a matched event must still be TRUE after its dwell window, and a mute is
evaluated at EMIT so a problem that outlives the window still surfaces.

Everything else bounds the volume rather than the meaning: bursts coalesce into
one enriched signal, inhibition suppresses the consequences of a known cause, and
a per-source emit cap REPORTS its clipping rather than quietly dropping.
## Requirements
### Requirement: Suppression rules are ordered, first-match-wins, and use Alertmanager matcher syntax
A source's `config.rules` SHALL be an ordered list evaluated first-match-wins. Each rule carries `matchers` (Alertmanager syntax over the signal's label set: `=`, `!=`, `=~`, `!~`, with quoted values), an optional `for` duration, and an optional `action` (`drop`). A rule with empty `matchers` is a catch-all. A rule with `action: drop` suppresses the event outright; otherwise the event enters the dwell queue for `for` (default 0 = emit immediately).

Only those four operators SHALL be supported. Unsupported syntax, an unparsable duration, or a malformed rule SHALL be reported as a False `Ready` condition on that source through the contract status API, leaving other sources served — the existing behavior for invalid config.

A rule shadowed entirely by an earlier rule SHALL be reported as a warning on the source's `Ready` condition without failing the source, because ordering mistakes are silent and common.

The legacy `includeReasons` and `excludeReasons` fields SHALL remain accepted and SHALL translate to equivalent rules, so sources written against the previous config keep working unchanged.

#### Scenario: A drop rule suppresses bookkeeping noise
- **WHEN** a source's rules begin with `matchers: ['reason=~"Pulling|SandboxChanged"'], action: drop` and such an event arrives
- **THEN** no signal is emitted and no later rule is consulted

#### Scenario: The catch-all applies to unmatched events
- **WHEN** an event matches no earlier rule and the final rule has empty `matchers` and `for: 3m`
- **THEN** the event is held for three minutes and re-checked before emitting

#### Scenario: Invalid matcher syntax surfaces on the source
- **WHEN** a rule uses an operator outside `=`, `!=`, `=~`, `!~`
- **THEN** the adapter sets a False Ready condition naming the offending rule, and other sources keep producing signals

#### Scenario: Legacy reason filters keep working
- **WHEN** a source declares only `excludeReasons: ["Unhealthy"]` and no `rules`
- **THEN** `Unhealthy` events are dropped and all other Warning events are emitted, exactly as before this change

### Requirement: A matched event must still be true after its dwell window
An event matched by a rule with `for > 0` SHALL be held and re-checked against the live involved object before emitting. The adapter SHALL drop the event when the object no longer exists, and drop it when the object exists and is healthy. Only an object that is still unhealthy at the end of the window SHALL produce a signal.

Verification SHALL proceed as a three-rung ladder:

1. **The kind has a health predicate** — evaluate it. **Pod** is the only kind that SHALL carry one: phase, `Ready` condition, and container waiting reasons (`CrashLoopBackOff`, `ImagePullBackOff`, `ErrImagePull`, `CreateContainerConfigError` and siblings). A container merely being created SHALL NOT count as unhealthy.

   Node, Job and PersistentVolumeClaim deliberately do NOT carry predicates, and fall to rung 2. Giving them predicates would mean watching three more resources — three more external RBAC grants — for a path the shipped defaults never take: the reasons that concern those kinds (`NodeNotReady`, node pressure conditions, `BackoffLimitExceeded`, `DeadlineExceeded`, `VolumeFailedDelete`) all carry `for: 0` and so are never verified at all. Rung 2 is sound for them, since a controller with a live problem keeps re-emitting.
2. **The object exists but has no health predicate** — emit only if the event RECURRED during the window; drop if it went silent. A controller with a live problem keeps re-emitting; a resolved one stops.
3. **Existence cannot be determined** — **emit**, failing open. Silently dropping an object the adapter cannot evaluate at all would convert an unknown into a nonexistent problem, which is the failure this capability exists to prevent.

Rung 2 SHALL NOT be an existence check. An object with no health predicate that still exists is not evidence of an ongoing problem — an autoscaler whose metric lookup flapped once still exists, and treating existence as confirmation would emit on every transient failure of every uninspectable kind.

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

### Requirement: Reasons describing a completed event never dwell
A rule SHALL NOT apply a non-zero dwell to a reason that reports something that has ALREADY happened rather than an ongoing state. For such reasons the liveness re-check is not merely uninformative but destructive: the object recovers or is replaced, the re-check finds it healthy, and a real incident is erased.

At minimum `OOMKilling`, `SystemOOM`, `BackoffLimitExceeded`, `DeadlineExceeded`, and `VolumeFailedDelete` SHALL carry `for: 0` in any shipped default rule set.

`Evicted` is past-tense too, and is therefore NEVER given a dwell — but the shipped default DROPS it rather than emitting it at `for: 0`, because an eviction is reported from the cause end by node pressure (one node-level signal instead of one per displaced pod) and from the consequence end by `FailedScheduling` (the pod that does not come back), while an API-initiated eviction is a drain and is routine. This is an application of the drop rule below, not an exception to this one: a rule set MUST NOT give `Evicted` a non-zero dwell in either case.

#### Scenario: An eviction is not given a dwell in any shipped default
- **WHEN** the shipped default rule set is rendered
- **THEN** `Evicted` is either dropped or carries `for: 0`, and never a non-zero dwell

#### Scenario: An OOM kill is not erased by the container's recovery
- **WHEN** a container is OOM-killed, restarts, and is Ready well within any dwell window
- **THEN** a signal is emitted for the OOM kill, because the reason carries `for: 0` and is never re-checked

#### Scenario: A completed Job failure survives
- **WHEN** a Job exceeds its backoff limit and its pods are subsequently cleaned up
- **THEN** a signal is emitted, rather than being dropped because the pods no longer exist

### Requirement: Default rules keep unknown reasons
A shipped default rule set SHALL end in a catch-all that assigns a dwell rather than a drop, so that a reason the rule set does not anticipate — a third-party controller's warning, a reason added in a future Kubernetes release — is verified and reported rather than discarded.

Reasons SHALL be dropped outright only where the underlying problem would still be caught by another reason that is not dropped. The asymmetry is deliberate: a wrong entry in the drop list costs a missed incident, while a wrong entry in the catch-all costs one surplus conversation.

#### Scenario: An unanticipated reason still reaches an agent
- **WHEN** a custom controller emits a Warning reason that appears in no rule
- **THEN** the catch-all rule applies its dwell, the object is verified, and a signal is emitted if the problem is still true

#### Scenario: A dropped precursor is covered by its consequence
- **WHEN** the default rules drop a node housekeeping reason such as an image-GC failure
- **THEN** the condition it precedes, such as node disk pressure, is not dropped and carries `for: 0`

### Requirement: A burst coalesces into one enriched signal
While an event is pending in the dwell queue, further events for the same involved object SHALL coalesce into that pending entry rather than queueing separately. The emitted signal's payload SHALL carry the accumulated evidence: per-reason occurrence counts, the number of distinct objects involved, the first-seen timestamp, and the time the problem was confirmed still true.

#### Scenario: Twenty-seven events become one signal
- **WHEN** three pods of one Deployment emit 23 `BackOff` and 8 `Unhealthy` events during a 3-minute dwell window and the workload is still unhealthy at the end
- **THEN** one signal is emitted whose payload reports both reasons with their counts, the pod count, the first-seen time, and the confirmation time

#### Scenario: Coalescing does not extend the window
- **WHEN** new events for a pending object keep arriving throughout the dwell window
- **THEN** the window still ends `for` after the FIRST matched event, not after the most recent one

### Requirement: Inhibition suppresses consequences of a known cause
`config.route.inhibitRules` SHALL suppress a matched event when another event matching the rule's `sourceMatchers` is active and every label named in `equal` has the same value on both. Inhibition SHALL be evaluated before the dwell queue, so an inhibited event never occupies the queue.

#### Scenario: A down node does not page for each of its pods
- **WHEN** an inhibit rule declares `sourceMatchers: ['reason="NodeNotReady"']`, `targetMatchers: ['reason=~"Unhealthy|FailedScheduling"']`, `equal: [node]`, a node reports NotReady, and its pods emit `Unhealthy`
- **THEN** only the node signal is emitted and the pod events are suppressed

#### Scenario: Inhibition is scoped by equal labels
- **WHEN** the same inhibit rule is active and a pod on a DIFFERENT, healthy node emits `Unhealthy`
- **THEN** that event is not inhibited and follows its ordinary rule

### Requirement: A source's emit rate is capped and clipping is reported
The adapter SHALL cap the number of signals it emits per source per minute. When the cap is reached it SHALL stop emitting for the remainder of the window and report the clipping on that source's `Ready` condition, naming the count. Clipping SHALL NOT be silent.

This cap is adapter-local and bounded in ambition: it turns a fast runaway into a visible condition rather than an unbounded write load. It does not replace a manager-side limit on conversation creation.

#### Scenario: A runaway becomes a visible condition
- **WHEN** matched events would produce more signals than the per-minute cap for a source
- **THEN** emission stops for that window and the source's Ready condition reports how many signals were clipped

#### Scenario: Normal volume is unaffected
- **WHEN** a source emits fewer signals than its cap
- **THEN** no condition change is made and every signal is delivered

### Requirement: Suppression has a time axis, spelled in Alertmanager's vocabulary
The adapter SHALL support named time intervals and mute references on a source's `route`, using Alertmanager's schema and field names — intervals of `times` (`startTime`/`endTime`), `weekdays`, `daysOfMonth`, `months`, `years` and a `location`, referenced by name from `muteTimeIntervals`.

The names and semantics SHALL be borrowed exactly rather than approximated. `rules` speaks Prometheus and `route` speaks Alertmanager, and a concept already specified in the half it belongs to SHALL NOT be re-spelled: a familiar word meaning something slightly different is worse than an unfamiliar one.

`location` SHALL be an IANA timezone. A window expressed without one is interpreted as UTC and will drift by an hour at each daylight-saving transition — stopping cover of the outage it was written for, on a date nobody chose.

Overlapping intervals SHALL union: an event is muted when any referenced interval matches.

#### Scenario: A nightly maintenance window is expressible
- **WHEN** a source declares an interval of 04:00–04:20 on all weekdays in a named location and references it from `muteTimeIntervals`
- **THEN** events that would emit inside that local window are muted

#### Scenario: Local time is honoured across a DST transition
- **WHEN** a window names an IANA location and the zone's offset changes
- **THEN** the window still covers the same local clock times

#### Scenario: An unparseable interval fails the source loudly
- **WHEN** a source declares a time interval that cannot be parsed
- **THEN** the source reports it on its Ready condition with the reason, rather than ignoring the entry and leaving a window that never fires

### Requirement: Mute is evaluated at emit, so a persistent problem still surfaces
Muting SHALL be evaluated after the dwell window and before emission, mirroring Alertmanager's mute intervals, which mute notifications rather than evaluation.

A matched event whose problem does not outlive the window SHALL be muted and discarded. A problem that persists past the window SHALL still be reported, because the cluster continues producing events for anything genuinely broken and the next one dwells and emits normally once the window has closed.

Muted events SHALL NOT count against the source's emit cap: they were never emitted, and charging them would make a mute window read as a runaway.

#### Scenario: Transient noise inside the window is dropped
- **WHEN** a scheduled outage produces events that stop when it ends
- **THEN** no signal is emitted for them

#### Scenario: A problem outliving the window is reported after it
- **WHEN** a failure begins during a mute window and is still producing events after it closes
- **THEN** a signal is emitted once a post-window event completes its dwell

#### Scenario: An event confirmed inside the window is muted
- **WHEN** an event arrives shortly before the window and its dwell completes inside it
- **THEN** it is muted, because the request was not to be told during the window

#### Scenario: Muting does not consume the emit budget
- **WHEN** a window mutes many events
- **THEN** the source's emit cap is unaffected and no clipping is reported for them

### Requirement: A mute window may be narrowed by matchers
A `muteTimeIntervals` entry MAY carry `matchers`, restricting what the window silences. An entry with no matchers SHALL mute everything from that source for the interval's duration.

Going deaf for the length of the window is the principal hazard of this feature. Narrowing it to the reasons a scheduled outage actually produces SHALL be available, so the safe configuration does not depend on the window being short or on anyone reviewing it later.

#### Scenario: A window silences the outage, not everything
- **WHEN** a window carries matchers selecting connectivity-related reasons
- **THEN** those events are muted inside the window and an unrelated reason still emits

#### Scenario: A window without matchers mutes the source
- **WHEN** a window declares no matchers
- **THEN** every event from that source is muted for its duration

### Requirement: An active mute is visible on the source
While a mute window is active the adapter SHALL report it on that source's `Ready` condition, naming the interval, and SHALL report how many events were muted.

Muting SHALL NOT be silent. A muted lane and an idle lane are indistinguishable from outside and only one of them means the cluster is healthy, so "why has nothing arrived since four?" SHALL be answerable from the source object rather than from adapter logs.

#### Scenario: Silence is explained
- **WHEN** a mute window is active
- **THEN** the source's Ready condition names the interval that is muting it

#### Scenario: The cost of the window is reported
- **WHEN** a mute window closes
- **THEN** the number of events it muted is reported, so an over-broad window is visible rather than inferred

