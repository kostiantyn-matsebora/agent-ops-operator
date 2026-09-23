## ADDED Requirements

### Requirement: Adapter CRs may declare the external systems they face

A `SignalAdapter` or `ChannelAdapter` MAY declare, as interface metadata beside
`configSchema` and `credentialKeys`, the external systems its implementation
talks to: for each, a name and a kind, such as a sender, a transport API or
the Kubernetes API.

The declaration SHALL be metadata only. The manager SHALL read no config to
verify it and SHALL grant nothing from it.

The console SHALL draw a declared external as a node beside the adapter,
verbatim, and SHALL draw none for an adapter that declares nothing. The
bundles SHALL declare the externals of the adapters they ship.

#### Scenario: A declared sender is drawn
- **WHEN** a SignalAdapter declares that it faces an alert manager
- **THEN** the Components view draws that system as an external node with an edge to the adapter, and the Infrastructure view draws it beside the adapter's pod

#### Scenario: Nothing is declared
- **WHEN** an adapter CR declares no externals
- **THEN** the adapter draws with no external node, and nothing else about it changes

#### Scenario: The declaration grants nothing
- **WHEN** an adapter declares the Kubernetes API as an external
- **THEN** no RBAC, no network policy and no credential follows from the declaration
