## Why

A `SignalSource` only accepts or drops a signal by `Wired`/cooldown today.
There is no way to narrow which signals a source routes at all.

An operator pointed at a noisy adapter (e.g. an Alertmanager webhook carrying
alerts for several unrelated teams) has no CRD-level way to admit only the
ones that matter, short of pre-filtering upstream of the adapter.

Kubernetes already has a well-understood shape for this: `matchExpressions`'
`key`/`operator`/`values`. The normalized signal already carries a `labels`
map this can evaluate against.

## What Changes

- `SignalSource.spec` gains `attributeFilters`, a list of
  `key`/`operator`/`values` requirements (the same shape as a Kubernetes
  `matchExpressions` entry: `In`, `NotIn`, `Exists`, `DoesNotExist`),
  evaluated against the normalized signal's `labels`. A signal failing any
  requirement is dropped before cooldown/routing. Empty or unset admits
  everything, unchanged from today.
- `SignalSource.spec` gains `logFiltered` (nullable `*bool`, default `true`
  when unset): whether a signal dropped by `attributeFilters` is still
  recorded as a `signal.dropped` activity event. Set `false` to silence a
  high-volume, known-noisy filter without losing the manager's record of
  signals it DID admit.
- Manager-side ingest only — adapters are not involved: they have no
  visibility into `SignalSource` CRs today and gain none here. Filtering is
  pure id-equality-adjacent evaluation, same tier as cooldown.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `signal-source-model`: `SignalSource.spec` gains `attributeFilters` and
  `logFiltered`. Ingest SHALL drop a signal failing the filter before
  cooldown/routing. It SHALL emit a `signal.dropped` event for that drop
  unless `logFiltered` is `false`.

## Impact

**Affected code:**
- `platform/manager/api/v1alpha1/signalsource_types.go` — new fields,
  regenerated deepcopy and `chart/crds/`
- `platform/manager/internal/httpapi/signals.go` — filter evaluation ahead of
  `cooldown`/`routeSignals`
- `platform/manager/internal/activity/event.go` — new `signal.dropped` drop
  code for a filtered signal
- `platform/manager/internal/ingest/` — filter-matching helper, evaluated
  against `NormalizedSignal.Labels` using Kubernetes label-selector-requirement
  semantics (`k8s.io/apimachinery/pkg/labels`, already a manager dependency)

**Reference docs made untrue:**
- `docs/concepts.md` — `SignalSource` field reference gains the two fields
- `docs/cr-reference.md` — regenerated (`docs-generate.py`) `SignalSource`
  fields table

**Adopter site made untrue:**
- `docs/guides/signal-adapter.md` — hand-written section on filtering a
  source, and its generated `SignalSource` example block's `fields=` list
  extended via `docs-generate.py`

**Deliberately NOT touched:** `docs/contracts/signal-adapter.md`.

Confirmed with the user that filtering is purely a manager/ingest concern.
The adapter has no role in it and no visibility into it, unlike cooldown
(`ADP-3` exists only because cooldown could tempt an adapter to
re-implement it).

No new adapter obligation and no wire-format change means no contract edit
and no version bump.
