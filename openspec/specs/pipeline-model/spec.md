# pipeline-model

## Purpose

The Pipeline CRD: a credential-free wiring layer binding N signal sources and M channels to one profile, and the SOLE source of its conversations' capabilities (`toolsets` refs plus their composition mode, and mode-less `mcpConfigs` refs) — with pipeline-only routing resolution, and at-most-one-pipeline-per-source claiming.
## Requirements
### Requirement: Pipeline CRD declares the wiring between sources, channels, and a profile
The `Pipeline` CRD SHALL bind N `signalSourceRefs` and M `channelRefs` to one `profileRef`: signals from every referenced source SHALL become conversations bound to ALL referenced channels with the pipeline's profile, and conversations originated from any referenced channel SHALL be bound to all referenced channels. The Pipeline SHALL also be the SOLE source of its conversations' capabilities, via two optional stanzas of ordered refs: `spec.toolsets` (→ `MCPToolset` CRs, the allowlist) and `spec.mcpConfigs` (→ `MCPConfig` CRs, the MCP servers). `spec.toolsets` SHALL carry a `mode` (`merge` | `overwrite`, default `merge`) declaring how its tools compose with those the AGENT'S OWN DEFINITION declares — `merge` extends them, `overwrite` replaces them. `spec.mcpConfigs` SHALL carry no mode: an agent definition declares no MCP servers, so there is nothing there to compose against. Neither stanza has a default — a Pipeline that declares no bindings gives its conversations no wiring-level capabilities, and nothing supplies them elsewhere. A Pipeline SHALL be reachable two ways and no others: a signal posted to a source it CLAIMS, and a chat command NAMING it on a surface whose chat source is itself claimed. There SHALL be no HTTP addressing form that names a Pipeline — a caller selecting its own wiring is the shape this CRD exists to prevent, and the chat form is bounded by a person having to be on a wired surface to type it. A Pipeline with neither sources nor channels carries no special meaning; it is simply a route no signal feeds, still nameable by command. The Pipeline SHALL carry no credentials, no server or tool definitions, and no runtime selection (runtime stays `profile.runtimeRef → "default"`). A reconciler SHALL maintain a `Ready` condition (all references resolve, including toolset and mcpConfig refs) without creating any workload.

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

### Requirement: Pipeline-only resolution
Routing SHALL resolve wiring exclusively through Ready Pipelines: a source's signals route via the pipeline that claims it (unclaimed sources drop signals with a visible reason and a `Wired=False` condition). This applies uniformly to every source kind, INCLUDING chat sources — a conversation originated by a general-surface chat message resolves through the Pipeline claiming that chat `SignalSource`, exactly as an alert resolves through the Pipeline claiming its alert source.

A `Channel` SHALL NOT supply a default profile, and inbound resolution SHALL NOT fall back to pipeline creation order: the previous "oldest Ready Pipeline referencing this channel" tiebreak is REMOVED, because origination no longer happens on a channel. Channels remain shareable across Pipelines for delivery and mirroring, unaffected. Thread replies continue to resolve through the conversation's own thread binding, independent of any pipeline lookup.

A `/<name> <task>` chat command SHALL address a PIPELINE by name — the Pipeline originates the conversation, so it supplies the profile AND the capabilities; addressing a profile would name something with no wiring and therefore nothing to grant. The agent listing SHALL enumerate Ready Pipelines rather than profiles, so it advertises only names a user can actually address. No CR other than `Pipeline` carries standing wiring; `Conversation` fields are materialized per-conversation state, not wiring.

#### Scenario: Unclaimed source routes nothing
- **WHEN** a signal arrives for a source no Ready Pipeline references
- **THEN** no conversation is created, the response says so, and the source shows `Wired=False`

#### Scenario: Unclaimed chat source routes nothing
- **WHEN** a general-surface chat message arrives for a chat source no Ready Pipeline claims
- **THEN** no conversation is created, the source shows `Wired=False`, and the drop reason reaches the originating surface

#### Scenario: A command addresses a pipeline and gets its capabilities
- **WHEN** a user sends `/some-pipeline do it` on a channel
- **THEN** the conversation uses that Pipeline's profile and carries its toolsets and mcpConfigs, rather than being created with none

#### Scenario: Commands work through the chat source
- **WHEN** a user sends `/some-pipeline do it` on a channel whose chat source is claimed
- **THEN** a conversation is created for that Pipeline through the signal path, bound to the originating channel

#### Scenario: The listing advertises only addressable names
- **WHEN** a user asks for the agent listing
- **THEN** it names Ready Pipelines, not AgentProfiles — a profile name cannot be addressed

#### Scenario: Bare message resolves through the claiming pipeline
- **WHEN** a non-command message arrives on a channel's general surface
- **THEN** the conversation uses the profile of the Pipeline claiming the chat source and is bound to all that Pipeline's channels

#### Scenario: Shared channels need no tiebreak
- **WHEN** two Ready Pipelines both reference channel `web` for delivery
- **THEN** neither ordering nor creation timestamps affect inbound resolution, because neither claims origination on it

#### Scenario: Replies bypass pipeline resolution entirely
- **WHEN** a user replies inside an existing thread
- **THEN** the input is appended to that thread's conversation with no pipeline lookup

### Requirement: One pipeline per source
A SignalSource SHALL be claimed by at most one Pipeline: when a second Pipeline references an already-claimed source, the newer Pipeline SHALL report `SourceConflict=True` naming the contested source and the older claim SHALL keep routing. Channels MAY be referenced by multiple Pipelines; a conversation's binding set comes from the pipeline that originates it, and inbound on a multi-pipeline channel originates via the oldest Ready claimant (deterministic).

#### Scenario: Second claim on a source is refused
- **WHEN** two Pipelines reference the same SignalSource
- **THEN** the newer reports `SourceConflict=True` and the source's signals keep routing per the older Pipeline

#### Scenario: Channel shared by two pipelines stays valid
- **WHEN** two Ready Pipelines both reference channel `web`
- **THEN** neither reports a conflict, and each pipeline's sources produce conversations bound per their own pipeline

### Requirement: Chart-managed wiring is declared once, at the top
A chart that ships components as subcharts SHALL NOT render `Pipeline` objects
from any subchart. Wiring names a profile, signal sources and channels that
routinely originate in DIFFERENT components, and a subchart can see only itself
— so a component that shipped wiring could only wire its own lane, producing one
Pipeline per lane where the install wanted one route.

Wiring SHALL instead be declared at the parent scope, which is the only one that
sees every component. A declaration SHALL require a profile and MAY name signal
sources, channels, toolsets and MCP configs; a declaration naming no profile
SHALL fail the render, because a Pipeline with no profile has no agent to run.

#### Scenario: One route spanning several components
- **WHEN** an install combines a cluster-events source from one component with a
  chat surface from another, answered by one agent
- **THEN** a single Pipeline declared at the parent scope claims both sources
  and delivers to the channel, and no component renders wiring of its own

#### Scenario: A component's source is inert until claimed
- **WHEN** a component renders a signal source and the install declares no
  Pipeline claiming it
- **THEN** the source reports `Wired=False` and drops its signals, exactly as an
  unclaimed source always does

#### Scenario: A profile-less declaration is refused
- **WHEN** a wiring entry omits its profile
- **THEN** the render fails naming the entry

### Requirement: Pipelines are named for their purpose, not their transport
A `Channel` is shareable across Pipelines by design — one chat surface carries
many jobs, so that operators need not run a bot and a group per route. A
`SignalSource` is claimed by exactly one Pipeline. Naming SHALL follow that
asymmetry: a Pipeline is named for the JOB it does, never for the channel it
answers on, because the channel will carry other jobs.

#### Scenario: Several pipelines share one chat surface
- **WHEN** two Pipelines with different purposes both deliver to one Channel
- **THEN** both are valid, and each carries its own profile and capabilities

#### Scenario: A chat source has exactly one answerer
- **WHEN** two Pipelines claim the same chat SignalSource
- **THEN** the older claimant wins and the other reports a source conflict, so
  "who answers this surface" always has one answer

