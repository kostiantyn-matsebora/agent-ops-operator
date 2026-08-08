# Adapter workloads run on demand, not on declaration

## Why

`ensureAdapterWorkload` renders a Deployment with `replicas: 1` for every
adapter CR that exists, unconditionally. Nothing asks whether anything is
actually served. An adapter CR is a *declaration that an implementation is
available*, and the reconciler treats it as a *demand that it run*.

The chart makes this concrete: `vm-bundle.alertmanager.enabled=true` with
`defaultSource.enabled=false` renders a `SignalAdapter` **and** its webhook
Service with zero `SignalSource`s — a pod and a Service permanently idle,
listening on a path (`/webhook/{source}`) that no source can route through.
Same for any bundle flag enabled before its CRs are written.

The resource cost is small. The inconsistency is not: this project's posture is
that **unclaimed things are inert** — unclaimed sources drop signals with
`Wired=False`, unwired channels answer bare messages with guidance only. A
running adapter with nothing to serve is the one component that ignores the
rule.

## What Changes

- **An adapter's workload scales to zero when nothing demands it.** Demand is
  defined per kind, and the asymmetry is deliberate:

  | | active when |
  |---|---|
  | `ChannelAdapter` | ≥1 `Channel` names it (`spec.adapter == <CR name>`) |
  | `SignalAdapter` | ≥1 `SignalSource` names it **and** a Ready `Pipeline` claims that source |

  A `Channel` is a live surface a human can type into at any moment, and a
  polling adapter must be awake to hear it — so a Channel existing is demand.
  A `SignalSource` no Pipeline claims **provably** drops every signal it
  produces, so its adapter has nothing to do and sleeping loses nothing.
- **Scale to zero, do not delete.** The Deployment, ServiceAccount, and Service
  stay; only `replicas` goes to 0. `kubectl get deploy` shows `0/0` rather than
  the workload vanishing, and no ownership or credential-projection logic moves.
- **A new `Active` condition on both adapter CRs** explains the state:
  `Active=False` with reason `NoServedChannels` / `NoWiredSources`, naming what
  would wake it. Without this, a sleeping adapter is indistinguishable from a
  broken one.
- **Scale-down is delayed by an idle grace period**; scale-up is immediate.
  Editing a Pipeline or recreating a Channel must not thrash a Deployment, but
  a newly wired source must not wait.
- **`Served` semantics are unchanged.** `Served` answers "does an
  implementation exist for this type", which stays true while an adapter
  sleeps. The two conditions answer different questions and are kept separate.
- **New watches**: the ChannelAdapter reconciler watches `Channel`; the
  SignalAdapter reconciler watches `SignalSource` and `Pipeline`. The reverse
  index already exists in `channel_controller.go:107-120` and
  `signalsource_controller.go:117` for the `Served` mapping.

Not in scope: per-Channel workload sharding (one adapter pod still serves all
its channels); scaling above 1; deleting workloads instead of scaling them;
any change to credential projection, tokens, or the adapter contracts.

## Capabilities

### New Capabilities
- `on-demand-adapter-workloads`: the demand definition per adapter kind, the
  scale-to-zero mechanism, the `Active` condition, idle hysteresis, and the
  watches that keep it converged.

### Modified Capabilities
- `channel-adapter-lifecycle`: the reconciler no longer renders `replicas 1`
  unconditionally — replica count follows served-Channel demand.
- `signal-adapter-lifecycle`: same, with demand additionally requiring a Ready
  Pipeline claim on the served source.

## Impact

- **`internal/controller/adapterworkload.go`**: `adapterWorkload` gains a
  desired-replicas input; `ensureAdapterWorkload` stops hardcoding
  `replicas := int32(1)`. The Service path is untouched.
- **`internal/controller/channeladapter_controller.go`**: counts served
  Channels, computes demand, sets `Active`, adds a `Channel` watch.
- **`internal/controller/signaladapter_controller.go`**: same over
  `SignalSource` × Ready `Pipeline`, adds two watches.
- **`api/v1alpha1`**: no schema change — `Active` is a status condition on the
  existing `conditions` list. Idle grace period is a manager env knob, not CRD
  surface.
- **Tests**: `internal/integration/channeladapter_test.go` and
  `signaladapter_test.go` currently assert a Deployment exists after creating
  an adapter CR alone; those assertions change meaning and must be updated
  deliberately rather than incidentally.
- **Docs**: `CLAUDE.md` adapter-lifecycle terminology and the invariant list;
  README/`docs/` note that enabling a bundle flag no longer costs a pod.
- **Interacts with `chat-signal-origination`**: that change makes one Telegram
  surface three workloads (router, signal adapter, channel adapter). The router
  is gated on the signal side — it is the component that would hear a wake-up —
  and this needs stating explicitly so the trio does not sleep as a unit and go
  deaf.
