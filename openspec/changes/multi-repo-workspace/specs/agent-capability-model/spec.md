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

### Requirement: A capability's repositories replace the profile's

An `AgentCapability` that declares a non-empty `repositories` SHALL replace the list the profile declares for the conversation, never merge with it, exactly as `multi-repo-workspace` states. Its `agentFile` and `workingDir` SHALL travel with the list, so a capability that replaces the list also replaces them. The inline capability on a `Pipeline` or `Coordinator` SHALL behave identically, and neither kind SHALL carry a repository field of its own.

#### Scenario: The replacement is wholesale

- **WHEN** a profile declares `agentFile` against `app` and a capability declares `repositories: [docs]` and no `agentFile`
- **THEN** the resolved declaration has `docs` and no agent file

#### Scenario: Inline and referenced agree

- **WHEN** a Pipeline declares `repositories` inline and another references an AgentCapability with the same ones
- **THEN** both resolve to the same list

