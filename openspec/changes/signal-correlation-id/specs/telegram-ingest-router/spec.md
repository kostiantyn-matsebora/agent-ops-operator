# telegram-ingest-router — delta

## MODIFIED Requirements

### Requirement: Offset persistence is delegated downstream
The router SHALL own the in-flight offset value but SHALL NOT own its
storage. It SHALL obtain the offset at startup and report each confirmed
offset downstream, where a receiving adapter persists it through the
existing adapter state API.

Because storage never moves, replacing the router workload SHALL NOT lose
the cursor.

#### Scenario: Restart resumes from the persisted offset
- **WHEN** the router restarts
- **THEN** it resumes from the persisted offset rather than re-reading the full
  backlog

#### Scenario: A replayed batch is harmless
- **WHEN** the router crashes before an offset is persisted and replays a batch
- **THEN** each replayed update yields the same `initiationId` as its first delivery, so routing converges instead of duplicating conversations

#### Scenario: Re-modelling the router keeps the cursor
- **WHEN** the router moves from an adapter CR to a chart-owned Deployment
- **THEN** the new workload resumes from the same persisted offset, because the
  offset lives in the channel adapter and was never the router's to hold
