## MODIFIED Requirements

### Requirement: One service fans in every source the browser must not touch
The console SHALL be a single Go service that aggregates Kubernetes list/watch of configuration, the manager's state read surface for conversations and signals, the manager's activity stream, install facts (Deployments and pods), and the manager's channel and signal contracts, and serves a browser API plus the frontend.

The browser SHALL NOT hold Kubernetes credentials or reach the Kubernetes API, the manager, or the activity stream directly.

The service SHALL make no write to the Kubernetes API. Its only writes SHALL be `POST /channel/inbound` (reply) and `POST /signal/inbound` (originate), both through existing contracts.

#### Scenario: One upstream stream serves many browsers
- **WHEN** several browsers are connected
- **THEN** the console holds one activity-stream connection to the manager and multiplexes it, rather than one upstream connection per browser

#### Scenario: Two state feeds serve many browsers
- **WHEN** several browsers are connected
- **THEN** the console holds one conversation feed and one signal feed to the manager and multiplexes them, and resynchronises by relisting when the manager says its resume point is unknown

#### Scenario: No write path to the cluster
- **WHEN** any console endpoint is exercised, including origination and chat
- **THEN** no Kubernetes object is created, updated or deleted by the console

### Requirement: Snapshots are authoritative and the stream carries cursors
The console SHALL serve a snapshot endpoint per view and a single SSE stream carrying configuration deltas, state deltas from the manager's state read surface, activity events and transcript appends, each with a monotonic cursor.

The browser SHALL treat snapshots as authoritative and re-fetch on RESYNC. A missed event SHALL cost staleness, never a wrong screen. First connect and reconnect SHALL follow the same path.

**A DELTA CARRIES ITS OBJECT, AND IS APPLIED.** An event SHALL carry the changed object as the console would have fetched it, and the browser SHALL apply it to what it already holds.

Re-fetching on every delta was the mechanism, and it cost a request and a blank page per change while the answer was already on the wire.

A snapshot stays AUTHORITATIVE, which is what makes applying safe. A resync replaces applied state wholesale, so an applier that is ever wrong is corrected by the next reconnect rather than persisting.

#### Scenario: A sleeping tab converges
- **WHEN** a browser is disconnected while many changes occur, then reconnects
- **THEN** it receives a resync, re-fetches snapshots, and its rendered state equals a cold load

#### Scenario: A delta updates the view without a request
- **WHEN** a watched object changes while a view holding it is open
- **THEN** the view updates from the event, and no snapshot is re-fetched

#### Scenario: The wire format survives CRD evolution
- **WHEN** a CRD gains fields
- **THEN** the browser renders the fields it knows and ignores the rest, exactly as it does from a snapshot
