## ADDED Requirements

### Requirement: An AgentCapability is never addressable from a surface

`/pipelines` and every discovery path SHALL list Ready Pipelines and Ready
Coordinators, each with its addressed form, and nothing else. An `AgentCapability` SHALL have no chat command and
SHALL appear in no menu; reaching one from a surface is wrapping it in a
Pipeline.

#### Scenario: AgentCapabilities absent from the menu
- **WHEN** a surface's discovery list is built
- **THEN** it names no AgentCapability, whether or not one is wired
