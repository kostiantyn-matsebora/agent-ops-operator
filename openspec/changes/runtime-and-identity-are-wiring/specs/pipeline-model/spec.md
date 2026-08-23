## MODIFIED Requirements

### Requirement: Pipeline CRD declares the wiring between sources, channels, and a profile

The `Pipeline` CRD SHALL bind N `signalSourceRefs` and M `channelRefs` to one
`profileRef`: signals from every referenced source SHALL become conversations
bound to ALL referenced channels with the pipeline's profile, and conversations
originated from any referenced channel SHALL be bound to all referenced
channels.

The Pipeline SHALL be the SOLE source of its conversations' capabilities, via
two optional stanzas of ordered refs: `spec.toolsets` (→ `MCPToolset` CRs, the
allowlist) and `spec.mcpConfigs` (→ `MCPConfig` CRs, the MCP servers).
`spec.toolsets` SHALL carry a `mode` (`merge` | `overwrite`, default `merge`)
declaring how its tools compose with those the AGENT'S OWN DEFINITION declares.
`spec.mcpConfigs` SHALL carry no mode. Neither stanza has a default — a Pipeline
that declares no bindings gives its conversations no wiring-level capabilities.

THE PIPELINE SHALL ALSO SELECT WHAT EXECUTES ITS CONVERSATIONS AND UNDER WHOSE
IDENTITY, via two further optional fields:

- `spec.runtimeRef` — the `AgentRuntime`. Absent, the runtime named `default`.
- `spec.serviceAccountName` — the identity that runtime executes under,
  OVERRIDING the runtime's own. Absent, the runtime's `serviceAccountName`.

Both SHALL be optional, and an install setting neither SHALL behave exactly as
one that could not set them.

CAPABILITIES AND EXECUTION IDENTITY ARE THE SAME DECISION and SHALL live on the
same object. One states which tools may be called, the other with whose
credentials — and split across two objects, no single object states an agent's
power and no single reviewer can approve it.

The Pipeline SHALL carry no credentials and no server or tool definitions. It
SHALL NOT create a ServiceAccount: naming one is a reference, and who may create
one and what it is bound to remains an external grant.

A Pipeline SHALL be reachable two ways and no others: a signal posted to a
source it LISTS, and a chat command NAMING it on a surface whose chat source is
itself served. There SHALL be no HTTP addressing form that names a Pipeline. A
reconciler SHALL maintain a `Ready` condition (all references resolve, including
toolset and mcpConfig refs) without creating any workload.

#### Scenario: Signals fan out to every pipeline channel
- **WHEN** a Pipeline binds source `alertmanager` to channels `home-ops` and `web` and an alert fires
- **THEN** the resulting conversation carries channel bindings for both `home-ops` and `web` and uses the pipeline's profile

#### Scenario: Chat-originated conversations are pipeline-bound
- **WHEN** a user starts a conversation on a channel referenced by a Pipeline
- **THEN** the conversation is bound to all the Pipeline's channels, not just the originating one

#### Scenario: Dangling references surface on Ready
- **WHEN** a Pipeline references a SignalSource that does not exist
- **THEN** the Pipeline reports `Ready=False` naming the missing reference

#### Scenario: Capabilities bind per route
- **WHEN** two Ready Pipelines route to the same profile with different `toolsets`
- **THEN** conversations from each carry exactly that Pipeline's tools, and the profile declares none

#### Scenario: A mode declares how the route composes with the agent
- **WHEN** a Pipeline binds `toolsets` in `overwrite` mode to a profile whose agent definition declares its own tools
- **THEN** conversations from that route use the Pipeline's tools alone, while a `merge`-mode Pipeline to the same profile extends the agent's

#### Scenario: An absent mode is merge
- **WHEN** a Pipeline binds `toolsets` without naming a mode
- **THEN** it composes as `merge`, so the route adds to what the agent declares rather than replacing it

#### Scenario: A Pipeline is reached through the sources it claims
- **WHEN** a `kind: task` signal is posted to a source a Ready Pipeline claims
- **THEN** the created conversation uses that Pipeline's profile, channel set, and capabilities

#### Scenario: A sourceless, channelless Pipeline is unremarkable
- **WHEN** a Pipeline names only a `profileRef` and capability stanzas
- **THEN** it is a route no signal feeds — it claims no source, so nothing resolves to it — while a chat command naming it still opens a conversation, and it carries no per-profile default meaning

#### Scenario: Dangling tooling ref surfaces on Ready
- **WHEN** a Pipeline's `toolsets.refs` or `mcpConfigs.refs` names a CR that does not exist
- **THEN** the Pipeline reports `Ready=False` naming the missing reference

#### Scenario: One runtime image serves two trust levels

- **WHEN** two Pipelines name the same `runtimeRef` and different
  `serviceAccountName` values
- **THEN** their conversations run the same image with different cluster power,
  and no second `AgentRuntime` exists solely to carry the second identity

#### Scenario: Setting neither field changes nothing

- **WHEN** a Pipeline names no runtime and no service account
- **THEN** its conversations resolve the runtime named `default` and run under
  that runtime's service account, exactly as before these fields existed

#### Scenario: The Pipeline's service account wins over the runtime's

- **WHEN** a Pipeline names a service account and the runtime it selects also
  names one
- **THEN** the pod runs under the PIPELINE's, because expressing a trust level
  per route is what the field is for

#### Scenario: Naming an account does not create one

- **WHEN** a Pipeline names a service account that does not exist
- **THEN** no reconciler creates it and the render does not fail — the pod fails
  at admission, naming the account

### Requirement: Runtime selection is wiring, never identity

An `AgentProfile` SHALL NOT select the runtime. `spec.runtimeRef` on a profile
is DEPRECATED and read for one release only, so a profile applied before the
upgrade keeps dispatching to the runtime it named.

Resolution SHALL run in a stated order, and the CONVERSATION's snapshot SHALL
come first:

1. the conversation's materialized runtime and service account
2. the originating Pipeline's
3. the profile's `runtimeRef` — deprecated, runtime only
4. the `AgentRuntime` named `default`, then the manager's bootstrap
   configuration

#### Scenario: A profile keeps working for one release

- **WHEN** a profile names a `runtimeRef` and the Pipeline routing it names none
- **THEN** the profile's runtime is used, and the field is removed in the
  following release

#### Scenario: The Pipeline wins over the deprecated field

- **WHEN** both a Pipeline and its profile name a runtime
- **THEN** the Pipeline's is used, so adopting the new model needs no profile
  edit
