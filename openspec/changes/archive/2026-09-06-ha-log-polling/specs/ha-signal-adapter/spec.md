## MODIFIED Requirements

### Requirement: The adapter resumes where it stopped
The adapter SHALL observe Home Assistant's log by reading the system log
listing on a short fixed interval, and SHALL NOT depend on Home Assistant
firing `system_log_event`: that event is off by default, and an install that
never enabled it MUST still have every matching record observed, including
every RECURRENCE of a record — Home Assistant's listing advances a
deduplicated entry's timestamp and count on each occurrence, and the adapter
SHALL treat an entry newer than its read position as an occurrence to
consider. The live event, where the instance fires it, SHALL be accepted as
a lower-latency path for the same records, and a record delivered by one
path SHALL NOT be considered again by the other.

The adapter SHALL persist its read position through the manager's signal state
API and SHALL resume from it on restart, so a restart neither replays what it
already reported nor skips what arrived while it was down. A position the
upstream no longer accepts SHALL cause a full re-read rather than a stall.
Where the configuration declines the backfill, the adapter SHALL move its read
position past the listing on connect rather than stop observing.

#### Scenario: Restart resumes
- **WHEN** the adapter restarts
- **THEN** it resumes from its persisted position and does not replay already-reported conditions

#### Scenario: Stale position recovers
- **WHEN** the persisted position is no longer valid upstream
- **THEN** the adapter re-reads from the current position and continues, rather than failing repeatedly

#### Scenario: A default install is observed
- **WHEN** Home Assistant never fires `system_log_event` and a matching record appears in its listing after the adapter connected
- **THEN** the adapter observes the record within one poll interval and it enters the rule path exactly as a live event would

#### Scenario: Recurrence is observed without events
- **WHEN** Home Assistant fires no events and a record already observed recurs, so the listing's entry carries a later timestamp and a higher count
- **THEN** the adapter observes the recurrence, and a dwell rule with no health predicate can find the record still recurring at the close

#### Scenario: A record is not counted twice
- **WHEN** the instance fires `system_log_event` and a record arrives through it
- **THEN** the next poll does not consider that occurrence again

#### Scenario: Declining the backfill still polls
- **WHEN** the source's configuration sets `backfill: false`
- **THEN** records present at connect time are not reported, and records logged after the connect are still observed
