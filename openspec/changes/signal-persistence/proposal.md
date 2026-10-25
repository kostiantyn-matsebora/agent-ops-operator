# signal-persistence

## Why

Signals are the only input the manager handles with no durable record.
`/signal/inbound` routes synchronously through the manager's single ingest
core, and whatever cannot route at that instant — a full backlog, a crash
mid-route — is gone, invisibly.

Planned operational scenarios (pausing consumption, maintenance windows,
per-binding filters) all need signals to exist as persistent, inspectable
objects first. This change is that enabler, and ONLY that enabler.

## What Changes

- A new `Signal` CRD. `/signal/inbound` becomes persist-then-ack: the CR is
  written first, the response follows. A signal whose `initiationId` matches a
  still-open `Signal` is absorbed into it — payload appended, never a second CR.
- One routing path, two callers: ingest performs the inline first attempt
  (which keeps today's response semantics), and a new `Signal` reconciler
  owns retries and crash recovery. No in-memory routing state — a new
  leader resumes from the CRs alone.
- Per-claimant routing workflow in `status.claimants[]`: one entry per
  claimant, `Routing → Routed | Failed | Skipped`, the set snapshotted
  once at first routing attempt. Retries touch only non-terminal claimants, and
  equality reuse (`conversationId == initiationId`) makes them idempotent.
- The signal phase is a derived roll-up over claimants: `Pending`,
  `Routed`, `PartiallyRouted`, `Dropped` (reason: unclaimed, cooldown,
  overflow), `Failed` (all claimants terminal, none routed — surfaced
  as loudly as the unwired drop).
- Lifecycle bounds: per-source cap with absorb-by-`initiationId` as the
  primary enforcement (the `MaxCooldownEntries` precedent), TTL GC after a
  terminal phase — short for `Dropped`, longer for `Routed` (the audit trail).
  `ownerRef` to the `SignalSource`, so a deleted source takes its signals.
- Backlog-full stops dropping machine signals. An alert, job or task signal
  arriving against a full pending backlog waits as a `Pending` `Signal`
  instead of being discarded. Chat keeps its immediate decline — a person is
  told now, never left waiting silently.
- Console: a read-only view of signals and their per-claimant state. One
  more watched kind, no write path.
- RBAC per the control-plane pattern: the manager holds full CRUD on
  `Signal`, the console reads only.
- Explicitly OUT of scope, as future changes this one prepares for:
  pause/hold gates, maintenance or drop modes, per-binding filters, any
  control-plane resident. The signal adapter contract
  (`docs/contracts/signal-adapter.md`) is NOT edited — request and response
  semantics are preserved by the inline first routing attempt.
- **BREAKING**: none. New CRD only — upgrades run
  `kubectl apply -f chart/crds/` as every CRD-adding release already does.

## Capabilities

### New Capabilities

- `signal-persistence`: the `Signal` CRD and its lifecycle — persist-then-ack
  ingest, absorb-by-`initiationId`, the per-claimant routing workflow,
  derived phases, bounds and TTL GC, the reconciler's retry and
  leader-change recovery, and the manager-exclusive-write RBAC posture.
- `console-signals`: the console's read-only signal view — the list, the
  per-claimant state on a signal's detail, and the watch that feeds both.

### Modified Capabilities

- `conversation-capacity`: the bounded-backlog requirement changes — a full
  backlog lets machine-originated signals wait as `Pending` `Signal` CRs
  instead of dropping the batch, while chat origins keep the immediate
  decline.
- `state-durability`: the `Signal` CR is a new declared home for live state,
  and the restart-resilience matrix gains its row.
- `console-topology`: `Signal` joins the read-only watched kinds and the CR
  inventory views.

## Impact

**Code**:

- `platform/manager/api/v1alpha1/`: the `Signal` type, deepcopy, CRD YAML
  under `chart/crds/`.
- `platform/manager/internal/httpapi/signals.go`: persist-then-ack, the
  inline first routing attempt extracted into the shared routing path.
- `platform/manager/internal/controller/`: the new `Signal` reconciler —
  retries, phase roll-up, TTL GC, leader-change resume.
- `platform/manager/internal/ingest/`: unchanged (cooldown stays as is).
- `chart/templates/rbac.yaml`: `signals` with full verbs on the manager's
  Role, read-only on the console's.
- `platform/console/`: the new watched kind, the signals view, manager
  client read path.

**Reference docs**:

- `docs/concepts.md` — the `Signal` kind, phases, claimant workflow, the
  CRD and state matrices.
- `docs/cr-reference.md` — regenerated (`docs-generate.py`), new kind.
- `docs/contracts.md` — the ingest flow note: persist-then-ack, response
  semantics unchanged.
- `docs/console.md` — the signals view and the added watched kind.
- `docs/CHANGELOG.md` — the new CRD and the `kubectl apply -f chart/crds/`
  upgrade step.

**Adopter site**:

- `docs/installation.md` — the CRD upgrade step for this release.
- `docs/console-guide.md` — what the signals view shows.
- `README.md` — the kind list gains `Signal` (budget respected).
- Landing page kind/diagram content if it enumerates kinds
  (`export.py`/`readme-flow.py` regenerate the diagrams, never hand-edited).

**Tests**: manager unit + envtest for lifecycle and recovery, conformance
suite untouched (the adapter contract does not move), console vitest for the
view, system-tier smoke for ingest-to-conversation through the new path.
