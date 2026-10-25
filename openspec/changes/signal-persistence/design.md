# signal-persistence — design

## Context

See `proposal.md` — Why. Current state this design builds on:

- `/signal/inbound` routes synchronously at ingest
  (`internal/httpapi/signals.go`): source lookup, Wired check, cooldown,
  claim fan-out, equality routing into Conversations.
- Cooldown already persists on the `SignalSource` and rebuilds after a
  restart. Grouping is gone — the sender mints `initiationId` (#387).
- Payloads already land in etcd as `ConversationInput` once routed. Only the
  pre-routing stretch has no durable record.
- The signal adapter specification (`docs/contracts/signal-adapter.md`)
  states synchronous response semantics — the unwired drop is reported in
  the response. That spec is not edited by this change.
- The merged `introduce-control-plane` change states the RBAC pattern for
  manager-owned kinds: the manager holds full CRUD, every other component
  reads at most.

## Goals / Non-Goals

**Goals:**

- Every accepted signal is a CR before it is acked. No routing decision
  comes from memory that was not just read from a CR.
- Behavior-preserving routing: same cooldown, same fan-out, same equality
  reuse, same response semantics as today.
- A platform other changes can gate, filter and hold — without this change
  shipping any of those.

**Non-Goals:**

- No pause, hold, drop mode, maintenance window or control-plane resident.
- No per-binding filters, no change to claim semantics.
- No change to any adapter, to `/signal/inbound`'s request or response
  shape, or to the signal adapter specification.
- No message-broker component, in-cluster or external.

## Decisions

### D-1: The queue is the Kubernetes API — a CRD, never a broker component

Held and in-flight signals persist as `Signal` CRs. The invariant rules
everything else out: manager state is Kubernetes API or derivable, the
manager mounts nothing, and an accepted signal is neither derivable nor
declared-lossy (the adapter will not resend).

Volumes are post-dedup ops signals — tens per hour. Caps and absorb keep the
object count bounded (D-6).

**Alternative considered:** an external broker (NATS, Redis Streams).
Rejected — a new stateful component, a new trust boundary and credential
custody problem, for throughput this domain never produces. The seam
remains: the store sits behind the routing path, swappable if volumes ever
change the math.

### D-2: Persist, then route inline, then respond

Ingest writes the `Signal` CR, runs the first routing attempt synchronously
through the ingest core, and answers with that attempt's outcome. The
reconciler owns every later attempt.

This is what keeps the adapter contract untouched: the response still
reports the unwired drop and its reason, latency is unchanged, and the spec
needs no edit — so no five-adapter conformance sweep.

**Alternative considered:** pure async — persist and return 202, route only
in the reconciler. Rejected for this change: it changes the documented
response semantics, which drags the specification and every shipped adapter
into scope. A future change may still choose it deliberately.

### D-3: One routing path, two callers

The routing logic moves out of `/signal/inbound`'s serving code into a
routing function both ingest (first attempt) and the reconciler (retries,
recovery) call. The reconciler acts only on non-terminal Signals.

Crash recovery is therefore free: a new leader lists `Pending` and
`PartiallyRouted` and calls the same function. Idempotency rides on equality
reuse — a conversation created before the crash is found by
`conversationId == initiationId` and appended to, never duplicated.

### D-4: Claimants are the state machines, phase is a roll-up

`status.claimants[]` carries one workflow per claimant:
`Routing → Routed | Failed | Skipped`. The signal's phase is
recomputed from the claimant states on every status write, never
authored independently.

- The claimant set snapshots ONCE, at first routing attempt — the same principle
  that pins a Conversation's refs. A re-wire never changes an in-flight
  signal's fan-out.
- Retries touch only non-terminal claimants. The record is the
  idempotency guard.
- A bounded retry budget flips exhausted claimants to `Failed`, so
  `PartiallyRouted` always exits.

**Alternative considered:** a single phase field written by the route
loop. Rejected — partial routing becomes invisible, and a retry cannot know
which claimant already took the signal.

### D-5: Absorb at ingest, by `initiationId`

A signal arriving with an `initiationId` matching a still-open Signal of the
same source is folded into it: payload appended, occurrence count bumped,
no second CR.

- A claimant already `Routed` receives the new payload through the
  normal equality-append to its open conversation.
- A non-terminal claimant carries it on its next attempt.
- No special case either way.

### D-5a: Terminal Signals are audit records, never reopened

Absorb applies to NON-terminal Signals only. A repeat whose only match is
terminal creates a fresh CR, and cooldown then usually ends it as
`Dropped` (reason cooldown). Reopening a terminal record would make the
audit trail mutable.

Two phase consequences fall out of the roll-up, stated in the spec:

- Retry exhaustion with at least one routed claimant rolls up to `Routed` — the
  exhausted claimant is `Failed`, visibly, per claimant.
- Phase `Failed` is reserved for a signal no claimant routed.

The `Dropped` reason set is: unclaimed, cooldown, overflow, capacity. The
last records a chat signal declined at a full backlog (D-8).

### D-6: Bounds — per-source cap, TTL GC, ownerRef

- A per-source cap on live (non-terminal) Signals. Absorb is the primary
  enforcement. Overflow evicts the OLDEST as `Dropped` (reason overflow),
  counted and logged — the `MaxCooldownEntries` degrade-gracefully
  precedent.
- TTL GC after a terminal phase, in the reconciler via requeue. `Dropped`
  short, `Routed` longer — the routed record is the audit trail — and
  `Failed` on the `Routed` TTL, since nobody-got-it is what an operator
  investigates.
- `ownerRef` to the `SignalSource`: a deleted source takes its signals from
  any phase, consistent with the unwired-drop posture.

### D-7: RBAC per the control-plane pattern

The manager's Role gains full CRUD on `signals` (lowercase plural — the
informer breaks silently otherwise). The console's Role gains
`get`/`list`/`watch` only. No other component touches the kind.

### D-8: Backlog-full machine signals wait, chat keeps the decline

With a durable record, dropping an alert because 50 conversations are
Pending is no longer forced.

- Machine kinds (alert, job, task) stay `Pending` as Signals and route
  when the backlog drains.
- Chat keeps the immediate decline on the originating surface — a person is
  answered now, never left waiting silently.

## Risks / Trade-offs

- **A persist failure is unacked** → the response reports it, and safety
  rests on the adapter contract's existing at-least-once posture: the
  sender re-sends with the same `initiationId`.
- **Overflow can evict a Signal with a `Routing` claimant mid-retry** →
  accepted, deliberately. The cap protects etcd. Pre-fan-out the eviction is
  a counted, logged `Dropped` (reason overflow). Post-fan-out it flips the
  non-terminal claimants to `Failed` (reason overflow) and the phase rolls
  up — `Dropped` stays strictly pre-fan-out, and the object set stays
  bounded either way.
- **etcd churn from a flapping source** → absorb collapses repeats into one
  object update, the per-source cap bounds the worst case, and cooldown
  still suppresses inside its window.
- **Two writers of one status (ingest's first routing attempt, reconciler's
  retries)** → ingest writes through the leader today
  (`leaderProxyMiddleware`), and the reconciler is leader-only. Optimistic
  concurrency on the status write settles any race the proxy misses.
- **A signal squatting non-terminal forever** → the retry budget flips
  claimants to `Failed`, the phase roll-up then terminates it, TTL GC
  removes it.
- **Console screenshot/demo drift** → the UI change re-runs the
  `screenshots` and `demo` npm scripts, per `documentation.md`.
- **Phase vocabulary leaking into adapter docs** → the specification is not
  edited. The `Signal` kind is documented in `concepts.md` and the console
  pages only.

## Migration Plan

Additive. New CRD only.

1. `chart/crds/` gains the `Signal` CRD (controller-gen output). Upgrades
   run `kubectl apply -f chart/crds/` — the standing rule, Helm never
   upgrades an existing CRD.
2. RBAC rows land in the same release as the manager that needs them.
3. Rollback: the previous manager ignores the kind entirely. Orphaned
   Signal objects are inert and GC with their sources.

## Open Questions

- Exact TTL defaults for `Dropped` vs `Routed` (chart-tunable, defaults
  decided at implementation against the housekeeping cadence).
- The retry budget's value and home (chart value vs fixed). The spec only
  requires the bound to exist and exhaustion to record as the `Failed`
  reason.
- Whether the console signals view lives as its own page or as a tab on the
  SignalSource detail — settled by the frontend task against the existing
  navigation, the spec constrains content, not placement.
