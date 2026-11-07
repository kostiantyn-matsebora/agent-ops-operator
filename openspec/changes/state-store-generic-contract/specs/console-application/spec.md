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
