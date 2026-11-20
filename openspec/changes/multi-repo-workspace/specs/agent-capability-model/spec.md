## MODIFIED Requirements

### Requirement: An AgentCapability declares a capability and no wiring

The `AgentCapability` CRD SHALL carry exactly the capability fields a Pipeline
carries inline — `profileRef`, `runtimeRef`, `serviceAccountName`, `toolsets`,
`mcpConfigs`, `persistence`, and the workspace fields `repositories`,
`agentFile` and `workingDir` — with the same semantics, defaults and validation
each has on a Pipeline. It SHALL carry no `signalSourceRefs`, no `channelRefs`
and no other field that reaches a signal or a surface.

#### Scenario: The capability fields validate as on a Pipeline
- **WHEN** an AgentCapability names a `runtimeRef` no `AgentRuntime` backs
- **THEN** its `Ready` condition is False naming the ref, exactly as a Pipeline's would be

#### Scenario: Wiring fields are pruned
- **WHEN** an AgentCapability manifest carries `signalSourceRefs` or `channelRefs`
- **THEN** the API server silently PRUNES the unknown field rather than rejecting it — structural CRDs with no `additionalProperties: false` prune, they do not error
- **AND** the stored object therefore carries no wiring whatever the manifest wrote

## ADDED Requirements

### Requirement: A capability's repository fields resolve as multi-repo-workspace states

An `AgentCapability`'s `repositories`, `agentFile` and `workingDir` SHALL resolve against the profile's as the requirement "A capability's list replaces the profile's list" in `multi-repo-workspace` states, and SHALL behave identically whether declared inline on a `Pipeline` or `Coordinator` or on a referenced `AgentCapability`.

#### Scenario: Inline and referenced agree

- **WHEN** a Pipeline declares `repositories` inline and another references an AgentCapability with the same ones
- **THEN** both resolve to the same list
