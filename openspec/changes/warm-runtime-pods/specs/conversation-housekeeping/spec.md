## ADDED Requirements

### Requirement: A reservation keeps the orphan ordering true
A reservation's workspace and context directories SHALL be created only after
its `Reserved` conversation exists, so the ordering that identifies an orphan —
the conversation predates its directory — holds for warm pods without any
change to the reclaiming job. A reservation evicted unused SHALL be deleted as
an object, after which its directories are ordinary orphans.

#### Scenario: A waiting reservation is not reclaimed
- **WHEN** the reclaiming job runs while a reservation waits
- **THEN** the reservation's workspace directory is left alone

#### Scenario: An evicted reservation's directory is reclaimed
- **WHEN** a reservation is evicted and its conversation object deleted
- **THEN** the next run removes its workspace directory
