# pipeline-model

## Purpose

The Pipeline CRD: a credential-free wiring layer binding N signal sources and M channels to one profile, and the SOLE source of its conversations' capabilities (`toolsets` refs plus their composition mode, and mode-less `mcpConfigs` refs) — with pipeline-only routing resolution, and shareable sources whose signals fan out to every pipeline listing them.
## Requirements
### Requirement: Pipeline CRD declares the wiring between sources, channels, and a profile
The `Pipeline` CRD SHALL bind N `signalSourceRefs` and M `channelRefs` to one `profileRef`: signals from every referenced source SHALL become conversations bound to ALL referenced channels with the pipeline's profile, and conversations originated from any referenced channel SHALL be bound to all referenced channels. The Pipeline SHALL also be the SOLE source of its conversations' capabilities, via two optional stanzas of ordered refs: `spec.toolsets` (→ `MCPToolset` CRs, the allowlist) and `spec.mcpConfigs` (→ `MCPConfig` CRs, the MCP servers). `spec.toolsets` SHALL carry a `mode` (`merge` | `overwrite`, default `merge`) declaring how its tools compose with those the AGENT'S OWN DEFINITION declares — `merge` extends them, `overwrite` replaces them. `spec.mcpConfigs` SHALL carry no mode: an agent definition declares no MCP servers, so there is nothing there to compose against. Neither stanza has a default — a Pipeline that declares no bindings gives its conversations no wiring-level capabilities, and nothing supplies them elsewhere. A Pipeline SHALL be reachable two ways and no others: a signal posted to a source it LISTS, and a chat command NAMING it on a surface whose chat source is itself served. There SHALL be no HTTP addressing form that names a Pipeline — a caller selecting its own wiring is the shape this CRD exists to prevent, and the chat form is bounded by a person having to be on a wired surface to type it. A Pipeline with neither sources nor channels carries no special meaning; it is simply a route no signal feeds, still nameable by command. The Pipeline SHALL carry no credentials, no server or tool definitions, and no runtime selection (runtime stays `profile.runtimeRef → "default"`). A reconciler SHALL maintain a `Ready` condition (all references resolve, including toolset and mcpConfig refs) without creating any workload.

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
Routing SHALL resolve wiring exclusively through Ready Pipelines: a source's signals route via EVERY Ready Pipeline that lists it, and a source no Ready Pipeline lists drops its signals with a visible reason and a `Wired=False` condition. Resolution SHALL NOT fall back to pipeline creation order in any lane: the "oldest claimant" tiebreak is REMOVED, because a source no longer has a single claimant to pick.

This applies uniformly to every source kind, INCLUDING chat sources — a conversation originated by a general-surface chat message resolves through the Pipelines listing that chat `SignalSource`, exactly as an alert resolves through the Pipelines listing its alert source. Chat differs in ONE respect, and only for a message addressing no Pipeline: it is routed only when exactly one Ready Pipeline serves the source, because a person is owed a single answer and can name the agent they want.

A `Channel` SHALL NOT supply a default profile, and inbound resolution SHALL NOT fall back to pipeline creation order: the previous "oldest Ready Pipeline referencing this channel" tiebreak is REMOVED, because origination no longer happens on a channel. Channels remain shareable across Pipelines for delivery and mirroring, unaffected. Thread replies continue to resolve through the conversation's own thread binding, independent of any pipeline lookup.

A `/<name> <task>` chat command SHALL address a PIPELINE by name — the Pipeline originates the conversation, so it supplies the profile AND the capabilities; addressing a profile would name something with no wiring and therefore nothing to grant. The agent listing SHALL enumerate Ready Pipelines rather than profiles, so it advertises only names a user can actually address. No CR other than `Pipeline` carries standing wiring; `Conversation` fields are materialized per-conversation state, not wiring.

#### Scenario: Unclaimed source routes nothing
- **WHEN** a signal arrives for a source no Ready Pipeline references
- **THEN** no conversation is created, the response says so, and the source shows `Wired=False`

#### Scenario: Unclaimed chat source routes nothing
- **WHEN** a general-surface chat message arrives for a chat source no Ready Pipeline lists
- **THEN** no conversation is created, the source shows `Wired=False`, and the drop reason reaches the originating surface

#### Scenario: A command addresses a pipeline and gets its capabilities
- **WHEN** a user sends `/some-pipeline do it` on a channel
- **THEN** the conversation uses that Pipeline's profile and carries its toolsets and mcpConfigs, rather than being created with none

#### Scenario: Commands work through the chat source
- **WHEN** a user sends `/some-pipeline do it` on a channel whose chat source is listed by a Ready Pipeline
- **THEN** a conversation is created for that Pipeline through the signal path, bound to the originating channel

#### Scenario: The listing advertises only addressable names
- **WHEN** a user asks for the agent listing
- **THEN** it names Ready Pipelines, not AgentProfiles — a profile name cannot be addressed

#### Scenario: Bare message resolves only when one pipeline serves the surface
- **WHEN** a non-command message arrives on a channel's general surface and exactly one Ready Pipeline lists the chat source
- **THEN** the conversation uses that Pipeline's profile and is bound to all its channels

#### Scenario: No creation-order tiebreak remains anywhere
- **WHEN** two Ready Pipelines list the same source
- **THEN** neither is preferred by creation timestamp in any lane

#### Scenario: Shared channels need no tiebreak
- **WHEN** two Ready Pipelines both reference channel `web` for delivery
- **THEN** neither ordering nor creation timestamps affect inbound resolution, because neither claims origination on it

#### Scenario: Replies bypass pipeline resolution entirely
- **WHEN** a user replies inside an existing thread
- **THEN** the input is appended to that thread's conversation with no pipeline lookup

### Requirement: Sources are shareable and signals fan out
A `SignalSource` MAY be listed by any number of Ready Pipelines, of any signal kind. Doing so SHALL NOT produce a conflict condition and SHALL NOT affect any Pipeline's `Ready`. Listing a source means "I watch this" — it makes the source wired and, on a chat surface, makes the Pipeline addressable there — not "I own this".

A signal admitted on a source served by several Ready Pipelines SHALL produce one conversation PER Pipeline, each carrying that Pipeline's own profile, channel set and capabilities. Per-source ingest policy — fingerprint cooldown and signature grouping — SHALL be evaluated ONCE, before the fan-out, so a fingerprint is admitted once and then delivered to each server rather than being suppressed for all but the first.

Channels MAY likewise be referenced by multiple Pipelines; a conversation's binding set comes from the Pipeline that originated it.

#### Scenario: Two pipelines watching one alert source both investigate
- **WHEN** an alert arrives on a source two Ready Pipelines list
- **THEN** two conversations are created, one per Pipeline, each with its own profile and capabilities, and neither Pipeline reports a conflict

#### Scenario: Cooldown is not spent by the first server
- **WHEN** a fingerprint is admitted on a source two Ready Pipelines list
- **THEN** both Pipelines receive it, and the fingerprint is recorded as admitted once for the source

#### Scenario: Several pipelines serve one chat surface
- **WHEN** two Ready Pipelines both list the same chat SignalSource
- **THEN** neither reports a conflict, both stay `Ready=True`, and both appear in the surface's listing of available agents

#### Scenario: Channel shared by two pipelines stays valid
- **WHEN** two Ready Pipelines both reference channel `web`
- **THEN** neither reports a conflict, and each pipeline's sources produce conversations bound per their own pipeline

### Requirement: A conversation records the pipeline that originated it
A Conversation SHALL record the Pipeline that created it. That reference is PROVENANCE: it SHALL be written once at creation and SHALL NOT be read to resolve wiring — the profile, channels and capabilities a conversation runs with come from its own materialized fields, so editing or deleting the originating Pipeline SHALL NOT alter a running conversation.

The reference SHALL scope conversation reuse: a signal MAY only be appended to an existing conversation originated by the SAME Pipeline, so two Pipelines fanning out from one source never share a conversation. A conversation predating the reference SHALL be reusable only while exactly one Ready Pipeline serves the source, and SHALL NOT be backfilled by inference.

Attribution displays SHALL read the recorded origin rather than inferring it from matching bindings.

#### Scenario: Fanned-out conversations do not merge
- **WHEN** a second signal with the same signature arrives on a source two Ready Pipelines serve
- **THEN** each Pipeline's existing conversation receives it, and neither receives the other's

#### Scenario: Origin survives a rewiring
- **WHEN** the originating Pipeline's profile or capability bindings are edited after a conversation exists
- **THEN** the conversation keeps running with the bindings it materialized, and its recorded origin still names that Pipeline

#### Scenario: Attribution is read, not guessed
- **WHEN** two Pipelines with identical bindings each originate a conversation
- **THEN** each conversation is attributed to its own Pipeline rather than left blank as ambiguous

### Requirement: Chart-managed wiring is declared once, at the top
Wiring names a profile, signal sources and channels that routinely originate in
DIFFERENT components, and a subchart can see only itself. The parent scope is
therefore the DEFAULT place wiring is declared, and remains the only scope that
sees every component. A declaration SHALL require a profile and MAY name signal
sources, channels, toolsets and MCP configs; a declaration naming no profile
SHALL fail the render, because a Pipeline with no profile has no agent to run.

A subchart MAY render `Pipeline` objects only when ALL of the following hold:

- rendering is gated by an explicit wiring flag the operator can turn off,
  leaving every other component of that subchart intact;
- every reference to an object the subchart does not itself render is a
  values-supplied NAME, omitted from the rendered Pipeline when unset, so the
  subchart never names an object no component created;
- each rendered Pipeline renders only when its own profile renders;
- the wiring flag DEFAULTS OFF, so enabling a subchart for its components never
  silently adds a route to an install that declares its own.

The default-off rule has one permitted exception: a values path whose declared
purpose is a turnkey install — a demo or quickstart mode — MAY force a
subchart's wiring flag on, and SHALL force on only the LEAST-PRIVILEGED route
that subchart offers. A subchart that substantially owns its lane MAY document
an exception to the default in its own specification; the general rule does not
carry it.

A subchart that cannot meet these conditions SHALL render no wiring. Shipping
wiring is a choice a bundle makes for a lane it substantially owns — it does not
become the norm, and a bundle whose sources and channels all come from elsewhere
has nothing to gain from it.

#### Scenario: One route spanning several components
- **WHEN** an install combines a cluster-events source from one component with a
  chat surface from another, answered by one agent
- **THEN** a single Pipeline declared at the parent scope claims both sources
  and delivers to the channel

#### Scenario: A component's source is inert until claimed
- **WHEN** a component renders a signal source and neither the install nor that
  component declares a Pipeline claiming it
- **THEN** the source reports `Wired=False` and drops its signals, exactly as an
  unclaimed source always does

#### Scenario: A profile-less declaration is refused
- **WHEN** a wiring entry omits its profile
- **THEN** the render fails naming the entry

#### Scenario: A bundle's wiring can be declined
- **WHEN** a subchart that ships wiring has its wiring flag turned off
- **THEN** it renders no Pipeline, every other component of that subchart still
  renders, and the install's own declarations are unaffected

#### Scenario: Enabling a subchart adds no route by itself
- **WHEN** an operator enables a subchart that ships wiring, without setting its
  wiring flag or a turnkey mode
- **THEN** no Pipeline is rendered by that subchart, and the install's own
  `pipelines:` declarations remain the only routes

#### Scenario: A turnkey mode forces the safe route
- **WHEN** a demo or quickstart values path turns a subchart's wiring on
- **THEN** the route it renders is the subchart's least-privileged one, and a
  more privileged route renders only where the install asked for it explicitly

#### Scenario: Two claimants are permitted, not refused
- **WHEN** a subchart's wiring and an install-declared Pipeline both claim one
  source
- **THEN** both Pipelines render and the source fans out to both, because
  sources are shareable and no conflict guard exists to reinstate

### Requirement: Pipelines are named for their purpose, not their transport
A `Channel` is shareable across Pipelines by design — one chat surface carries
many jobs, so that operators need not run a bot and a group per route. A
`SignalSource` is shareable too. Naming SHALL follow from what a Pipeline IS: it
is named for the JOB it does, never for the channel it answers on, because the
channel will carry other jobs — and never for the source it watches, because
another Pipeline may watch the same one for a different purpose.

#### Scenario: Several pipelines share one chat surface
- **WHEN** two Pipelines with different purposes both deliver to one Channel
- **THEN** both are valid, and each carries its own profile and capabilities

#### Scenario: Two purposes may watch one source
- **WHEN** two Pipelines with different purposes both list one SignalSource
- **THEN** both are valid and neither is preferred, so each name must say which
  job it does rather than which source it reads

