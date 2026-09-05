## Context

See `proposal.md` — Why. What shapes the approach:

- **The adapter already has the shape a surface needs.** `consider` is one
  entry for every record: self-exclusion, scope, rules, inhibition, dwell,
  post. The dwell keys entries by integration and re-checks them through
  `health`, which already reads config entry states as its first rung. The
  sweep polls the log listing every fifteen seconds and reads
  `config_entries/get` beside it. What is missing is records that are not log
  records, and the reads that produce them.
- **The surfaces differ in cost and cadence.** Config entries and repairs are
  tens of small objects and one command each. States are the whole house —
  703 entities on the reference install, several hundred kilobytes — and the
  entity registry, which maps an entity to the integration that owns it, is
  the same size again. Neither needs fifteen-second latency: a device fault
  and a pending update are minutes-scale facts, and the rules that ship for
  them dwell.
- **A surface condition is a STATE, not an occurrence.** A log record happens;
  a failed entry stands. The cursor and the timestamp dedup are built for
  occurrences and have nothing to compare for a state that is simply still
  true on the next read.
- **The signal contract has no resolution.** A signal opens or joins a
  conversation; nothing tells the manager a condition cleared. "Closes when it
  loads again" therefore means the adapter stops reporting and a later failure
  is a new report, which the manager's cooldown and signature window turn into
  the same conversation while its window is open.
- **The `kind` word is taken.** `Signal.Kind` is the contract's lane
  (`alert` | `job` | `task` | `chat`) and `signal-k8s-events` labels the object
  kind. The label that names which surface a record came from is `surface`.

## Goals / Non-Goals

**Goals:**

- One route and one rules policy cover the log and the four surfaces, with a
  rule able to pick any one of them.
- A disabled surface costs nothing: no read, no memory. The one shared read
  is `config_entries/get`, which the log lane's dwell already used as its
  first rung before any surface existed; with the surface off it is read
  only while something is pending, so an install with nothing pending and
  the surface off issues it never.
- A standing condition is reported once per standing period, and is verified
  against its own surface at the close of a dwell.

**Non-Goals:**

- Entity availability as a surface. See the proposal for why; it is the one
  candidate deliberately left out rather than deferred.
- Acting on any surface — installing an update, running a repair's fix flow.
  The adapter observes; what an agent does with it is the profile's and the
  toolset's business.
- Renaming the adapter, the source or the `logsAdapter` values path to say
  "health". A rename is a migration for every install, for a word.
- Redrawing the integration page's diagram. It draws a log record as the
  example of what the lane re-checks; the caption is generalised and the
  drawing kept, because it is a hand-authored SVG with no generator behind it
  and the log path is still the path it shows correctly.

## Decisions

**Two cadences, not one.** The existing fifteen-second sweep reads the log
listing and config entries, and now `repairs/list_issues` beside them — all
three are small. A second loop, `statesEvery` (one minute), reads `get_states`;
the entity registry is read on connect and every fifth states read. Both loops
end with the session, exactly as the poller does.

- *Why not events for states?* Home Assistant's `state_changed` is the way its
  own frontend stays current, and it would be cheaper on a busy house. It is
  also a firehose the adapter would have to filter and re-sync, with the
  registry to keep current beside it. Every rule that ships for these surfaces
  dwells for minutes, so a minute of latency is below what anyone can observe.
  Events can be added later as the fast path exactly as the log event is.
- *Why not one cadence?* Reading the whole house every fifteen seconds is a
  megabyte a minute against an instance that may be a Raspberry Pi. Reading
  the log every minute would add a minute to a `for: 0` rule that today waits
  fifteen seconds.

**A surface produces records through the same `consider`, via a standing
set.** Each source keeps, per surface, the set of conditions currently
standing, keyed by the condition's fingerprint. A read computes the current
set; a condition in the new set and not the old one is fed to `consider`; one
in the old set and not the new one is forgotten. The record fed in is a
`logRecord` shaped for the surface — name, message, level, source location
and timestamp filled from the condition — so `consider`, the rules, the dwell
queue and self-exclusion are unchanged.

- *Alternative: a second `consider` for non-log records.* Rejected — two
  entry points is how one surface ends up with self-exclusion and the other
  without.
- *The timestamp dedup does not interfere.* A standing condition is fed in
  once, at its first sighting, with the time it was first seen; it is the
  standing set, not the timestamp, that stops the next read re-feeding it.
- *A restart re-reports what stands.* The standing set is memory, and
  persisting it would be a second cursor-shaped state to keep true for every
  surface. The cost is one repost per standing condition per restart, which
  the manager's cooldown (six hours by default) absorbs and the signature
  window lands on the existing conversation. The same tolerance the
  post-then-persist cursor relies on.

**The dwell re-check consults the surface.** `health` already answers rung 1
from the config-entry snapshot. It gains the other two surfaces' current sets:
a config-entry condition is unhealthy while its entry is in a counting state;
a repair while its issue is filed; a sensor while its state still faults. A
condition absent from the current set at the close is healthy — it cleared
before the window closed — and is dropped as churn. The digest carries a zero
dwell and never reaches the ladder.

**The update digest is one fingerprint per source, posted on growth.** Its
payload is the whole pending set. It is re-posted when a new entity joins the
set, not when one leaves — a list shrinking because someone installed an
update is not news. The manager's cooldown collapses a burst of releases.

**Configuration is a `surfaces` block, one object per surface.**

```yaml
surfaces:
  configEntries:
    enabled: true
    states: [setup_retry, setup_error, migration_error]
  repairs:
    enabled: true
    severities: [critical, error]
  sensors:
    enabled: true
    deviceClasses: [problem, connectivity]
  updates:
    enabled: false
```

- *Why a knob per surface rather than `enabled` alone?* Each has one axis a
  house plausibly tunes: an install that treats `not_loaded` as a failure, a
  house that wants warning-severity repairs, a sensor class Home Assistant
  adds later. One list each, validated against what Home Assistant defines.
- *Why not more?* Everything else — a noisy integration, a sensor that lies —
  is a rule, and rules exist. A knob that duplicates a rule is a second
  evaluation path, which `includeIntegrations` already showed the cost of.
- *Repairs default to `critical` and `error`.* Warning-severity repairs are
  deprecation notices and version warnings, the same background hum `levels`
  keeps out of the log lane.
- *The chart mirrors the block one to one* under `logsAdapter.source.surfaces`,
  declares every key in the adapter's `configSchema`, and ships one rule per
  surface ahead of the log rules — with `surface=` matchers, so the log rules'
  `message=~` patterns cannot capture a repair whose text happens to say
  "Timeout".

**The `integration` label is resolved per surface.** A config entry names its
domain; a repair names its domain; a sensor's platform comes from the entity
registry, and an entity the registry does not list is labelled by its entity
domain (`binary_sensor`) rather than dropped; the digest is labelled `updates`
so it groups with nothing else.

## Risks / Trade-offs

- **A house with many entities pays a states read a minute.** → A disabled
  sensor surface with updates off issues no states read at all; the registry
  read is a fifth of that again. The cadence is a constant; if it ever needs
  to be a knob, it is one number.
- **Home Assistant's restart churns config entries and connectivity sensors.**
  → The shipped rules dwell both, and rung 1 re-checks the entry's state at the
  close, so a restart's `setup_retry` that loaded is dropped as churn — the
  scenario the spec pins.
- **A repair's text may name the owner** (the automation is theirs, in their
  house). → Self-exclusion's own-user rule drops a record naming the token's
  user; the integration page already says the operator user must be a
  dedicated one, and this is the second reason.
- **A restart re-reports every standing repair.** → Absorbed by cooldown and
  the signature window, and a repair that stood for months already has its
  conversation. Named rather than persisted, because a persisted standing set
  is a state to migrate.
- **The diagram shows a log record.** → Caption generalised; the drawing is
  the log path, which is still true. A redraw is its own change.

## Migration Plan

Deploy is an image tag bump in the bundle's values plus new values with
defaults; an install that sets nothing gets the three default-on surfaces and
no digest. Rollback is the previous tag. No state migrates.
