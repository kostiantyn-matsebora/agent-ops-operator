## MODIFIED Requirements

### Requirement: Snapshots are authoritative and the stream carries cursors
The console SHALL serve a snapshot endpoint per view and a single SSE stream carrying CR deltas, activity events and transcript appends, each with a monotonic cursor. The browser SHALL treat snapshots as authoritative and re-fetch on RESYNC; a missed event SHALL cost staleness, never a wrong screen. First connect and reconnect SHALL follow the same path.

**A DELTA CARRIES ITS OBJECT, AND IS APPLIED.** An event SHALL carry the changed object as the console would have fetched it, and the browser SHALL apply it to what it already holds. Re-fetching on every delta was the mechanism, and it cost a request and a blank page per change while the answer was already on the wire.

A snapshot stays AUTHORITATIVE, which is what makes applying safe: a resync replaces applied state wholesale, so an applier that is ever wrong is corrected by the next reconnect rather than persisting.

#### Scenario: A sleeping tab converges
- **WHEN** a browser is disconnected while many changes occur, then reconnects
- **THEN** it receives a resync, re-fetches snapshots, and its rendered state equals a cold load

#### Scenario: A delta updates the view without a request
- **WHEN** a watched object changes while a view holding it is open
- **THEN** the view updates from the event, and no snapshot is re-fetched

#### Scenario: The wire format survives CRD evolution
- **WHEN** a CRD gains fields
- **THEN** the browser renders the fields it knows and ignores the rest, exactly as it does from a snapshot
