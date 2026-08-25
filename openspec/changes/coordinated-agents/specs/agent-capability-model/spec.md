## Purpose
The `AgentCapability` CRD is the CAPABILITY kind: what an agent is and what power it holds, declared once and wired by nothing on the object itself.

## ADDED Requirements

### Requirement: An AgentCapability declares a capability and no wiring

The `AgentCapability` CRD SHALL carry exactly the six capability fields a Pipeline
carries inline — `profileRef`, `runtimeRef`, `serviceAccountName`, `toolsets`,
`mcpConfigs`, `persistence` — with the same semantics, defaults and validation
each has on a Pipeline. It SHALL carry no `signalSourceRefs`, no `channelRefs`
and no other field that reaches a signal or a surface.

#### Scenario: The capability fields validate as on a Pipeline
- **WHEN** an AgentCapability names a `runtimeRef` no `AgentRuntime` backs
- **THEN** its `Ready` condition is False naming the ref, exactly as a Pipeline's would be

#### Scenario: Wiring fields are refused
- **WHEN** an AgentCapability manifest carries `signalSourceRefs` or `channelRefs`
- **THEN** the API server rejects it as an unknown field

### Requirement: An unwired AgentCapability is inert

An AgentCapability that no Pipeline references and no Coordinator lists SHALL originate
nothing, be reachable by nothing, and report no condition about it. Being
unwired is a configuration, not a defect.

#### Scenario: Nothing reaches an unlisted AgentCapability
- **WHEN** an AgentCapability exists and nothing references it
- **THEN** no signal, chat command or coordinator can start a conversation with it
- **AND** its `Ready` condition does not mention that it is unwired

### Requirement: Inline and referenced capabilities resolve to one shape

Everything downstream of wiring — conversation creation, the identity and
storage snapshot, tool composition, pod build — SHALL consume one resolved
capability, and SHALL NOT be able to tell whether it came from an AgentCapability or from
a Pipeline's inline fields.

#### Scenario: Two spellings, one conversation
- **WHEN** one Pipeline declares a capability inline and another references an AgentCapability with identical fields
- **THEN** conversations started by either carry identical snapshots and identical work units
