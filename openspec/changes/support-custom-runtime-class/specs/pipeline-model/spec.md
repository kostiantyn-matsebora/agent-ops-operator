## MODIFIED Requirements

### Requirement: Pipeline CRD declares the wiring between sources, channels, and a profile

The `Pipeline` CRD SHALL bind N `signalSourceRefs` and M `channelRefs` to one
`profileRef`. Signals from every referenced source SHALL become
conversations bound to ALL referenced channels with the pipeline's profile.

A channel ORIGINATES nothing, so there is no channel-originated case. A
conversation started by a chat command on a surface binds to the addressed
Pipeline's channels PLUS the surface it was typed on, which is where the
person is looking.

The Pipeline, inline or through its `capabilityRef`, SHALL be the SOLE
source of its conversations' capabilities, via two optional stanzas of ordered refs: `spec.toolsets` (→ `MCPToolset`
CRs, the allowlist) and `spec.mcpConfigs` (→ `MCPConfig` CRs, the MCP
servers).

`spec.toolsets` SHALL carry a `mode` (`merge` | `overwrite`, default
`merge`) declaring how its tools compose with those the AGENT'S OWN
DEFINITION declares. `spec.mcpConfigs` SHALL carry no mode. Neither stanza
has a default. A Pipeline that declares no bindings gives its conversations
no wiring-level capabilities.

THE PIPELINE SHALL ALSO SELECT WHAT EXECUTES ITS CONVERSATIONS, UNDER WHOSE
IDENTITY AND UNDER WHICH KERNEL ISOLATION, via three further optional
fields:

- `spec.runtimeRef` — the `AgentRuntime`. Absent, the runtime named
  `default`.
- `spec.serviceAccountName` — the identity that runtime executes under,
  OVERRIDING the runtime's own. **Absent, the runtime's own account, and
  absent there too, the MINIMUM-PRIVILEGE floor account, which holds no RBAC
  at all.**
- `spec.runtimeClassName` — the pod's `RuntimeClassName`, OVERRIDING the
  runtime's own. Absent, the runtime's value applies, and absent there too,
  the cluster's default kernel.

All three SHALL be optional. `runtimeRef` unset SHALL behave exactly as an
install that could not set it.

**`serviceAccountName` UNSET SHALL MEAN NO CLUSTER POWER, and that is a
DELIBERATE BREAK.** A route inherits nothing by staying silent.

What an agent may do in the cluster is stated on the route or it does not
exist, the same rule the toolsets already follow, applied to the
credential half.

**`runtimeClassName` carries no such break.** Unset SHALL mean exactly what
unset means today: no isolation change, the runtime's or the cluster's own
kernel.

CAPABILITIES AND EXECUTION IDENTITY ARE THE SAME DECISION and SHALL live on
the same object. One states which tools may be called, the other with
whose credentials. Split across two objects, no single object states an
agent's power and no single reviewer can approve it.

The Pipeline SHALL carry no credentials and no server or tool definitions.
It SHALL NOT create a ServiceAccount. Naming one is a reference, and who
may create one and what it is bound to remains an external grant.

The same holds for `runtimeClassName`. Naming one is a reference, and the
chart never checks a `RuntimeClass` with that name exists.

`Ready` SHALL NOT validate `spec.serviceAccountName` or
`spec.runtimeClassName`. Checking either exists needs RBAC the manager
holds none of, and granting a permission to produce a WARNING is a worse
trade than the failure it would pre-empt.

That failure is already loud, local and named. The pod is refused at
admission, naming the account or the class.

`spec.runtimeRef` IS validated, because the manager already reads
`AgentRuntime`, and a dangling runtime ref is the same class of dangling
reference every other stanza reports.

A Pipeline SHALL be reachable two ways and no others. A signal posted to a
source it LISTS, and a chat command NAMING it on a surface whose chat
source is itself served.

There SHALL be no HTTP addressing form that names a Pipeline. A
reconciler SHALL maintain a `Ready` condition, all references resolved
including toolset and mcpConfig refs, without creating any workload.

#### Scenario: Signals fan out to every pipeline channel
- **WHEN** a Pipeline binds source `alertmanager` to channels `home-ops` and `web` and an alert fires
- **THEN** the resulting conversation carries channel bindings for both `home-ops` and `web` and uses the pipeline's profile

#### Scenario: Chat-originated conversations are pipeline-bound
- **WHEN** a user starts a conversation with a `/<pipeline> <task>` command on a chat surface
- **THEN** the conversation is bound to all the addressed Pipeline's channels, not just the originating one, and the originating channel is folded in whether or not that Pipeline lists it

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

#### Scenario: One runtime image serves two isolation levels

- **WHEN** two Pipelines name the same `runtimeRef` and different
  `runtimeClassName` values
- **THEN** their conversations run the same image under different kernels,
  and no second `AgentRuntime` exists solely to carry the second isolation level

#### Scenario: A Pipeline naming no account has no cluster power

- **WHEN** a Pipeline names no runtime and no service account
- **THEN** its conversations resolve the runtime named `default` and run under
  the account that runtime names, which ships as the floor — denied every verb,
  with no setting able to widen it

#### Scenario: The runtime's own account is still a rung

- **WHEN** a Pipeline names no account and its `AgentRuntime` names one
- **THEN** the runtime's is used, because a runtime declaring an identity is an
  explicit statement and the floor is only what remains when nobody stated one

#### Scenario: The Pipeline's service account wins over the runtime's

- **WHEN** a Pipeline names a service account and the runtime it selects also
  names one
- **THEN** the pod runs under the PIPELINE's, because expressing a trust level
  per route is what the field is for

#### Scenario: The Pipeline's runtime class wins over the runtime's

- **WHEN** a Pipeline names a `runtimeClassName` and the runtime it selects
  also names one
- **THEN** the pod runs under the PIPELINE's, because expressing an
  isolation level per route is what the field is for

#### Scenario: Naming an account does not create one

- **WHEN** a Pipeline names a service account that does not exist
- **THEN** no reconciler creates it, the render does not fail and `Ready` stays
  true — the pod fails at admission, naming the account

#### Scenario: Naming a runtime class does not create one

- **WHEN** a Pipeline names a `runtimeClassName` that does not exist in the
  cluster
- **THEN** no reconciler creates it, the render does not fail and `Ready`
  stays true — the pod fails at admission, naming the class

#### Scenario: A dangling runtime ref does surface on Ready
- **WHEN** a Pipeline's `runtimeRef` names an `AgentRuntime` that does not exist
- **THEN** the Pipeline reports `Ready=False` naming it, because the manager
  already reads that kind and the ref is a reference like any other

### Requirement: A Pipeline names its agent inline or by reference

A Pipeline SHALL declare its agent EITHER through the seven inline
capability fields OR through `spec.capabilityRef` naming an
`AgentCapability`.

The API server SHALL reject a manifest carrying both.

The inline form is unchanged from before this requirement existed.

A referenced AgentCapability's CONTENT is re-read as an inline field's
would be. The resolved identity, runtime class and storage are
snapshotted onto the conversation exactly as before.

#### Scenario: Both forms refused
- **WHEN** a Pipeline carries `capabilityRef` and `profileRef`
- **THEN** the API server rejects it naming the exclusivity

#### Scenario: A dangling capabilityRef
- **WHEN** a Pipeline's `capabilityRef` names no AgentCapability
- **THEN** its `Ready` is False naming the ref, and it claims nothing

#### Scenario: Existing Pipelines are untouched
- **WHEN** a Pipeline written before `capabilityRef` existed is applied
- **THEN** it validates and behaves exactly as it did
