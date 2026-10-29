# signal-persistence

## Why

Signals are the only input the manager handles with no durable record.
`/signal/inbound` routes synchronously through the manager's single ingest
core, and whatever cannot route at that instant — a full backlog, a crash
mid-route — is gone, invisibly.

The manager should behave as a message broker. Publishing a signal stores it
and nothing more. A separate consumer picks stored signals up and routes
them.

Planned operational scenarios then become a change to the consumer alone:
pausing consumption for a maintenance window, per-binding filters. The
endpoint keeps accepting and storing meanwhile.

This change is that enabler, and ONLY that enabler.

## What Changes

- A new `Signal` CRD — the queue. One `Signal` is one message: every
  admitted signal becomes its own CR, and no repeat is folded into an earlier
  one. Correlating a message with a conversation (by `initiationId`) is the
  consumer's job, never the endpoint's.
- **Publish.** `/signal/inbound` validates, applies the source's
  `attributeFilters`, creates the `Signal` CR, and acknowledges. It makes no
  routing decision: no claim lookup, no cooldown, no unwired check, no
  capacity check, no addressing. A chat command that only answers
  (`/pipelines`, a bare `/<pipeline>`) opens nothing and is answered in
  place, as today. Publication runs on the leader through the existing
  `leaderProxyMiddleware`.
- **Optional `signalId`.** An adapter may send a per-message id, stable across
  a re-send of that message. The `Signal`'s name derives from the source and
  `signalId`, so publish is idempotent: any number of signals with the same
  `(source, signalId)` land as one `Signal`. Every shipped signal adapter
  sends one.
- **Consume.** The leader runs a pool of concurrent consumers, a competing
  consumer on the leader. The manager's cooldown map, channel-operation queue
  and activity log are per-process and filled only by the leader, so the
  consumer runs there. One consumer holds a `Signal` at a time, and messages of
  one conversation are processed in arrival order. A leader change resumes
  every unfinished `Signal` from its CR. The `Signal` records who consumed
  it and how many attempts it took.
- **The consumer makes every routing decision:** the addressed target of
  `/<pipeline> <task>` or the source's Ready claimants (snapshotted at first
  processing, as before), cooldown, unwired and ambiguous chat, capacity, and
  the idempotent create-or-append into the conversation. The `Signal` records
  a drop and its reason. A chat notice (unwired, ambiguous, at capacity) is
  enqueued as today, fire-and-forget.
- **Housekeeping is the leader's alone:** the retry deadline flips claimants
  to `Failed`, the per-source cap evicts the oldest, and TTL garbage
  collection deletes finished signals — `Routed` 8h, `Dropped` 4h, `Failed`
  72h, counted from `status.completedAt`. Every bound is a chart value.
- Per-claimant routing workflow in `status.claimants[]`
  (`Routing → Routed | Failed | Skipped`) and the derived phase roll-up
  (`Pending`, `Routed`, `PartiallyRouted`, `Dropped`, `Failed`), as before.
- Backlog-full stops dropping machine signals. An alert, job or task signal
  arriving against a full pending backlog stays `Pending` and is routed once
  the backlog drains. Chat keeps its immediate decline, addressed chat
  included, told on the surface it arrived on.
- Console: a read-only view of signals and their per-claimant state, under
  the existing Configuration pages. One more watched kind, no write path. A
  later change may move state kinds to a page of their own.
- RBAC per the control-plane pattern: the manager holds full CRUD on
  `Signal`, the console reads only.
- The signal adapter contract (`docs/contracts/signal-adapter.md`) IS edited,
  to version **2.3.0** (MINOR): the inbound response acknowledges storage —
  `queued` counts signals stored — and no longer reports `conversations` or a
  routing `reason` (only the `attributeFilters` refusal remains). `signalId`
  is added. No shipped adapter's obligation reads the withdrawn fields, so
  none stops conforming.
- Explicitly OUT of scope, as future changes this one prepares for: the
  pause/hold gate itself, maintenance or drop modes, per-binding filters, any
  control-plane resident. The seam is consumption being separable from
  publication.
- **BREAKING**: none. New CRD, an additive request field and a narrowed
  response — upgrades run `kubectl apply -f chart/crds/` as every
  CRD-adding release already does.

## Capabilities

### New Capabilities

- `signal-persistence`: the `Signal` CRD and its lifecycle — publish-only
  ingest, the optional `signalId` and idempotent publish, the leader's
  concurrent consumer pool with per-conversation ordering, the per-claimant
  routing workflow, derived phases, leader-only housekeeping, per-phase TTLs
  and the chart values for every bound, and the manager-exclusive-write RBAC
  posture.
- `console-signals`: the console's read-only signal view — the list, the
  per-claimant state and consumer on a signal's detail, and the watch that
  feeds both.

### Modified Capabilities

- `signal-adapter-contract`: the inbound response acknowledges storage, the
  optional per-message `signalId` is added, every shipped adapter sends one,
  and the specification moves to version 2.3.0.
- `chat-signal-origination`: every chat message that can open or continue a
  conversation becomes a `Signal` first and the consumer decides. Commands
  that only answer stay in place.
- `cron-signal-adapter`, `alertmanager-signal-adapter`,
  `k8s-events-signal-adapter`, `ha-signal-adapter`, `pulse-signal-adapter`:
  each states what its message is and sends a stable `signalId`.
- `conversation-capacity`: the bounded-backlog requirement changes — the
  consumer applies it, a full backlog lets machine-originated signals wait,
  and chat origins keep the immediate decline.
- `state-durability`: the `Signal` CR is a declared home for live state, and
  the restart-resilience matrix gains its row.
- `console-topology`: `Signal` joins the read-only watched kinds and the CR
  inventory views.

## Impact

**Code**:

- `platform/manager/api/v1alpha1/`: the `Signal` type — spec fields,
  `consumedBy`, `consumedAt`, `attempts`, `completedAt` — deepcopy, CRD YAML
  under `chart/crds/`.
- `platform/manager/internal/httpapi/signals.go`: publish only — validate,
  filter, create, acknowledge. The routing moves out.
- `platform/manager/internal/signalroute/` and
  `platform/manager/internal/controller/`: the leader's consumer pool with
  per-conversation ordering, and the leader-only housekeeping.
- `platform/manager/internal/chat/`: addressing and the chat notices run from
  the consumer. The addressed conversation adopts the `initiationId` as its
  `conversationId`.
- `platform/manager/internal/ingest/`: the cooldown store does not move, but
  it is spent by the consumer under optimistic concurrency, and a suppression
  is recorded as `Dropped` (reason cooldown).
- `signals/{cron,alertmanager,k8s-events,ha,pulse,telegram}/`: each sends a
  stable `signalId`. The Telegram adapter has no spec of its own, so its
  requirement lives in `chat-signal-origination`.
- `chart/templates/` and `chart/values.yaml`: `signals` RBAC (full on the
  manager, read-only on the console), and the settings `signalTtlRouted`,
  `signalTtlDropped`, `signalTtlFailed`, `signalRetryDeadline`,
  `maxSignalsPerSource`, `signalConsumerConcurrency`.
- `platform/console/`: the new watched kind, the signals view, manager
  client read path.

**Reference docs**:

- `docs/concepts.md` — the `Signal` kind, phases, claimant workflow, the
  consumer, the CRD and state matrices.
- `docs/crds/signal.md` — regenerated (`docs-generate.py`), new kind.
- `docs/contracts/signal-adapter.md` — version 2.3.0: the response, the
  `signalId`, the rewritten `API-2`/`API-3`, new rows, changelog.
- `docs/contracts.md` — the ingest flow note: publish then consume.
- `docs/console.md` — the signals view and the added watched kind.
- `docs/configuration.md` — the six chart values.
- `docs/CHANGELOG.md` — the new CRD, the contract version and the
  `kubectl apply -f chart/crds/` upgrade step.

**Adopter site**:

- `docs/installation.md` — the CRD upgrade step for this release.
- `docs/console-guide.md` — what the signals view shows.
- `docs/integrations/*.md` and `docs/guides/signal-adapter.md` — the claimed
  specification version.
- `README.md` — the kind list gains `Signal` (budget respected).
- Landing page kind/diagram content if it enumerates kinds
  (`export.py`/`readme-flow.py` regenerate the diagrams, never hand-edited).

**Tests**:

- Manager unit and envtest: publish idempotency, the consumer's routing
  decisions, per-conversation ordering under concurrent consumers, and
  leader-only housekeeping.
- Conformance suite: the `signalId` and re-send-after-`5xx` checks, and a
  Pulse case.
- Console vitest for the view.
- System-tier smoke: publish to conversation through the queue, including an
  addressed message and a leader change.
