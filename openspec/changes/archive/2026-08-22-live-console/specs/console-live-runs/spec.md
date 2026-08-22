## MODIFIED Requirements

### Requirement: Streamed updates degrade to snapshot + reconnect
Browser update streams SHALL be resumable: on disconnect, the client re-fetches a consistent snapshot and re-subscribes; the server SHALL NOT require clients to have observed every intermediate event for correctness (CR state is authoritative, events are deltas).

WHILE CONNECTED, an event SHALL be applied to what the browser holds rather than triggering a re-fetch. Re-fetching SHALL be reserved for the cases the stream cannot serve — first load, a resync, an explicit action, and a value that decays with time rather than with change.

A cursor the manager cannot serve — evicted from the bounded ring, or predating the current manager process — SHALL be answered with a resync boundary rather than a shorter list the client would mistake for continuity. The console SHALL render that boundary as an explicit gap in the activity timeline, and SHALL NOT present a post-restart window as a period in which nothing happened. Configuration and conversation views SHALL be unaffected, since both re-read authoritative state.

#### Scenario: Browser sleeps and returns
- **WHEN** a browser tab reconnects after minutes offline
- **THEN** it loads the current snapshot and resumes streaming, showing correct current state with no duplicated or stuck entries

#### Scenario: A run's progress arrives as content
- **WHEN** a run advances while its conversation is open
- **THEN** the view updates from the event, without re-fetching and without a loading state

#### Scenario: Manager restart shows a gap, not silence
- **WHEN** the console reconnects after a manager restart with a cursor the new process cannot serve
- **THEN** the timeline shows an explicit gap marking lost history, and current conversations, topology and configuration still render correctly

#### Scenario: Console restart needs no stored state
- **WHEN** the console pod restarts
- **THEN** it rebuilds configuration from list/watch and activity by cursor replay, mounting no volume and losing no authoritative state
