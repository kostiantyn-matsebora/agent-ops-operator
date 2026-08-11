## Why

A router that restarts at 04:00 every night takes the cluster's connectivity
with it for ten to fifteen minutes. Kubernetes reacts exactly as it should —
nodes go `NotReady`, probes fail, pods are evicted and rescheduled — and the
events adapter reports all of it. An agent is woken, repeatedly, about a
condition that is known, scheduled, self-healing, and already understood. It
costs LLM credits and it trains whoever reads the channel to ignore the 04:00
burst, which is the expensive part.

None of the three suppression mechanisms already in the adapter can express it:

- **`for:` dwell** verifies a condition still holds after a delay. Here it
  genuinely does hold — for a quarter of an hour — so a dwell long enough to
  cover the outage would delay every real incident by the same amount, all day.
- **Inhibition** suppresses the consequences of a cause that is *already
  reported*, keyed on a cause event. A router losing power produces no
  in-cluster cause object to key on; the cluster only sees consequences.
- **Matchers** select on labels. There is no label for "it is four in the
  morning".

The missing axis is TIME, and it is the one axis Alertmanager already models.

## What Changes

- **`route` gains `timeIntervals` and `muteTimeIntervals`**, using Alertmanager's
  names, schema and semantics: named intervals of `times`, `weekdays`,
  `daysOfMonth`, `months`, `years` and a `location`, referenced by name from the
  route. This project borrows Prometheus and Alertmanager vocabulary exactly and
  refuses near-synonyms — the `for:` versus `group_wait` rule exists precisely
  because a familiar word meaning something slightly different is worse than a
  new one.
- **Muting is evaluated at EMIT time, after the dwell**, which is what
  Alertmanager's mute intervals mean: they mute *notifications*, not evaluation.
  This buys a property worth having — a problem that starts during the window and
  **persists past it** still surfaces, one dwell after the window closes, because
  the cluster keeps producing events for anything genuinely still broken.
- **A mute entry MAY carry `matchers`**, so a window can silence the reasons a
  scheduled outage produces without going blind to everything else. Going deaf
  for fifteen minutes is the real risk of this feature, and narrowing the mute is
  the mitigation that does not depend on the window being short.
- **`location` is an explicit IANA timezone.** "Four in the morning" is a local
  fact, and a window pinned to UTC drifts by an hour twice a year — arriving
  exactly when nobody is looking at it.
- **Muting is never silent.** The adapter counts muted events and reports them on
  the source's `Ready` condition, and while a window is active the condition says
  so — mirroring how emit-cap clipping is already surfaced, so that
  `kubectl get signalsource` explains the silence rather than leaving it looking
  like an idle lane.
- **Chart values expose it** on the k8s bundle's events source, with the nightly
  maintenance window as the documented example.

## Capabilities

### New Capabilities

<!-- none — this adds an axis to existing suppression rather than a new surface -->

### Modified Capabilities

- `k8s-event-suppression`: suppression gains a time axis — named intervals, mute
  references, emit-time evaluation, optional matchers, and the reporting rule
  that a muted window is visible on the source.
- `k8s-bundle`: the events component's values carry the interval definitions, so
  a maintenance window is release configuration rather than a hand-edited CR.

## Impact

- **Adapter**: `signal-k8s-events/` — a new time-interval matcher, evaluation
  hooked in after the dwell queue and before emit, muted counters, and the
  `Ready` condition reporting. `inhibit.go` and `pending.go` are untouched in
  behaviour; only the emit path gains a check.
- **Config**: `SignalSource.spec.config.route` gains two keys. Absent, nothing
  changes.
- **Chart**: `chart/charts/k8s-bundle` values for the events source, with a
  worked nightly example and the timezone spelled out.
- **Docs**: `docs/k8s-bundle.md` — the suppression section gains the time axis
  alongside `rules` and `route`.
- **Out of scope**: an ad-hoc "mute now for 30 minutes" command. That needs a
  write path into a running adapter, and the config is declarative on purpose.
- **Deliberately not included**: `activeTimeIntervals` (the inverse — notify only
  during a window). Named in the design so the omission is a decision rather than
  an oversight.
