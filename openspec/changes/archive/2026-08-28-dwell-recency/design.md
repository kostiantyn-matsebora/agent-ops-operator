## Context

Both dwell queues (`signals/k8s-events/pending.go`, `signals/ha/pending.go`)
keep one `pendingEntry` per workload or integration with a `recurred bool`
set by the second arrival. `decide` runs the three-rung ladder at the deadline
and, for an `Unknown` verdict, emits on `recurred`. The two files are
deliberately the same shape and are edited together.

The deadline is set by the first event and may only be SHORTENED — by a
stricter rule arriving later, or by `escalateAfterObjects` (a quarter of the
dwell, floored at one minute). So "the window" at decision time is
`deadline − firstSeen`, not the rule's `for`.

See `proposal.md` — Why for the incident shape this answers.

## Goals / Non-Goals

**Goals:**
- Rung 2 distinguishes a burst that ended from one still going, in both adapters, with identical semantics.
- No new config surface; the closing window derives from the dwell.
- The emitted evidence says when the last event arrived.

**Non-Goals:**
- Any change to rung 1 (predicates), rung 3 (fail open), `for: 0`, escalation, inhibition, mute or the emit cap.
- Health predicates for Longhorn or any other CRD kind — that is the "three more RBAC grants" argument the spec already makes, one kind over.
- The reboot-manager (`NodeNotReady`) class: `for: 0` by design, answered by the documented mute example; an install's values, not this change.

## Decisions

**The HA adapter had a SECOND "recurred at all" path, found during apply.** Its
`health` returned `Unhealthy` whenever the snapshot's log count had risen since
the window opened — rung 1, not rung 2, so the boolean fix alone would not have
touched it. `logRecord.Timestamp` is the record's latest occurrence, so `health`
now takes `since` (the start of the closing window) and the count branch
believes a rise only when that timestamp falls inside it. A rise that stopped
earlier falls through to the config-entry predicate, or to rung 2.

**`lastSeen time.Time` replaces `recurred bool`.** The boolean loses the
information the decision needs. `recurred` is derivable (`lastSeen != firstSeen`)
so nothing else changes; the evidence renderer reads `lastSeen` for the new
line.

**Closing window = `max(30s, window/3)` where `window = deadline − firstSeen`.**
Derived from the window actually waited, so escalation and rule-shortening keep
the proportion (3m → 1m closing, 1m → 30s floor, 10m → 3m20s).
- *Alternative — a `recurWithin` rule field.* Rejected: one more value whose
  only sensible setting is "a fraction of `for`", and a knob documents itself as
  a decision an adopter must make. `escalateAfterObjects`' derived quarter is
  the precedent.
- *Alternative — extend the deadline on each arrival (sliding window).* Rejected:
  the spec says the deadline is set by the first event and never extended, and a
  controller retrying forever would then never emit.
- *Why a third and not a half.* Longhorn's observed bursts run 30–65s under a
  3m catch-all; a 1m closing window drops them while a retry at 2m10s still
  reports. Halving would drop a burst that ended at 1m29s — plausible for a
  problem that is merely slow, not healed.

**The rule lives in one helper per adapter, `stillRecurring(e, now) bool`**, with
the constant named, so the test pins the rule and the two adapters cannot
drift on the number silently. Cross-adapter agreement is a comment naming the
sibling file, the same convention the two ladders already use.

**Evidence line.** `last seen <d> before the window closed` appended to the
existing "unhealthy for" header. Dropped entries log the same at debug level
with the verdict — the same place the existing churn drop logs.

## Risks / Trade-offs

- [A problem that retries on a period longer than the closing window — e.g. a controller backing off to 90s under a 3m dwell — is dropped] → the floor of 30s and the third keep this to slow backoffs only; such a controller's next retry re-enters the queue as a fresh entry with its own dwell, so the report is delayed by at most one window, not lost. Named in the CHANGELOG.
- [Existing tests assert `recurred` semantics] → rewritten to arrival timelines; the two new scenarios are pinned per adapter.
- [Adopters relying on "recurred once = emit"] → behaviour change entry in `docs/CHANGELOG.md`; restating the rule with a shorter `for` restores near-old behaviour.

## Migration Plan

Two patch image releases (`signal-k8s-events`, `signal-ha`), tags pinned in
the two bundle values files, bundle chart and parent chart bumped. No config
change, no CRD change. Rollback is the previous image tag.
