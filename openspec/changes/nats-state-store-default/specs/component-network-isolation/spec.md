## ADDED Requirements

### Requirement: The state server admits the manager and no other pod
Where the `nats` backend is selected and network restriction is enabled (the
chart's existing network policy toggle), the chart SHALL admit ingress to the
state server's client port from the manager's pods alone. It SHALL NOT admit
a runtime pod, an adapter, the console or a housekeeping job, which read state
through the manager's `/state/*` surface. The server's peer port SHALL admit
only its own pods.

#### Scenario: A runtime pod cannot dial the state server
- **WHEN** restriction is enabled and a runtime pod connects to the state
  server's client port
- **THEN** the connection is refused

#### Scenario: Every wired flow is allowed
- **WHEN** the manager dials the state server
- **THEN** the connection succeeds, and the flow table test lists it
