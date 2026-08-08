## Context

`ensureAdapterWorkload` (`internal/controller/adapterworkload.go:144`) hardcodes

```go
replicas := int32(1)
deploy.Spec.Replicas = &replicas
```

for every adapter CR, with no reference to what the adapter serves. Both adapter
reconcilers call it the same way, so the behavior is uniform: **CR exists ⇒ pod
runs**.

Facts that shape the design:

| Fact | Consequence |
|---|---|
| A polling adapter at 0 replicas receives nothing | Sleeping a pull-based adapter makes it deaf, and no in-band signal can wake it |
| `OpQueue` is in-memory by design ("queue loss on restart is regenerated or safely dropped") | Pending ops cannot be a durable demand signal, and are invisible across manager replicas |
| `Conversation` has no terminal phase — `Idle`/`Queued`/`Working` only | "bound by a live conversation" would keep an adapter awake forever after its first conversation; it does not gate |
| An unclaimed `SignalSource` drops every signal before cooldown (`routeSignals` returns the drop reason first) | A signal adapter with no claimed source provably has no work |
| The adapter→served-CR reverse index already exists for `Served` (`channel_controller.go:107-120`) | The watch and mapping work is mostly present |
| The HTTP API is not leader-gated; reconcilers are | Demand must be computable from CR state, not from manager-process state |
| Credential projection rolls the pod on Channel/Source changes | Demand changes and projection changes converge through the same reconcile |

Requester decision carried in: implement both rules — L1 for channel adapters,
L2 for signal adapters.

## Goals / Non-Goals

**Goals:**
- An adapter CR that nothing uses costs nothing to declare.
- The sleeping state is legible: visible in `kubectl get deploy` and explained
  on the CR.
- Demand is a pure function of CR state, so any replica computes it identically.
- No adapter ever sleeps while something could still need it.

**Non-Goals:**
- Scaling above 1, or per-Channel workload sharding.
- Deleting workloads rather than scaling them.
- Changing `Served`, credential projection, token derivation, or the contracts.
- A CRD field to opt out — demand is derived, not declared (see Open Questions).

## Decisions

### D1. Demand is asymmetric because deafness is asymmetric

```
ChannelAdapter   active ⟺ ∃ Channel where spec.adapter == <CR name>
SignalAdapter    active ⟺ ∃ SignalSource where spec.adapter == <CR name>
                          ∧ ∃ Ready Pipeline claiming that source
```

The tempting uniform rule — require a Pipeline for both — is wrong for channels
in a way that only shows up at runtime. `channel-telegram` polls `getUpdates`;
at zero replicas the user's message never reaches the manager, so there is no op
to enqueue and nothing to wake it. Delete a Pipeline out from under live
conversations and every reply into those topics would vanish silently.

Signal adapters have no such hazard: an unclaimed source's signals are dropped
by `routeSignals` before anything else happens, so a sleeping adapter and a
running one produce identical observable behavior — minus a pod.

Rejected: **wake-on-op** (scale up when the OpQueue enqueues for a sleeping
adapter). It fails for exactly the case it was invented for — a pull-based
adapter can't be woken by traffic it never received — and it depends on
in-memory queue state that a non-leader replica may hold.

Rejected: **live-Conversation demand**. Without a terminal phase, every channel
that ever hosted a conversation stays permanently in demand, so the gate never
gates. Adding a terminal phase to `Conversation` to serve a scaling decision
would be the tail wagging the dog.

### D2. Scale to zero; never delete

`replicas: 0` keeps the Deployment, its ServiceAccount, and any Service in
place. Three reasons over deletion:

- **Legibility.** `kubectl get deploy` shows `0/0`, not absence. "Where did my
  adapter go?" is the worst failure mode for a feature whose whole job is to
  make things disappear.
- **No ownership churn.** Credential projection, `ownerRef`, and the Service
  path in `ensureAdapterService` stay exactly as they are; only one integer
  moves.
- **Fast wake.** Scaling 0→1 skips object creation and admission entirely.

### D3. `Active` is a new condition; `Served` keeps its meaning

They answer different questions:

| Condition | On | Question |
|---|---|---|
| `Served` | Channel / SignalSource | is there an implementation for my type? |
| `Active` | ChannelAdapter / SignalAdapter | is my workload running, and if not why? |

Reasons: `Active=True/HasDemand`; `Active=False/NoServedChannels`;
`Active=False/NoWiredSources` (sources exist, none claimed — the vm-bundle
case, worth its own reason so the message can name the missing Pipeline).

Deliberately *not* folded into `Served`: a Channel whose adapter is asleep
because the Channel was just created is momentarily unserved-looking, and
overloading `Served` would make a transient scale-up read as a configuration
error.

### D4. Scale-down waits; scale-up does not

Scale-up on the reconcile that first observes demand. Scale-down only after
demand has been absent for an idle grace period (manager env, default on the
order of minutes). Asymmetry matches the cost profile: a late scale-up delays
real work, a hasty scale-down thrashes a Deployment during ordinary edits —
`kubectl apply` of a Channel set, a Pipeline being rewired, a bundle upgrade
replacing CRs.

Implementation is a requeue-after rather than stored state: when demand is
absent and the workload is still at 1, record the observation time in the
`Active` condition's `lastTransitionTime` and requeue; scale down once the
grace period has elapsed. No new persistence, and the countdown is visible on
the CR.

### D5. Demand is computed from CR state only

Both reconcilers `List` the served kind in their namespace and count matches on
`spec.adapter`; the signal side additionally intersects with Ready Pipelines
using the existing `PipelineForSource` semantics. Nothing consults the OpQueue,
conversation state, or any in-process bookkeeping — so every manager replica,
leader or not, computes the same answer, and a reconcile after restart
converges without warm-up.

### D6. Watches mirror the existing `Served` mapping

`ChannelAdapter` gains a `Channel` watch; `SignalAdapter` gains `SignalSource`
and `Pipeline` watches, each mapping back to the adapters whose demand could
have changed. The inverse direction already exists for `Served`, so this is the
same map function pointed the other way — and the Pipeline watch can requeue all
adapters in the namespace, as `pipeline_controller.go` already does for
referenced kinds ("referenced kinds are few and pipelines fewer").

### D7. The telegram trio is gated per component, not as a unit

With `chat-signal-origination`, one Telegram surface becomes three workloads.
Their gates differ and must not be collapsed:

| Component | Kind | Sleeps when |
|---|---|---|
| `telegram-router` | SignalAdapter | its source is unclaimed — but it is the only poller, so this is the one to think hard about |
| `signal-telegram` | SignalAdapter | the chat source is unclaimed |
| `channel-telegram` | ChannelAdapter | no Channel names it |

The router is the component that hears everything, including replies destined
for `channel-telegram`. If the router sleeps, the channel adapter goes deaf
even though its own gate says active. So the router's demand SHALL additionally
include "a Channel names the channel adapter it forwards to" — i.e. the router
is active if *either* side has demand. This is the one place the two rules must
be OR-ed rather than applied independently, and it is a consequence of the
router being a shared front door.

## Risks / Trade-offs

- [A polling channel adapter sleeps and goes deaf] → Structurally prevented by
  D1: channel adapters use the Channel-exists gate, which cannot be false while
  a surface exists to type into. Pinned by a test that a Channel with no
  Pipeline keeps its adapter running.
- [The telegram router sleeps and takes the channel adapter's ears with it] →
  D7's OR-ed gate. This is the subtlest failure in the change and deserves its
  own integration test, not just a unit test.
- [Cold-start latency on first use after wiring] → Scale-up is immediate on the
  reconcile that observes demand, and a newly wired source is a CR write that
  triggers one. Worst case is pod start (~5-10s) before the first signal is
  accepted; for a source that was just claimed, nothing was flowing anyway.
- [Deployment thrash during bulk edits] → D4's grace period, with the countdown
  visible via `lastTransitionTime`.
- [Existing tests assert "adapter CR ⇒ Deployment"] → They change meaning.
  Updating them is a task, not a side effect, so the semantics change is
  recorded rather than absorbed.
- [Someone wants an adapter warm before wiring it] → No opt-out is offered
  initially (see Open Questions). The workaround is to create the served CR,
  which is also the honest way to express "I intend to use this".
- [A hand-deployed adapter is unaffected and may look inconsistent] → Correct
  and intended: the operator only manages workloads it owns.

## Migration Plan

Behavior-only; no CRD change, no manifest change, no data migration.

1. Ship the demand computation and `Active` condition **reporting only** —
   replicas still pinned at 1. Confirms the demand signal is right on real
   clusters without changing behavior.
2. Enable scale-to-zero with the grace period, defaulting to a conservative
   value.
3. Update the integration tests that assert unconditional Deployment creation.

Rollback: revert to the hardcoded `replicas := int32(1)`. Sleeping adapters
scale straight back up on the next reconcile; nothing else is stateful.

## Open Questions

- **Should an adapter be able to opt out of sleeping?** A `spec.alwaysRun` knob
  would fit the "workload knobs" allowance on adapter CRs, but every use case
  found so far is better expressed by creating the served CR. Deferred until
  someone hits a real one.
- **Should `Active=False/NoWiredSources` name the fix?** The message can name
  the unclaimed sources, which is the vm-bundle diagnostic ("adapter idle:
  source `alerts` exists but no Ready Pipeline claims it"). Cheap and probably
  worth doing; listed as a task, called out here because message quality is the
  whole value of the condition.
- Whether the idle grace period belongs in manager env (chosen) or as a
  per-adapter field. Env keeps CRD surface flat and matches how
  `MAX_RUNTIMES`/idle eviction are already configured.
