# pipeline-model — delta

## MODIFIED Requirements

### Requirement: Pipeline CRD declares the wiring between sources, channels, and a profile
The `Pipeline` CRD SHALL bind N `signalSourceRefs` and M `channelRefs` to one `profileRef`: signals from every referenced source SHALL become conversations bound to ALL referenced channels with the pipeline's profile, and conversations originated from any referenced channel SHALL be bound to all referenced channels. The Pipeline SHALL also be the SOLE source of its conversations' wiring-level capabilities, via two optional stanzas: `spec.toolsets` (→ `MCPToolset` CRs) and `spec.mcpConfigs` (→ `MCPConfig` CRs), each an ordered ref list. `spec.toolsets` SHALL carry a `mode` (`merge` | `overwrite`, default `merge`) declaring how its tools compose with those the agent's own definition declares — `merge` extends them, `overwrite` replaces them. A Pipeline that declares no bindings gives its conversations no wiring-level capabilities, and nothing supplies a default. A Pipeline SHALL be ADDRESSABLE by name: the task API creates a conversation against a named Pipeline, taking its profile, channels, and capabilities from it. The Pipeline SHALL carry no credentials, no server or tool definitions, and no runtime selection. A reconciler SHALL maintain a `Ready` condition (all references resolve, including toolset and mcpConfig refs) without creating any workload.

#### Scenario: Signals fan out to every pipeline channel
- **WHEN** a Pipeline binds source `alertmanager` to channels `home-ops` and `web` and an alert fires
- **THEN** the resulting conversation carries channel bindings for both `home-ops` and `web` and uses the pipeline's profile

#### Scenario: Dangling references surface on Ready
- **WHEN** a Pipeline references a SignalSource that does not exist
- **THEN** the Pipeline reports `Ready=False` naming the missing reference

#### Scenario: Capabilities bind per route
- **WHEN** two Ready Pipelines route to the same profile with different `toolsets`
- **THEN** conversations from each carry exactly that Pipeline's tools, and the profile declares none

#### Scenario: A mode declares how the route composes with the agent
- **WHEN** a Pipeline binds `toolsets` in `overwrite` mode to a profile whose agent definition declares its own tools
- **THEN** conversations from that route use the Pipeline's tools alone, while a `merge`-mode Pipeline to the same profile extends the agent's

#### Scenario: A Pipeline is addressable by name
- **WHEN** the task API names a Ready Pipeline
- **THEN** the created conversation uses that Pipeline's profile, channel set, and capabilities

#### Scenario: Dangling tooling ref surfaces on Ready
- **WHEN** a Pipeline's `toolsets.refs` or `mcpConfigs.refs` names a CR that does not exist
- **THEN** the Pipeline reports `Ready=False` naming the missing reference
