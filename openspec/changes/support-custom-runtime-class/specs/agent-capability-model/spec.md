## MODIFIED Requirements

### Requirement: An AgentCapability declares a capability and no wiring

The `AgentCapability` CRD SHALL carry exactly the seven capability fields a
Pipeline carries inline — `profileRef`, `runtimeRef`, `serviceAccountName`,
`runtimeClassName`, `toolsets`, `mcpConfigs`, `persistence` — with the same
semantics, defaults and validation each has on a Pipeline.

It SHALL carry no `signalSourceRefs`, no `channelRefs` and no other field
that reaches a signal or a surface.

`runtimeClassName` SHALL override the `AgentRuntime`'s own
`runtimeClassName` when set, the same way `serviceAccountName` already
overrides the runtime's own identity. Unset SHALL mean the runtime's value
applies unchanged.

#### Scenario: The capability fields validate as on a Pipeline
- **WHEN** an AgentCapability names a `runtimeRef` no `AgentRuntime` backs
- **THEN** its `Ready` condition is False naming the ref, exactly as a Pipeline's would be

#### Scenario: Wiring fields are pruned
- **WHEN** an AgentCapability manifest carries `signalSourceRefs` or `channelRefs`
- **THEN** the API server silently PRUNES the unknown field rather than rejecting it — structural CRDs with no `additionalProperties: false` prune, they do not error
- **AND** the stored object therefore carries no wiring whatever the manifest wrote

#### Scenario: A capability's runtime class overrides its runtime's
- **WHEN** an AgentCapability sets `runtimeClassName` and its `runtimeRef` names an `AgentRuntime` with a different `runtimeClassName`
- **THEN** conversations through this capability resolve to the capability's value, not the runtime's

### Requirement: Inline and referenced capabilities resolve to one shape

Everything downstream of wiring — conversation creation, the identity,
runtime-class and storage snapshot, tool composition, pod build — SHALL
consume one resolved capability.

It SHALL NOT be able to tell whether that capability came from an
AgentCapability or from a Pipeline's inline fields.

The resolved `runtimeClassName` SHALL be written onto
`Conversation.spec.runtimeClassName` at creation, exactly once, on the same
terms as `serviceAccountName`. A later change to the capability or runtime
SHALL NOT alter an already-created conversation's pods.

#### Scenario: Two spellings, one conversation
- **WHEN** one Pipeline declares a capability inline and another references an AgentCapability with identical fields
- **THEN** conversations started by either carry identical snapshots and identical work units

#### Scenario: A re-wire does not move an inflight conversation's isolation
- **WHEN** a Pipeline's `runtimeClassName` changes after a conversation it started already exists
- **THEN** that conversation's `Conversation.spec.runtimeClassName` is unchanged, and any pod rebuilt for it keeps using the value frozen at creation
