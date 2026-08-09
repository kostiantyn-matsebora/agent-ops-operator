# Proposal: smart-k8s-events

## Why

The cluster-events lane creates **hundreds of conversations** on a healthy cluster. Two causes compound:

1. **The signature is per-pod.** `k8s-bundle` defaults `grouping.signatureLabels` to `[namespace, kind, name]`, and `name` is a *pod* name — unique per replica and regenerated on every rollout. Conversation count therefore scales with **pods × rollouts**, and the 7-day window reuse can never fire for a pod because the signature never repeats.
2. **A Kubernetes Event is a point-in-time fact, not a state with a duration.** The adapter emits on the first occurrence, so ordinary rollout churn — a readiness probe failing on a pod that is 20 seconds from Ready, a `FailedScheduling` that resolves on the next scheduler pass, a terminating pod on its way out — is indistinguishable from a real outage.

Worse, the lane **feeds itself**. A runtime pod that cannot start (quota, unschedulable, `ErrImagePull`, OOM) emits a Warning event on `Pod/agentops-conv-<name>`; that becomes a signal; the manager opens a Conversation; the reconciler creates another runtime pod; it fails the same way under a *new* pod name. Every defense in this proposal's first half fails to break that cycle — the fingerprint is fresh (new pod name), the workload is fresh (owner is the Conversation CR), and the liveness re-check fires *correctly* because the pod really is still broken. `MAX_RUNTIMES` caps concurrent pods but not Conversation creation, so the runaway fills etcd while eight pods thrash.

This is the signal-lane twin of the existing **no relay loops** invariant: the system must never re-ingest its own output as input. agent-ops' own health is *status*, not *signal* — the reconciler already holds the pod's failure; routing it back around through the event pipeline to wake an agent is the architectural error.

## What Changes

### Self-reference is structurally impossible (the loop breaker)

- The adapter SHALL drop every event whose involved object is agent-ops' own machinery, via **three independent mechanisms** so that no single cold cache or values edit can re-open the cycle:
  - **owner/label rule** — the object carries `app.kubernetes.io/name` in the agentops family (`agentops-runtime`, `agentops-manager`, adapter workloads), or its owner chain reaches a `Conversation`;
  - **name-prefix rule** — `agentops-conv-`, `agentops-adapter-`, `agentops-signal-`. Requires no pod read, so it holds during adapter startup before the informer is warm — exactly when a mass pod-creation failure is most likely in flight;
  - **release-namespace exclusion**, on by default and overridable (the only one of the three that is configurable).
- The first two are **not configurable and not deny-list entries**. A deny-list is editable, and an editable loop breaker means a values typo can take out a cluster.

### Events acquire a duration (`for:`) — the rollout-noise killer

- Per-source config gains a **`rules`** stanza: ordered rules of `matchers` + `for` (dwell) + `action`. A matched event is held for `for`, then **re-checked against the live object** before emitting:
  - object gone → **drop** (it was churn — the terminating pod of a rollout);
  - object recovered → **drop** (the new pod became Ready);
  - still unhealthy → **emit once**, enriched with the occurrence count and the distinct reasons seen during the window.
  A healthy rollout dies on both branches; a broken one still fires. `for: 0` stays available for reasons where waiting is never right (node lost, volume lost).
- Health predicates are **kind-specific** (Pod, Node, Job, PVC first); every other kind gets an existence check only. When health is unverifiable the adapter **fails open** (emits), because a silent drop on an unrecognized kind is worse than a false positive.
- Dwell **coalesces** a burst into one enriched signal instead of N signals. A 3-pod, 27-event flap becomes one conversation carrying `BackOff ×23 (3 pods) / Unhealthy ×8 (2 pods), first seen …, still true at …` — strictly more context than 27 conversations ever carried.

### Config speaks Prometheus + Alertmanager, split honestly

- **`rules`** is the Prometheus half (what counts as a problem, and for how long it must hold). **`route`** is the Alertmanager half (`inhibitRules`, matchers). Dwell is deliberately **not** spelled `group_wait`: Alertmanager's `group_wait` batches a group before its first notification and is not `for:`. Spelling one as the other would ship a lie in Alertmanager's clothing.
- Matchers use Alertmanager syntax (`reason=~"BackOff|Failed"`, `namespace!="dev"`) over the signal's label set, replacing the exact-match `includeReasons`/`excludeReasons` pair (which remain accepted and are translated).
- **Deny-list posture** (decided): everything `Warning` is a candidate unless a rule drops it. Defaults ship a curated drop list of pure-bookkeeping reasons plus dwell values for the known-flappy ones. Nothing is missed by default; noise is removed by rule, not by allow-list.

### Signals carry workload identity

- The adapter enriches labels with **`workload`** (`Deployment/app`, resolved Pod → ReplicaSet → Deployment through owner references), **`node`**, and selected pod labels — the last of which is what makes `matchers: ['app.kubernetes.io/part-of="infra"']` expressible at all.
- Resolution uses a **pod + replicaset informer** (list/watch), not per-event GETs: it is the same watch machinery the adapter already runs, and it serves both enrichment and the dwell re-check from one cache. String-munging a pod name into its workload is explicitly rejected — it breaks on StatefulSets, DaemonSets, and bare pods.
- **BREAKING**: `k8s-bundle` changes its default `grouping.signatureLabels` from `[namespace, kind, name]` to `[namespace, workload]`. Existing per-pod conversations keep their old signature hash and go orphaned (they age out of the 7-day window); new conversations group per workload. Upgrade note required.

### A local emit cap, not a circuit breaker

- The adapter caps signals per source per minute, clipping with a reported condition rather than silently dropping. This is deliberately **local to this adapter** and small: the general, all-source circuit breaker is the separate `signal-circuit-breaker` change. It turns a fast runaway from "etcd fills up" into "a condition says something is wrong" in the interval before that change lands.

### Out of scope

- **Silences** — a runtime create/expire/list API and persisted state in an adapter that today persists one cursor string. `rules` covers most of the need.
- **`send_resolved`** — there is no resolved input lane (`InputAlert`/`InputJob`/`InputTask`/`InputRecurrence`), so it drags in a manager-side change.
- **Manager-side circuit breaker (L3)** — separate change `signal-circuit-breaker`. Consequence accepted: until it lands, the self-exclusion rules are load-bearing, which is why there are three of them.

## Capabilities

### New Capabilities

- `k8s-event-suppression`: the rules engine — Alertmanager-syntax matchers, `for:` dwell with kind-specific liveness re-check, burst coalescing into one enriched signal, inhibit rules, drop actions, and the fail-open rule for unverifiable kinds.
- `signal-self-exclusion`: the invariant that agent-ops never re-ingests its own machinery as a signal — the three independent detection mechanisms, their non-configurability, and the rationale linking it to the existing no-relay-loops invariant.

### Modified Capabilities

- `k8s-events-signal-adapter`: config grows `rules`/`route` (with `includeReasons`/`excludeReasons` translated for compatibility); emitted labels grow `workload`, `node`, and selected pod labels; the adapter gains a pod+replicaset informer and therefore new RBAC; the "no dedup, no grouping, no cooldown" requirement is amended — the adapter still performs no *grouping* (that stays manager-side) but now performs *suppression*, which is filtering and has always been its job.
- `k8s-bundle`: the events component's default `grouping.signatureLabels` becomes `[namespace, workload]` (**BREAKING**); default `rules` replace the empty `includeReasons`/`excludeReasons`; the adapter's RBAC grows `list`/`watch` on `pods` and `replicasets`, and the values surface documents why.

## Impact

- `signal-k8s-events/`: new rules engine + matcher parser, dwell queue with coalescing, pod/replicaset informer and cache, owner resolution, self-exclusion predicates, emit cap. Module stays dependency-free.
- `chart/charts/k8s-bundle/`: `values.yaml` defaults (signature labels, rules, RBAC), `templates/events.yaml` (RBAC rules, `configSchema` for the new config shape).
- **No manager changes.** Grouping, cooldown, window reuse, and recurrence stay exactly where they are; the only manager-visible difference is a richer label set on incoming signals.
- **RBAC widens** for the adapter's SA (`agentops-signal-k8s-events`): adds `list`/`watch` on `pods` and `replicasets`. Bound by the chart against the deterministic SA name, as always — the operator still grants adapters nothing.
- Docs: `docs/k8s-bundle.md` (values + rules cookbook), `docs/concepts.md` (label vocabulary), `CHANGELOG.md` (the signature-labels break), `CLAUDE.md` (self-exclusion as an invariant alongside no-relay-loops).
- Relation to `signal-circuit-breaker`: that change adds the generic per-source conversation-creation cap. Independent — no code conflict, and this change's emit cap is adapter-local and does not pre-empt it.
