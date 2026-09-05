## Why

**The Home Assistant log lane delivers nothing on a default Home Assistant
install, and reports itself healthy while it does not.** The adapter learns of
live records by subscribing to `system_log_event`, and Home Assistant fires
that event only when `system_log: fire_event: true` is in its configuration —
the default is `false`, and nothing in this repository (the adapter, the
bundle, the integration page) says so. On such an install the adapter's only
sight of the log is the one `system_log/list` read it makes at connect time.
Every record it ever considers is therefore seen once, at a reconnect, and
never again — so the dwell judges each one "quiet" and drops it, while the
TCP session stays established and the source reports `Ready=True`.

Measured on the reference install on 2026-09-05: Home Assistant logged a ring
camera stream timeout 42 times over six and a half hours, matching the shipped
Timeout rule; the adapter posted nothing all day, and its log holds three
lines, each seconds after a reconnect, each a "dropped as quiet".

## What Changes

- **The adapter observes the log by POLLING `system_log/list`**, on a fixed
  short interval, feeding every record newer than its cursor through the same
  path a live event takes — exclusion, scope, rules, inhibition, dwell. Home
  Assistant deduplicates its log by logger and source location and advances
  the entry's timestamp and count on every recurrence, so a poll sees a
  recurring record as a newer one, which is exactly what the dwell's
  "still recurring at the close" rung needs.
- **The event subscription stays as a fast path.** Where `fire_event` is on,
  a record arrives within milliseconds and advances the cursor, so the next
  poll does not consider it again. The two paths deduplicate through the
  cursor they already share; no second bookkeeping is added.
- **The connect-time backfill becomes the first poll**, and `backfill: false`
  keeps its meaning — do not report what was logged while the adapter was
  down — by moving the cursor to the newest record on connect instead of
  considering the listing.
- **Not breaking.** No config key changes, no CRD, no chart value beyond the
  adapter's image tag. An install that already set `fire_event: true` keeps
  the latency it had.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `ha-signal-adapter`: the requirement "The adapter resumes where it stopped"
  gains the observation model it was silently assuming — records are observed
  by polling the listing, the event is an optional fast path, and a record's
  recurrence is observed whether or not Home Assistant fires events.

## Impact

**Code**

- `signals/ha/main.go` — a per-session poll loop over `system_log/list`,
  sharing the read with the dwell's snapshot refresh; the connect-time
  backfill becomes the first poll; `backfill: false` seeds the cursor
  instead of skipping the read.
- `signals/ha/adapter_test.go`, `fakeha_test.go` — a fake Home Assistant that
  fires no events at all, and tests proving a record and its recurrence are
  observed through the poll alone, that an event-delivered record is not
  considered twice, and that `backfill: false` still polls.
- `chart/charts/home-assistant/values.yaml` — the pinned `signal-ha` image
  tag moves to the version carrying the fix.

**Reference docs**

- `docs/CHANGELOG.md` — an Unreleased entry naming the new `signal-ha` tag
  and the defect it fixes.
- `.claude/rules/structure.md` — the `signals/ha/` entry says the adapter
  reads `system_log_event` with `system_log/list` "for backfill"; it now
  polls, and the entry must say so or the next reader re-derives the event
  as the primary path.
- `.claude/rules/chart.md` — the operator-credential paragraph names
  `subscribe_events` as the admin-only command that makes the ingest lane
  need the operator token; `system_log/list` is admin-only too, and the
  paragraph should name the command the lane now depends on.

**The adopter site**

- `docs/integrations/home-assistant.md` — the callout on the operator token
  names `subscribe_events`; it gains `system_log/list`. The "What differs
  from the cluster lane" table gains a row on how records are observed
  (polled, with `fire_event: true` as an optional latency improvement), and
  the "which two users to mint" section says the operator user should be a
  DEDICATED admin user, because the adapter drops any record whose text
  names its own user — a token minted from the household owner's account
  silences every error mentioning them.
- The landing page, Introduction, Getting started, Installation and the
  guides do not describe how the Home Assistant lane observes records, so
  none of them is made untrue. Checked, not assumed.

**Not affected**

- The rule vocabulary, the dwell ladder, self-exclusion, the cursor's state
  key, the bundle's templates and the CRDs.
