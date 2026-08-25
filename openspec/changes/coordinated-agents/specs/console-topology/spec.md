## ADDED Requirements

### Requirement: AgentCapabilities and Coordinators are graph nodes

The topology graph SHALL render `AgentCapability` and `Coordinator` as nodes; a
Pipeline's `capabilityRef` and a Coordinator's `agents[]` SHALL be edges to the
Agent, and an AgentCapability with no edge SHALL be shown as unwired, distinct from a
misconfigured node.

#### Scenario: Member edges
- **WHEN** a Coordinator lists three AgentCapabilities
- **THEN** the graph shows three edges from the Coordinator to those AgentCapabilities
