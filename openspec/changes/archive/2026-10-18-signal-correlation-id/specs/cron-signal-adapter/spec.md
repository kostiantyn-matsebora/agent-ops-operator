# cron-signal-adapter — delta

## MODIFIED Requirements

### Requirement: Firing is restart-safe and idempotent
The adapter SHALL persist each source's last-fired tick through the
contract's state API, so restarts never double-fire a tick and missed ticks
during downtime fire at most once on recovery.

Each fired signal SHALL carry an `initiationId` equal to the source's own
job identity — the source name. All ticks of one job correlate to one
conversation by id equality, with no reliance on any empty-signature
collapse.

An at-least-once re-post of a tick repeats the same id and is absorbed by
the manager's cooldown, or appended as the recurrence it is.

#### Scenario: Restart does not re-fire
- **WHEN** the adapter fires the 06:00 tick, restarts at 06:02, and re-evaluates schedules
- **THEN** the 06:00 tick is not fired again

#### Scenario: Recurring runs share one remembering conversation
- **WHEN** a cron source fires on consecutive days while its conversation is open
- **THEN** the ticks carry the same `initiationId`, land in the same conversation, and later ones resume the agent session as recurrences
