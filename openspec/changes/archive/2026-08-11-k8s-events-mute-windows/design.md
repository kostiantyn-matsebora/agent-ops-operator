## Context

The events adapter already suppresses along three axes, and its config is
deliberately split by whose vocabulary it borrows:

- **`rules`** — Prometheus: `matchers` plus `for:` dwell, first-match-wins.
- **`route.inhibitRules`** — Alertmanager: `sourceMatchers` / `targetMatchers` /
  `equal`, suppressing consequences of a reported cause.

The evaluation order is: self-exclusion → rule match → inhibition → dwell queue
(`pending.go`) → emit cap (`emitcap.go`) → emit.

The standing rule about that split is explicit in `CLAUDE.md`: *"`for:` is
Prometheus, `group_wait` is Alertmanager, and they are NOT the same thing."*
Spelling a borrowed concept with a near-synonym is named there as a mistake
already paid for.

A scheduled maintenance window fits none of the three axes, for reasons worth
recording because each looks like it might work:

| Mechanism | Why it cannot express this |
|---|---|
| `for:` dwell | the condition genuinely holds for 10–15 minutes; a dwell that outlasts it delays every real incident all day |
| inhibition | needs a *cause event* to key on; a router losing power produces no in-cluster object, only consequences |
| matchers | select on labels, and there is no label for the time of day |

Alertmanager models exactly this as `time_intervals` + `mute_time_intervals`.

## Goals / Non-Goals

**Goals:**

- Express "do not wake anyone about this cluster between 04:00 and 04:20 local".
- Keep a real problem that outlives the window reportable.
- Make an active mute visible, so silence is never mistaken for calm.
- Borrow Alertmanager's schema and semantics rather than approximate them.

**Non-Goals:**

- Ad-hoc muting ("silence this for 30 minutes now"). That wants a write path into
  a running adapter; the config is declarative.
- Muting manager-side. Suppression is the adapter's job and grouping is the
  manager's; this change does not blur that line.
- Silencing the router problem itself. If connectivity loss is worth knowing
  about, that is a different lane's job — this mutes the *cluster's reaction* to
  a known event.

## Decisions

### D1: Alertmanager's vocabulary, schema and semantics — not an approximation

`route.timeIntervals` defines named intervals with Alertmanager's fields
(`times` with `startTime`/`endTime`, `weekdays`, `daysOfMonth`, `months`,
`years`, `location`), and `route.muteTimeIntervals` references them by name.

*Why:* the config's whole organising principle is that `rules` speaks Prometheus
and `route` speaks Alertmanager, and this concept is already named and specified
in the half it belongs to. An operator who has written an Alertmanager silence
window can write this one; more importantly, one who reads it will not have to
ask whether "window" here means what it means there.

*Alternatives:* a bespoke `blackout: {from, to, days}` (rejected — a smaller
surface that is a different thing wearing a familiar shape, which is the exact
failure the `for:`/`group_wait` rule exists to prevent); a cron expression
(rejected — cron names instants, not intervals, so an end time has to be
inferred from a duration, and "is now inside the window" becomes a computation
rather than a lookup).

### D2: Mute is evaluated at EMIT, after the dwell

The check sits between the dwell queue and the emit cap.

*Why:* this is what Alertmanager's mute intervals mean — they mute
*notifications*, not evaluation — and it produces the behaviour that makes the
feature safe to use:

- An event **during** the window whose problem is transient is muted and gone.
- An event during the window whose problem **persists** keeps generating events
  after the window closes, so it dwells and emits then. The window delays a real
  incident by roughly one dwell rather than hiding it.
- An event that arrives just **before** the window and confirms inside it is
  muted. That is the correct reading of "do not tell me during this window".

Evaluating at ingest instead would drop events out of the dwell queue entirely,
so a genuine problem starting at 04:05 would need a fresh event after 04:20 to be
noticed at all — the same outcome in the common case, and a strictly worse one
when the cluster stops repeating itself.

### D3: A mute entry may carry `matchers`

`muteTimeIntervals` entries accept optional `matchers`, narrowing what the window
silences; without them the window mutes everything from that source.

*Why:* the real hazard of this feature is going deaf. A router restart produces
connectivity-shaped reasons (`NodeNotReady`, probe failures, evictions); it does
not produce `OOMKilling`. Letting the window name what it expects means an
operator can keep hearing about everything else, and the mitigation does not
depend on the window being short or on anyone remembering to review it.

*Alternative:* mute everything, keep windows narrow (rejected as the only option
— it makes the safe configuration the one that requires discipline).

### D4: `location` is an IANA timezone and is not optional in practice

The field defaults to UTC, as Alertmanager's does, but the chart's shipped
example names a zone and the documentation states the consequence.

*Why:* "four in the morning" is a local fact. A window pinned to UTC drifts by an
hour at each DST transition, which means it stops covering the outage on a date
nobody chose, at an hour nobody is watching. This is the single most likely thing
to get quietly wrong, so it gets a decision rather than a default.

### D5: Muting is reported, never silent

While a window is active the source's `Ready` condition says so, naming the
interval. When the window closes the adapter reports how many events it muted.

*Why:* the adapter already does exactly this for emit-cap clipping — *"Clipping
SHALL NOT be silent"* — and the reasoning transfers directly. A muted lane and a
quiet lane are indistinguishable from outside, and only one of them means the
cluster is healthy. `kubectl get signalsource` should answer "why has nothing
arrived since four?" without anyone reading adapter logs.

Muted events do not consume the emit cap: they were never emitted, and charging
them would make a mute window look like a runaway.

### D6: Windows are per SOURCE

The config lives in `SignalSource.spec.config.route`, alongside the inhibition
rules it sits beside.

*Why:* it is where the rest of suppression already lives, it keeps the adapter
free of global state, and two lanes served by the same adapter can legitimately
want different windows — a production lane and a lab lane do not share a
maintenance schedule.

## Risks / Trade-offs

- **A real incident during the window is lost** → this is what a mute is, and
  pretending otherwise would be the wrong design. Three things bound it: D2 means
  anything persistent resurfaces after the window, D3 means the window can be
  narrowed to the reasons the outage produces, and D5 means the mute is visible
  rather than assumed.
- **A misconfigured window mutes far more than intended** → the `Ready` condition
  reports an active mute, so a lane muted all day says so on
  `kubectl get signalsource` instead of looking idle.
- **DST moves the window** → `location`, documented with the consequence, and an
  example that is not UTC.
- **Overlapping intervals** → union, as Alertmanager does; muted if any interval
  matches. Stated so it is not discovered.
- **Config validation is adapter-side** → an unparseable interval must fail the
  source's Ready condition with the reason, not be silently ignored, or a typo
  becomes an unnoticed window that never fires. Same posture as the existing
  config handling.

## Migration Plan

1. Additive: absent `timeIntervals` / `muteTimeIntervals`, behaviour is identical.
2. The bundle ships the keys empty with a worked nightly example in comments.
3. Verification is a clock test, not a wait: the interval matcher takes an
   injected `now`, so "inside the window", "outside", "spanning midnight" and
   "across a DST transition" are unit tests rather than an overnight vigil.
4. Rollback: remove the keys.

## Open Questions

- **Should `activeTimeIntervals` (notify only during a window) be supported too?**
  It is the same matcher with the condition inverted, and "only page me during
  working hours" is a real request. Left out of this change to keep the surface
  one concept, but it is nearly free once the matcher exists.
- **Should a window spanning midnight be expressed as one interval or two?**
  Alertmanager requires `startTime` < `endTime` within a day, so `23:50–00:10`
  is two entries. Following that exactly is consistent but is the sharpest edge
  in the schema; the alternative is accepting a wrapping range and normalising it,
  which is friendlier and a divergence.
- **Should muted events be counted per rule, or only per source?** Per rule is
  more useful when diagnosing an over-broad window, and more state to hold.
