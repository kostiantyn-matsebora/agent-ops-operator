# Design: smart-k8s-events

## Context

`signal-k8s-events` today is a pure normalizer: it watches core `v1` Events, applies exact-match `severities`/`namespaces`/`includeReasons`/`excludeReasons` filters, and posts every survivor to `/signal/inbound`. Everything downstream is manager-side — `Cooldown.Fresh(fingerprint)`, `Signature(labels, signatureLabels)`, 7-day window reuse in `routeSignalGroup`.

That division is correct and this change preserves it. What it does not survive is the *input*: Kubernetes Events are point-in-time facts about objects whose lifecycle is churn by design. The adapter emits on first sight because it has nothing else to go on — it sees an event, not a state.

Three concrete failures follow, all observed:

```
1. per-pod signature       [namespace, kind, name] where name = pod name
                           → conversations scale with pods × rollouts
                           → 7d window reuse never fires (signature never repeats)

2. rollout churn           Unhealthy on a pod 20s from Ready is indistinguishable
                           from Unhealthy on a pod that will never be Ready

3. self-reference loop     Conversation → runtime pod → pod fails → Warning event
                           → signal → Conversation → …   (new pod name each turn,
                           so cooldown, workload grouping, and even a correct
                           liveness re-check all pass it through)
```

Constraints that shape every decision below:

- **The operator grants adapters nothing.** New RBAC is a chart-bound grant against `agentops-signal-<name>`, so every added verb is a visible, deliberate cost.
- **Adapters are dependency-free modules.** No client-go, no informer library — raw `net/http` list/watch, the technique `kube.go` already uses for Events.
- **Grouping stays manager-side** (`SignalSource.spec.grouping`). Suppression is not grouping; the adapter has always filtered.
- **`spec.config` is opaque to the manager.** Whatever shape this config takes, the manager never parses it; the adapter validates and reports via the source's Ready condition.

## Goals / Non-Goals

**Goals**

- Make self-ingestion structurally impossible, with no single point of failure.
- Give events a duration, so ordinary rollout churn never reaches an agent.
- Collapse conversations to one per *workload problem*, stable across rollouts.
- Express configuration in vocabulary a Prometheus/Alertmanager operator already knows, without misusing its terms.
- Keep the adapter dependency-free and the manager untouched.

**Non-Goals**

- Silences (runtime API + persisted state).
- `send_resolved` / auto-close (needs a manager-side input lane).
- A generic per-source conversation-creation cap — that is `signal-circuit-breaker`.
- Replacing manager-side grouping, cooldown, or window reuse.
- Talking to a real Alertmanager instance (considered and rejected — see Decision 1).

## Decisions

### Decision 1 — Alertmanager-*shaped* config in the adapter, not a real Alertmanager

**Chosen:** the adapter interprets Alertmanager-shaped config in `spec.config`.

**Alternatives considered:**

- *Generalize `GroupingSpec` manager-side* so `group_wait`/`repeat_interval`/`inhibitRules` apply to every source type. Rejected as the primary answer: the manager holds labels, not the cluster. It can never answer "is this Deployment mid-rollout" or "is this pod Ready now", which is the entire rollout-noise problem. Parts of this survive as the separate `signal-circuit-breaker` change.
- *Post events into a real Alertmanager* (`POST /api/v2/alerts`) and receive them back through the existing `signal-vmalertmanager` adapter. Genuinely attractive — inhibit rules, silences, and a UI for free, and `vm-bundle` already speaks this vocabulary. Rejected because it makes a hard runtime dependency on an Alertmanager the user may not run, adds an out-and-back round trip, and **still does not deliver `for:`** — alerts would need a self-expiring TTL and a liveness decision to produce them, which is the dwell logic again in a different coat.

### Decision 2 — Config is Prometheus + Alertmanager, split, and dwell is never called `group_wait`

```
Prometheus                          Alertmanager
──────────                          ────────────
expr + for:                         group_by / group_wait / group_interval
"has it held?"                      repeat_interval / inhibit_rules / silences
"is it STILL true?"                 "how do I notify about a SET of alerts?"
   │                                    │
   ▼                                    ▼
config.rules   (adapter)            config.route  (adapter: inhibit, matchers)
                                    spec.grouping (manager: signature, cooldown, window)
```

Alertmanager's `group_wait` holds a group briefly so the *first* notification batches more alerts of that group. It is not `for:`, and `for:` does not exist in Alertmanager at all — it lives in the Prometheus rule. Spelling dwell as `group_wait` would be an Alertmanager term meaning something Alertmanager does not mean, and the first reader who knows Alertmanager would be permanently confused.

Shape:

```yaml
config:
  namespaces: []                # watch scope — RBAC-shaped, unchanged

  rules:                        # ordered; first match wins
    - matchers: ['reason=~"Pulling|SandboxChanged|Preempting"']
      action: drop
    - matchers: ['reason="Unhealthy"']
      for: 10m                  # probes flap; only a long-held one is real
    - matchers: ['reason=~"NodeNotReady|VolumeFailedDelete"']
      for: 0                    # waiting is never right for these
    - matchers: []              # catch-all
      for: 3m

  route:
    inhibitRules:
      - sourceMatchers: ['reason="NodeNotReady"']
        targetMatchers: ['reason=~"Unhealthy|FailedScheduling"']
        equal: [node]
```

`includeReasons`/`excludeReasons` stay accepted and translate to a leading `action: drop` / trailing catch-all pair, so existing sources keep working unchanged.

### Decision 3 — Dwell re-checks the live object, via a pod informer

```
event matches rule ──► pending[object, reason] (coalesce; count++, track pods)
                              │
                          wait `for`
                              │
                   object still exists? ──no──► DROP   (terminating pod, rollout)
                              │ yes
                   object still unhealthy? ─no──► DROP  (new pod became Ready)
                              │ yes
                            EMIT once, enriched
```

**Alternatives for the re-check:**

| approach | new RBAC | why not |
|---|---|---|
| `GET pod/<name>` on demand after dwell | `get pods` (+ owner kinds) | N API calls per burst; still needs owner reads for enrichment anyway |
| **pod + replicaset informer** (chosen) | `list,watch pods,replicasets` | — |
| watch for a recovery event (`Started`, `Pulled`) during dwell | **none** | tempting, but *absence of a recovery event ≠ still broken*. Also requires watching `Normal` events the adapter does not emit. Fragile in exactly the case that matters |

The informer wins because the data is needed twice: owner resolution for the `workload` label needs pod ownerReferences, and the dwell re-check needs pod status. One cache, one watch mechanism (the same raw list/watch `kube.go` already implements), and pod labels fall out for free — which is what makes `matchers: ['app.kubernetes.io/part-of="infra"']` expressible.

**Verification is a three-rung ladder, not one check.** An earlier draft of this design said "health predicate, else existence, else fail open" — which is wrong for the large set of objects that have no health predicate. An HPA whose `FailedGetResourceMetric` flaps still *exists*, so existence-only would emit every time metrics-server hiccups, and that reason is among the noisiest in a real cluster.

```
1. kind has a health predicate  → evaluate it
     Pod   phase, Ready condition, container waiting reason
           ∈ {CrashLoopBackOff, ImagePullBackOff, ErrImagePull}
     Node  Ready condition
     Job   Failed condition
     PVC   phase
2. object exists, no predicate  → DID THE EVENT RECUR during the window?
     recurred → still happening → emit
     silent   → it stopped → drop
3. existence undeterminable     → emit (fail open)
```

Rung 2 is the events-only technique rejected as the *primary* mechanism (absence of a recovery event is not proof of health) — but as a *fallback for kinds we cannot inspect* it is strictly better than the fail-open-always it replaces, because a controller with a live problem keeps re-emitting while a resolved one goes quiet. Rung 3 stays fail-open: silently dropping an object we cannot evaluate at all would convert an unknown into a nonexistent problem, which is the failure this change exists to prevent.

### Decision 3b — Past-tense reasons must never dwell

The liveness re-check assumes the reason describes an **ongoing state**. A large class of Kubernetes Warning reasons describes something that **already happened**, and for those the re-check is not merely useless but actively harmful: `OOMKilling` fires, the container restarts, and three minutes later the pod is `Ready` — so the re-check drops a real OOM kill because the *replacement* is healthy.

```
ongoing   BackOff · Unhealthy · FailedScheduling · FailedMount · Failed
          → re-check is meaningful; the object is still in the state

past      OOMKilling · SystemOOM · Evicted · BackoffLimitExceeded
          · DeadlineExceeded · VolumeFailedDelete
          → re-check finds a healthy successor and erases the incident
          → these MUST carry `for: 0`
```

This is the single easiest way to build a rule set that looks careful and silently loses the incidents that matter most. Any default rule set, and any rule an operator writes, must classify a reason as ongoing or past before choosing a dwell.

### Decision 3c — Breadth escalates past the dwell

A 10-minute dwell on `Unhealthy` is right for one flapping pod and wrong for an outage: if every replica of a Deployment goes unready, the service is down and nobody hears about it for ten minutes.

The pending entry already tracks distinct objects for the payload, so the fix is nearly free: a rule may declare `escalateAfterObjects` (default 3). When a pending entry accumulates that many distinct objects sharing a workload, it **shortens** the dwell to `max(dwell/4, 60s)`.

The reasoning the dwell rests on — *one pod misbehaving is churn* — stops applying the moment several do. Escalation is what lets the defaults use long dwells for flappy reasons without trading away detection of the case those reasons matter in.

**It shortens, it does not eliminate — corrected after a live false positive.** The first implementation made a broad entry due immediately, which fires on every ordinary rollout: three replicas starting together are all legitimately not-Ready for the first minute, so "three objects failing" is true and premature at the same time. A 45-second start on a real cluster opened a conversation about a healthy deployment. Breadth on its own cannot separate an outage from a rollout, because a rollout also makes every replica unready at once — only *duration* can, so escalation has to keep some. A quarter of the dwell, floored at a minute, turns the default 10m `Unhealthy` rule into 2m30s: four times faster for a real outage, still longer than a normal start.

### Decision 4 — `workload` by owner references, never by name munging

`Pod/app-7f9c8d4-xk2p9` → ownerRef → `ReplicaSet/app-7f9c8d4` → ownerRef → `Deployment/app`.

The tempting shortcut — strip two dash-separated segments — breaks on StatefulSets (`app-0`, one segment), DaemonSets (`app-xk2p9`, one segment), bare pods (zero), and any workload whose own name contains a hash-shaped segment. Rejected as a trap.

New default signature `[namespace, workload]` (not `[namespace, workload, alertname]`): "my app is broken" is one investigation whether it presents as `BackOff` or `Unhealthy`, and splitting by reason would re-introduce a smaller version of the same fan-out. Workload names are stable across rollouts, so the 7-day window reuse becomes live for the first time and an ongoing problem resumes its session as `InputRecurrence` rather than opening conversation N+1.

### Decision 5 — Self-exclusion is an invariant with three independent mechanisms

Not a deny-list entry. A deny-list is editable, and an editable loop breaker means a values typo can take out a cluster.

```
┌ owner/label rule ──── app.kubernetes.io/name ∈ agentops family, OR
│                       owner chain reaches a Conversation
│                       precise; needs the informer warm
├ name-prefix rule ──── agentops-conv- / agentops-adapter- / agentops-signal-
│                       crude; needs no pod read, so it holds during startup —
│                       exactly when a mass pod-creation failure is most likely
└ release-namespace ─── excluded by default, overridable
                        the only configurable one of the three
```

Three, not one, because `signal-circuit-breaker` is out of scope: with no backstop shipping alongside, these rules are load-bearing and a single cold cache must not re-open the cycle. The label/owner rule and the name-prefix rule are the belt and the braces; namespace exclusion is the default posture for people who do not co-locate apps with the operator.

The principle behind it, worth stating once: **agent-ops' own health is status, not signal.** The reconciler already holds the pod's failure; routing that knowledge back through the ingest pipeline to wake an agent is the architectural error, not merely a noisy one.

### Decision 6 — Burst coalescing changes the payload, not just the count

A pending entry accumulates occurrences rather than queueing N entries. What emits is strictly more informative than what N separate signals carried:

```
Deployment/app in "prod" — unhealthy 4m
  BackOff    ×23  (3 pods)  Back-off restarting failed container
  Unhealthy  × 8  (2 pods)  Readiness probe failed: connection refused
  first 14:03:11 · still true 14:07:42
```

One conversation, one agent run, full evidence. This is a *feature* of dwell, not a side effect: the window is exactly the interval over which evidence accumulates.

### Decision 7 — The emit cap is adapter-local and deliberately small

Per-source signals-per-minute ceiling; clipping is reported on the source's Ready condition, never silent. It does not pre-empt `signal-circuit-breaker` (which caps *conversation creation* across every source type) — it exists so that in the interval before that change lands, a fast runaway degrades to "a condition says something is wrong" rather than "etcd fills up".

### Decision 8 — The default rule set

Calibrated to two failure modes at once: never lose an actionable incident, never open a conversation for cluster churn. The ordering matters — first match wins.

```yaml
rules:
  # 1 ── never actionable on their own ─────────────────────────────────────
  - matchers: ['reason=~"ProbeWarning|SandboxChanged|Preempting|NodeNotSchedulable|ExternalProvisioning|FailedToUpdateEndpoint.*|FailedPreStopHook|FailedKillPod|ContainerGCFailed|ImageGCFailed"']
    action: drop

  # 2 ── PAST TENSE: re-checking would erase a real incident (Decision 3b) ──
  - matchers: ['reason=~"OOMKilling|SystemOOM|Evicted|BackoffLimitExceeded|DeadlineExceeded|VolumeFailedDelete"']
    for: 0

  # 3 ── node-level: immediately actionable, blast radius is the whole node ─
  - matchers: ['reason=~"NodeNotReady|NodeHasDiskPressure|NodeHasMemoryPressure|NodeHasPIDPressure"']
    for: 0

  # 4 ── flappy by nature: long dwell, but escalate on breadth ─────────────
  - matchers: ['reason=~"Unhealthy|FailedGetResourceMetric|FailedComputeMetricsReplicas"']
    for: 10m
    escalateAfterObjects: 3

  # 5 ── real, but resolve on their own timescale ─────────────────────────
  - matchers: ['reason=~"FailedScheduling|BackOff|FailedMount|FailedAttachVolume|FailedMapVolume|FailedBinding|ProvisioningFailed|SyncLoadBalancerFailed|FailedRescale"']
    for: 5m

  # 6 ── catch-all: everything unknown is KEPT, verified, and reported ─────
  - matchers: []
    for: 3m
```

**Why each tier is where it is**

| tier | reasons | rationale |
|---|---|---|
| 1 drop | `ProbeWarning` | probe *succeeded*, output was non-empty — informational by construction |
| | `SandboxChanged`, `FailedPreStopHook`, `FailedKillPod` | pod infra churn and teardown noise; if it matters, a downstream reason fires on the same object |
| | `Preempting`, `NodeNotSchedulable` | the scheduler and a cordoning operator doing exactly what they were told |
| | `FailedToUpdateEndpoint*`, `ExternalProvisioning` | controllers retrying on their own loop; self-healing by design |
| | `ContainerGCFailed`, `ImageGCFailed` | node housekeeping. Dropping the precursor is safe *because* the consequence (`NodeHasDiskPressure`) is tier 3 at `for: 0` |
| 2 `for: 0` | `OOMKilling`, `Evicted`, `BackoffLimitExceeded`, `DeadlineExceeded` | already happened. A dwell would find the healthy replacement and delete the evidence |
| 3 `for: 0` | node conditions | one event, many workloads affected; every minute of dwell is a minute of an unexplained multi-service outage |
| 4 `10m` | `Unhealthy` | the single noisiest reason in any cluster — every rollout, every slow start. A probe still failing at 10m is real; `escalateAfterObjects: 3` catches the whole-service case in seconds |
| | HPA metric reasons | metrics-server flaps constantly and recovers on its own; the HPA has no health predicate, so rung 2 (recurrence) decides |
| 5 `5m` | `FailedScheduling` | the cluster autoscaler needs 1–3 minutes to provision; firing sooner pages for a node that is already being built |
| | `BackOff` | a container that crashes twice and stabilizes is not an incident; one still in `CrashLoopBackOff` at 5m is |
| | volume reasons | RWO disks genuinely take minutes to detach from the old node during a rollout — a classic false positive |
| | `SyncLoadBalancerFailed`, `FailedRescale`, `ProvisioningFailed` | cloud API flakes that retry successfully |
| 6 `3m` | **everything else** | including `Failed`, `FailedCreate`, `FailedCreatePodSandBox`, and every reason a third-party operator invents |

**The catch-all is the "do not miss issues" guarantee.** Nothing is dropped for being unrecognized: a reason no one anticipated — a CRD controller's custom warning, a new reason in a future Kubernetes release — gets a 3-minute dwell, a verification pass, and a conversation if it is still true. The drop list is short, explicit, and each entry is justified by a downstream reason that would catch the same underlying problem. That asymmetry is deliberate: the cost of a wrong entry in tier 1 is a missed incident, and the cost of a wrong entry in tier 6 is one extra conversation.

**What the defaults do to the motivating case.** A ten-replica Deployment rolling out emits `Unhealthy` (tier 4) on starting pods and `FailedScheduling` (tier 5) on pods waiting for room. Every one of them either finds its pod gone or finds it Ready inside the dwell, so the rollout produces **zero** conversations. The same Deployment with a bad image emits `Failed` (tier 6) and `BackOff` (tier 5); both re-check as still-unhealthy, both resolve to the same `workload` signature, and they produce **one** conversation carrying both reasons and their counts.

## Risks / Trade-offs

- **[Dwell delays every alert by `for:`.]** A node loss or lost volume now takes minutes to reach an agent. → Default rules ship `for: 0` for the reasons where waiting is never right (`NodeNotReady`, volume failures, `BackoffLimitExceeded`); the catch-all dwell is 3m.
- **[The signature change is breaking.]** Existing per-pod conversations keep their old signature hash and go orphaned. → They age out of the 7-day window with no action needed; documented explicitly in `CHANGELOG.md` so it is not discovered live.
- **[Memory grows with pod count.]** A pod informer on a large cluster holds every pod. → Cache only the fields needed (owner refs, phase, Ready condition, waiting reasons, selected labels), not full pod objects; scope the watch to the same namespaces the events watch already covers.
- **[Wider RBAC.]** The adapter's SA gains `list`/`watch` on `pods` and `replicasets`. → Read-only, chart-bound against the deterministic SA name, and documented in the values file next to the grant. The operator still creates no RBAC.
- **[The reflexive loop is not closed.]** An agent with `k8s-admin` acts on the cluster, the cluster emits Warnings, and those wake it again — with nothing agentops-labelled anywhere in the cycle, so self-exclusion is blind to it. Dwell absorbs the benign case (a `rollout restart` whose pods recover), but a bad remediation that creates a genuinely broken state fires legitimately. → Accepted for this change; the adapter's emit cap limits the rate, and `signal-circuit-breaker` is the real answer.
- **[Matcher syntax is a parser.]** Alertmanager matcher syntax (`=`, `!=`, `=~`, `!~`, quoting) has to be implemented without dependencies. → Restrict to the documented four operators over a flat label map; reject anything else at config-validation time with the error on the source's Ready condition, where a config error is already reported today.
- **[Rules are ordered, first-match-wins — order is easy to get wrong.]** → Validation surfaces an unreachable rule (one shadowed entirely by an earlier matcher) as a warning on the Ready condition rather than failing the source.

## Migration Plan

1. Ship the adapter with the new config accepted **and** `includeReasons`/`excludeReasons` translated, so existing sources keep working on the new image before any values change.
2. Chart bump adds the pods/replicasets RBAC. Without it the adapter reports `Ready=False` with the exact missing grant rather than degrading silently.
3. Chart defaults flip `signatureLabels` to `[namespace, workload]` and install the default `rules`. **Breaking**, noted in `CHANGELOG.md`.
4. Rollback is a chart revert: the adapter image tolerates the old config, and the old signature labels are still valid input to the manager's unchanged grouping.

Available **today**, before any of this ships, as relief for a cluster currently being spammed:

```yaml
k8s-bundle:
  eventsAdapter:
    source:
      excludeReasons: [Unhealthy, FailedScheduling, SandboxChanged, Preempting]
      grouping:
        signatureLabels: [namespace, alertname]
```

Bounds conversations by namespaces × reasons instead of pods × rollouts. Agent *runs* do not drop (cooldown is still per object+reason, so N broken pods still queue N inputs), but the conversation explosion stops with a values edit.

## Open Questions

- ~~Should the pod informer be scoped to the same namespaces as the events watch, or run cluster-wide regardless?~~ **Resolved: scoped, sharing `desiredScopes()` with the events watch.** Not a preference in the end — the chart renders a namespaced Role when `rbac.clusterWide: false`, and the spec requires the pods/replicasets grant be namespaced identically. A cluster-wide pod watch would 403 outright in that configuration, so cluster-wide is not merely more expensive, it is broken for a supported install. The re-list on a `namespaces` change is the accepted cost.
- Are selected pod labels an allow-list (`labelKeys: [...]` in config) or all labels? All labels is simpler but puts arbitrary user labels into the signal's label map, which the manager then hashes into signatures if someone names them in `signatureLabels`.
- Does `inhibitRules` earn its place in v1, or does `rules` + dwell already cover the node-down case in practice (a down node's pods fail their liveness re-check as "gone or unhealthy" and fire anyway — inhibition is what stops that)? Worth validating against a real node drain before building it.
