# cron-signal-adapter (delta)

## ADDED Requirements

### Requirement: A fired tick carries a signalId

The adapter SHALL send a `signalId` on every signal it posts, per
`signal-adapter-contract`. A message here is one fired tick of one source.

The `signalId` is `cron:<source>:<tick>`, the tick as RFC 3339 UTC. A crash between firing and persisting the cursor re-fires the same tick under the same id.

The id is stable across a re-send of the same delivery, including the
re-send after a `5xx`. It differs between two messages, even when they share
an `initiationId`.

#### Scenario: A re-fired tick repeats its id

- **WHEN** the adapter fires the 06:00 tick, crashes before persisting the cursor, and fires it again
- **THEN** both posts carry the same `signalId`, and the 06:00 tick of the next day carries a different one
