## ADDED Requirements

### Requirement: Agents and Coordinators are graph nodes

The topology graph SHALL render `Agent` and `Coordinator` as nodes; a
Pipeline's `agentRef` and a Coordinator's `agents[]` SHALL be edges to the
Agent, and an Agent with no edge SHALL be shown as unwired, distinct from a
misconfigured node.

#### Scenario: Member edges
- **WHEN** a Coordinator lists three Agents
- **THEN** the graph shows three edges from the Coordinator to those Agents
