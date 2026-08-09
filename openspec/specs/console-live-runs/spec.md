# console-live-runs Specification

## Purpose
TBD - created by archiving change visualize-agent-ops. Update Purpose after archive.
## Requirements
### Requirement: Real-time run activity per pipeline
The console SHALL overlay live Conversation activity on the topology and provide a runs view per pipeline: for each Conversation — phase, inflight work (when `status.inflight` is set), bounded run history (`status.runs[]`, newest last), thread bindings, runtime pod name, and last activity — updated in real time from Conversation watch events pushed to browsers over a streaming connection.

#### Scenario: Inflight run is visible live
- **WHEN** a Conversation's `status.inflight` is set while a work unit executes
- **THEN** connected browsers show that conversation as running on its pipeline without a page refresh, and the indicator clears when the run completes

#### Scenario: Run history with results
- **WHEN** a user opens a Conversation's runs view
- **THEN** each entry from `status.runs[]` is shown with its outcome and result content as recorded by the manager

### Requirement: Activity badges on the topology
Pipeline nodes SHALL carry live activity badges (counts of active and recent Conversations) derived from the Conversation cache, so the topology answers "what is running right now" at a glance.

#### Scenario: Idle vs active pipelines distinguishable
- **WHEN** one pipeline has two conversations with inflight work and another has none
- **THEN** the first pipeline node shows an active count of 2 and the second shows idle

### Requirement: Streamed updates degrade to snapshot + reconnect
Browser update streams SHALL be resumable: on disconnect, the client re-fetches a consistent snapshot and re-subscribes; the server SHALL NOT require clients to have observed every intermediate event for correctness (CR state is authoritative, events are deltas).

#### Scenario: Browser sleeps and returns
- **WHEN** a browser tab reconnects after minutes offline
- **THEN** it loads the current snapshot and resumes streaming, showing correct current state with no duplicated or stuck entries

