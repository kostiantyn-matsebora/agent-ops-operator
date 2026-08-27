# Proposal: dwell-recency

## Why

The dwell's second verification rung asks the wrong question. For an involved
object with no health predicate — every kind but `Pod` in `signal-k8s-events`,
every integration without a config-entry state in `signal-ha` — the adapter
emits if the event **recurred at all** during the window. The spec's intent is
"drop if it went silent", but `recurred` is a boolean set by the second event
and never revisited, so a burst that fired five times in thirty seconds and
then stopped is indistinguishable from one still going at the end of the dwell.

On the reference install that is most of the backlog: of 25 alert
conversations, twelve are Longhorn `Volume`/`Engine` warnings (`FailedSnapshotPurge`,
a replica fault) that healed in 30–65 seconds and reached the 3-minute
catch-all only because Longhorn retries every few seconds while it is retrying;
four are Home Assistant network blips of the same shape. Each spent a full
agent run to write "transient, self-resolved, no action needed". The
verification the adapter was built to do before opening a conversation was
being done by the LLM after it.

The other large class — `NodeNotReady` from a reboot manager — is NOT a dwell
case: tier 3 is `for: 0` by design and is never verified. The shipped answer is
the time axis (`route.muteTimeIntervals`), already a worked example on the
Kubernetes integration page. That is an install's values decision, and no
change here.

## What Changes

- **Rung 2 checks recency, not occurrence.** Both dwell queues record the
  time of the last arrival per entry. At the deadline, an entry with no health
  predicate emits only if its last arrival fell inside the closing part of the
  window; one that went quiet before that is dropped as churn — the same
  verdict a pod that became `Ready` already gets.
- **The closing window is derived from the dwell, not configured** — the last
  third of the window actually waited, floored at thirty seconds — the same
  shape as `escalateAfterObjects` deriving its shortened dwell. A knob would be
  one more value to explain whose only sensible setting is "a fraction of
  `for`".
- **A problem still live keeps emitting.** A controller retrying at the end of
  the window has a fresh last arrival and passes; the pinned scenarios cover
  both directions.
- **Rung 3 (existence unknown) and rung 1 (predicate) are untouched**, and
  `for: 0` still emits immediately with no re-check.
- **The evidence payload names the quiet period** — "last seen 2m10s before the
  window closed" — so an emitted signal says why it was believed and a reader
  of a dropped one (at debug level) sees what dropped it.

## Capabilities

### New Capabilities

_None._

### Modified Capabilities

- `k8s-event-suppression`: the dwell-and-verify requirement's rung 2 becomes
  "emit only if the event was still recurring as the window closed", with the
  derived closing window stated and two scenarios pinning the quiet burst and
  the live retry.
- `ha-signal-adapter`: the same rung, stated for the log adapter's re-check.

## Impact

**Code**

- `signals/k8s-events/pending.go` and `pending_test.go` — `lastSeen` replaces
  the `recurred` boolean; `decide` compares it against the closing window;
  evidence names the gap.
- `signals/ha/pending.go` and `pending_test.go` — the same change; the two
  ladders are kept in the same shape on purpose.
- Image tags: `signal-k8s-events` and `signal-ha` each publish a patch release;
  `chart/charts/kubernetes/values.yaml` and `chart/charts/home-assistant/values.yaml`
  pin the new tags; both bundle charts and the parent chart bump.

**Reference docs**

- `docs/integrations/kubernetes.md` — the verification description ("How events
  are verified") states the closing-window rule for kinds without a predicate.
- `docs/integrations/home-assistant.md` — the re-check table row ("falls back to
  *did it recur*") becomes "was it still recurring as the window closed".
- `docs/CHANGELOG.md` — behaviour change entry: fewer conversations from
  self-healing bursts on uninspectable kinds; anything wanting the old
  behaviour restates its rule with a shorter `for`.
- `chart/charts/kubernetes/values.yaml` / `home-assistant/values.yaml` — the
  `rules` comment describing the re-check is updated where it says "recurred".

**Adopter site**

- The landing page, `introduction.md`, `getting-started.md`, `installation.md`
  and the guides do not describe rung 2 and are unchanged; the integration
  pages above are the adopter-facing surface for this behaviour.

**Not changed**

- The kured / scheduled-reboot mute: already documented as a worked example on
  the Kubernetes page and in the bundle values. Applying it is an install's
  values decision.
- No config field is added or removed; `.github/retired-vocabulary.json` gains
  no term.
