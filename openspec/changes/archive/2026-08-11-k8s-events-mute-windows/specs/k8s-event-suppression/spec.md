## ADDED Requirements

### Requirement: Suppression has a time axis, spelled in Alertmanager's vocabulary
The adapter SHALL support named time intervals and mute references on a source's `route`, using Alertmanager's schema and field names — intervals of `times` (`startTime`/`endTime`), `weekdays`, `daysOfMonth`, `months`, `years` and a `location`, referenced by name from `muteTimeIntervals`.

The names and semantics SHALL be borrowed exactly rather than approximated. `rules` speaks Prometheus and `route` speaks Alertmanager, and a concept already specified in the half it belongs to SHALL NOT be re-spelled: a familiar word meaning something slightly different is worse than an unfamiliar one.

`location` SHALL be an IANA timezone. A window expressed without one is interpreted as UTC and will drift by an hour at each daylight-saving transition — stopping cover of the outage it was written for, on a date nobody chose.

Overlapping intervals SHALL union: an event is muted when any referenced interval matches.

#### Scenario: A nightly maintenance window is expressible
- **WHEN** a source declares an interval of 04:00–04:20 on all weekdays in a named location and references it from `muteTimeIntervals`
- **THEN** events that would emit inside that local window are muted

#### Scenario: Local time is honoured across a DST transition
- **WHEN** a window names an IANA location and the zone's offset changes
- **THEN** the window still covers the same local clock times

#### Scenario: An unparseable interval fails the source loudly
- **WHEN** a source declares a time interval that cannot be parsed
- **THEN** the source reports it on its Ready condition with the reason, rather than ignoring the entry and leaving a window that never fires

### Requirement: Mute is evaluated at emit, so a persistent problem still surfaces
Muting SHALL be evaluated after the dwell window and before emission, mirroring Alertmanager's mute intervals, which mute notifications rather than evaluation.

A matched event whose problem does not outlive the window SHALL be muted and discarded. A problem that persists past the window SHALL still be reported, because the cluster continues producing events for anything genuinely broken and the next one dwells and emits normally once the window has closed.

Muted events SHALL NOT count against the source's emit cap: they were never emitted, and charging them would make a mute window read as a runaway.

#### Scenario: Transient noise inside the window is dropped
- **WHEN** a scheduled outage produces events that stop when it ends
- **THEN** no signal is emitted for them

#### Scenario: A problem outliving the window is reported after it
- **WHEN** a failure begins during a mute window and is still producing events after it closes
- **THEN** a signal is emitted once a post-window event completes its dwell

#### Scenario: An event confirmed inside the window is muted
- **WHEN** an event arrives shortly before the window and its dwell completes inside it
- **THEN** it is muted, because the request was not to be told during the window

#### Scenario: Muting does not consume the emit budget
- **WHEN** a window mutes many events
- **THEN** the source's emit cap is unaffected and no clipping is reported for them

### Requirement: A mute window may be narrowed by matchers
A `muteTimeIntervals` entry MAY carry `matchers`, restricting what the window silences. An entry with no matchers SHALL mute everything from that source for the interval's duration.

Going deaf for the length of the window is the principal hazard of this feature. Narrowing it to the reasons a scheduled outage actually produces SHALL be available, so the safe configuration does not depend on the window being short or on anyone reviewing it later.

#### Scenario: A window silences the outage, not everything
- **WHEN** a window carries matchers selecting connectivity-related reasons
- **THEN** those events are muted inside the window and an unrelated reason still emits

#### Scenario: A window without matchers mutes the source
- **WHEN** a window declares no matchers
- **THEN** every event from that source is muted for its duration

### Requirement: An active mute is visible on the source
While a mute window is active the adapter SHALL report it on that source's `Ready` condition, naming the interval, and SHALL report how many events were muted.

Muting SHALL NOT be silent. A muted lane and an idle lane are indistinguishable from outside and only one of them means the cluster is healthy, so "why has nothing arrived since four?" SHALL be answerable from the source object rather than from adapter logs.

#### Scenario: Silence is explained
- **WHEN** a mute window is active
- **THEN** the source's Ready condition names the interval that is muting it

#### Scenario: The cost of the window is reported
- **WHEN** a mute window closes
- **THEN** the number of events it muted is reported, so an over-broad window is visible rather than inferred
