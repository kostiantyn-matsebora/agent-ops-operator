## ADDED Requirements

### Requirement: A SignalAdapter declares whether it may read state
The `SignalAdapter` CRD SHALL carry an optional boolean `spec.stateAccess`, defaulting to false. When true, the adapter's derived token SHALL open the manager's state read surface. When false or absent, that token SHALL be refused by the read surface, which names the adapter and the field. The field SHALL grant nothing else: it SHALL NOT change the adapter's Kubernetes identity or its other manager scopes.

#### Scenario: Silence grants nothing
- **WHEN** a SignalAdapter does not set `stateAccess`
- **THEN** its derived token is refused on the state read surface

#### Scenario: Declaring the reach opens only the read surface
- **WHEN** a SignalAdapter sets `stateAccess: true`
- **THEN** its derived token is accepted on the state read surface and no other scope of that adapter changes
