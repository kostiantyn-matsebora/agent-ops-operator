## Context

See `proposal.md` — Why. What shapes the approach:

- **The adapter already owns every piece the poll needs.** `backfill` reads
  `system_log/list`, sorts by timestamp and feeds records newer than the
  cursor through `consider`; `refreshSnapshot` reads the same listing for the
  dwell ladder's evidence; the cursor is persisted per record through the
  manager's state API. The defect is that the first is called ONCE per
  connect and the second only while something is pending.
- **Home Assistant's listing is a deduplicated map, not a stream.** One entry
  per logger + source location, `count` and `timestamp` advanced on each
  occurrence, `first_occurred` kept, capped at `max_entries` (fifty by
  default). A poll therefore reads at most fifty small objects, and a
  recurring record shows up as an entry whose timestamp moved past the cursor.
- **`system_log/list` is admin-only**, exactly as `subscribe_events` is, so
  the credential the lane already requires is the one the poll needs.
- **The dwell's second rung is built on recurrence** — `stillRecurring` asks
  whether the entry's `lastSeen` fell inside the closing part of the window,
  and `health` asks whether the snapshot's count rose since the closing part
  began. Both were written against a stream of occurrences that never
  arrived.

## Goals / Non-Goals

**Goals:**

- Every matching record, and every recurrence of one, is observed on an
  install whose `system_log` configuration is the default.
- One code path for a record however it arrived; one cursor deduplicating
  the two arrival paths.
- No new configuration key, no new state key, no CRD or chart schema change.

**Non-Goals:**

- Detecting whether `fire_event` is on. Home Assistant exposes no way to ask,
  and the poll makes the answer irrelevant.
- Making the poll interval configurable. It is a constant chosen so that a
  `for: 0` rule waits at most a few seconds longer than it would under the
  event; a knob would invite tuning a number that has no wrong-by-default.
- Re-tuning the shipped rules. A record recurring every nine minutes under a
  ten-minute dwell with a three-minute closing window may still be judged
  quiet — a real concern, but a separate change about the rules, not about
  whether records are seen at all.
- The dedicated-user recommendation is documentation only. Self-exclusion's
  own-user rule is correct; a token minted from the owner's account is a
  configuration the docs must warn against, not a mechanism to weaken.

## Decisions

**Poll on a timer per session, reusing `backfill` as the sweep.** A goroutine
started after `startSession` succeeds, ticking at `pollEvery` (fifteen
seconds), ending with the session. Each tick calls the same routine the
connect-time backfill calls today: read the listing, recover a stale cursor,
sort, consider what is newer than the cursor.

- *Why not a fresh reader?* Two readers over one listing with two ideas of
  "already seen" is how a record gets considered twice or not at all. The
  backfill's cursor logic is already the tested definition of "new to us".
- *Why not extend the dwell flusher, which already reads the listing?* It
  reads only while something is pending, by design — a quiet install issues
  no commands. Making it read always AND consider records would fold two
  jobs with different cadences into one loop; a separate poll loop keeps the
  flusher's contract and lets the two share the READ (below) rather than the
  schedule.

**One read serves both the poll and the snapshot.** The sweep stores the
listing it read as the source's `healthSnapshot` (records half; config
entries are refreshed as before) before considering records. The dwell
flusher's refresh stays, so a pending entry is still verified against a
listing at most five seconds old at the close.

- *Alternative:* two independent reads. Rejected — it doubles the command
  traffic for no precision, and a snapshot older than the record just
  considered is a snapshot that cannot see the recurrence that was just fed in.

**The event path stays and deduplicates through the cursor.** `handleEvent`
is unchanged: it considers the record and advances the cursor to the
record's timestamp. The next poll sees that entry's timestamp equal to the
cursor and skips it. Where events are off, the poll advances the cursor
itself.

- *The ordering hazard, named:* an event for occurrence N+1 arriving while a
  poll is mid-sweep over a listing that already shows N+1. `consider` for the
  same record twice at the same timestamp coalesces in the dwell queue (same
  member key, `countAtOpen` kept from the first sighting) and, for a zero-dwell
  rule, posts twice with one fingerprint — which the manager's cooldown
  absorbs, the same tolerance the post-then-persist cursor already relies on.
  Accepted rather than locked against.

**`backfill: false` seeds the cursor to the newest listed record on connect.**
Its documented meaning is "do not report what was logged while I was down".
Under polling the literal implementation — skip the read — would skip every
read, so the flag's INTENT is preserved by moving the cursor instead.

- *Alternative:* retire the key. Rejected — it is a published config key, and
  its meaning survives intact.

**The poll interval is a constant, `pollEvery = 15s`.** Fifty small objects
every fifteen seconds is negligible for Home Assistant and for the adapter;
a `for: 0` rule gains at most that much latency, and every dwelled rule is
measured in minutes. `dwellTick` stays at five seconds.

## Risks / Trade-offs

- **A record that recurs faster than the poll reads it is seen once per poll,
  not once per occurrence.** → The dwell's evidence is the listing's own
  `count`, which is exact; only `levelCounts` in the rendered evidence
  under-counts, and it is prose, not a decision.
- **Home Assistant's listing is capped at `max_entries`.** A burst of more
  than fifty distinct records between two polls loses the oldest. → The
  listing was already the backfill's only source, so the cap was already the
  bound; the poll narrows the window it applies to from "since the last
  reconnect" to fifteen seconds.
- **Clock skew between Home Assistant and the adapter.** → The cursor compares
  Home Assistant's timestamps against each other only; the adapter's clock
  enters only through `cursorStaleSkew`, unchanged.
- **The listing read fails on a tick.** → Logged once per failure as today's
  backfill failure is, snapshot left in place, next tick retries. A session
  whose reads keep failing already ends through the ping loop.

## Migration Plan

Deploy is an image tag bump in the bundle's values. No state migrates: the
cursor key is unchanged, so an upgraded adapter resumes from where the old
one stopped and its first poll reports what was logged in between. Rollback
is the previous tag.
