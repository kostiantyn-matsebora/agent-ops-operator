# mcp-toolset-model

## Purpose

Wiring-level tool access: the `MCPToolset` CRD (a reusable, server-definition-free list of tool patterns), the two Pipeline bindings (`toolsets`, carrying the mode that composes with the agent definition, and `mcpConfigs`, mode-less, for MCP servers), their materialization onto the Conversation with lazy content resolution, and how compilation and dispatch apply the effective tooling.

## Requirements

### Requirement: MCPToolset CRD declares a reusable tool list
An `MCPToolset` CRD SHALL declare a named LIST of tool patterns (`spec.tools`): MCP namespaces like `mcp__victorialogs__*` and/or built-in tool names like `Bash`. It SHALL carry NO MCP server definitions — server definitions belong exclusively to `MCPConfig` CRs. The patterns are opaque to the manager (passed through to the runtime exactly like `allowedTools` today), so the CRD needs no resolution status.

#### Scenario: A bundle-shipped toolset is a single CR
- **WHEN** an `MCPToolset` with `tools: [mcp__victorialogs__*, mcp__victoriametrics__*]` is applied
- **THEN** it is referencable from any number of Pipelines' `toolsets` bindings

#### Scenario: Toolsets may grant built-in tools
- **WHEN** an `MCPToolset` lists `tools: [Read, Grep, mcp__victorialogs__*]`
- **THEN** all three entries participate in allowlist resolution — the toolset is not restricted to MCP namespaces

### Requirement: Bindings materialize on the Conversation with lazy content resolution

Conversations SHALL snapshot the ORIGINATING PIPELINE's wiring into the
Conversation spec at creation — materialized per-conversation state, following
the profileRef/channelRefs pattern.

That snapshot SHALL cover both tooling bindings (`toolsets`, `mcpConfigs`) AND
THE RESOLVED RUNTIME AND SERVICE ACCOUNT. That Pipeline is the only source: no
profile default, no baseline, no inheritance.

THE RESOLVED RUNTIME NAME IS SNAPSHOTTED, NOT THE PIPELINE'S RAW FIELD. A
conversation created while its Pipeline named no runtime SHALL keep the one it
actually ran on, rather than picking up a later edit to that Pipeline — or to
the deprecated profile ref below it.

THE SERVICE ACCOUNT IS SNAPSHOTTED ONLY WHERE THE PIPELINE NAMED ONE, and the
asymmetry is the ref/content rule, not an inconsistency. A Pipeline's account is
WIRING and is frozen. An `AgentRuntime`'s own account is that runtime's CONTENT,
so a conversation whose Pipeline named none SHALL leave the field empty and
resolve the runtime's at every pod build — otherwise correcting a mistyped
account would strand every conversation already created on a name the operator
has already fixed. Leaving it empty is SAFE against the escalation this rule
guards, because resolution never reads a Pipeline: the empty case falls to the
runtime, never to the edited wiring.

CONTENT SHALL be re-resolved at each use — toolset and MCPConfig contents at MCP
compilation and work-unit dispatch, and the `AgentRuntime`'s image, idle TTL,
volume AND its own service account at every pod build — so content edits reach
existing conversations while re-wiring affects only new ones.

**THE IDENTITY SNAPSHOT IS THE SHARPEST CASE OF THIS RULE.** Without it, editing
a Pipeline changes what service account an INFLIGHT conversation's next pod runs
as. That is not a re-wiring inconvenience, it is a privilege change applied to
work already in progress.

This SHALL hold identically for every origination path — a signal of any kind
through a claimed source, and a chat command naming a Pipeline — because all of
them read the same Pipeline for the same fields.

#### Scenario: Pipeline bindings follow the signal
- **WHEN** a signal routes through a pipeline with `toolsets: {refs: [vm-observability]}` and `mcpConfigs: {refs: [vm-logs]}`
- **THEN** the created conversation's spec records both ref sets

#### Scenario: Content edits heal running conversations
- **WHEN** a bound MCPConfig's server URL is corrected while conversations referencing it exist
- **THEN** subsequent MCP compilation for those conversations uses the corrected URL

#### Scenario: A posted task carries the claiming Pipeline's wiring
- **WHEN** a `kind: task` signal is posted to a source claimed by a Pipeline that binds toolsets and mcpConfigs
- **THEN** the created conversation carries that Pipeline's profile, channel set, and both tooling bindings

#### Scenario: A Pipeline declaring nothing yields no bindings
- **WHEN** a conversation originates from a Pipeline with neither binding declared
- **THEN** it carries no bindings and dispatches with an empty allowlist, with nothing supplying a default

#### Scenario: Re-wiring never moves a running conversation's identity

- **WHEN** a Pipeline's `serviceAccountName` is changed while one of its
  conversations is inflight
- **THEN** that conversation's next pod runs under the account it was created
  with, and only conversations created afterwards use the new one

#### Scenario: Fixing a runtime still heals running conversations

- **WHEN** the `AgentRuntime` a conversation snapshotted has its image corrected
- **THEN** the next pod uses the corrected image, because the REF is frozen and
  the CONTENT is not

#### Scenario: Fixing a runtime's own account heals them too

- **WHEN** a conversation whose Pipeline named NO service account runs on an
  `AgentRuntime` whose `serviceAccountName` is then corrected
- **THEN** the next pod uses the corrected account, because the Pipeline named
  none to freeze and the runtime's is content

#### Scenario: A conversation predating the snapshot resolves as before

- **WHEN** a conversation created before these fields existed dispatches
- **THEN** resolution falls through to the deprecated profile ref and the
  `default` runtime, and nothing backfills the snapshot

### Requirement: Compilation and dispatch apply the effective tooling
Every conversation with an `mcpConfigs` binding SHALL compile into a conversation-owned ConfigMap `agentops-mcp-conv-<conversation>` (ownerRef → Conversation, GC'd with it); the profile-keyed `agentops-mcp-<profile>` ConfigMap SHALL NOT be created, since profiles declare no MCP. Secret-backed header values SHALL still compile to `valueFrom` env placeholders — the manager reads no Secrets. Work-unit dispatch SHALL compute the WIRING'S allowlist contribution from the bound toolsets at dispatch time, so toolset edits apply from the next work unit with no pod restart; the unit SHALL also carry the binding's mode and the agent whose definition the runtime composes it with. A binding ref that fails to resolve SHALL fail visibly (conversation condition), never degrade silently to reduced tooling.

#### Scenario: Every MCP-bound conversation owns its ConfigMap
- **WHEN** two pipelines with different `mcpConfigs` bindings route to one profile and each creates a conversation
- **THEN** each mounts its own `agentops-mcp-conv-<name>` and no profile-keyed ConfigMap exists

#### Scenario: Toolset content edits reach running conversations
- **WHEN** a bound MCPToolset gains a tool while conversations referencing it exist
- **THEN** their next work unit carries it, with no runtime pod restart

#### Scenario: Missing binding ref fails the work visibly
- **WHEN** a conversation's bound MCPToolset or MCPConfig is deleted and a new work unit is dispatched
- **THEN** the failure surfaces on the conversation rather than proceeding with silently reduced tooling

### Requirement: Wiring-level tool access is the bound refs' contribution
A conversation's wiring-level tool access SHALL come from its materialized bindings — the profile contributes nothing, having no capability fields. **Allowlist contribution**: the concatenation of the `toolsets` refs' `tools` in ref order, deduped, first occurrence keeping its position. This is the WIRING'S CONTRIBUTION, not the final allowlist: the `toolsets` binding also carries a `mode` (`merge` | `overwrite`, default `merge`) saying how it composes with what the agent's own definition declares, and that composition SHALL happen in the runtime, which is the only component holding the repository. A binding stored without a mode SHALL compose as `merge`. **MCP servers**: the `mcpConfigs` refs' servers merged in ref order, per server key, later ref winning on collision; that binding carries NO mode, because an agent definition declares no servers. A conversation with no bindings contributes no tools and no MCP servers. A bound `MCPConfig` in raw form (`configMapRef`/`secretRef`) SHALL be exclusive — combining it with any other config SHALL surface a condition naming the conflict, since a hand-written `mcp.json` cannot be composed.

#### Scenario: Bound toolsets are the wiring's contribution
- **WHEN** a pipeline binds two toolsets to a conversation
- **THEN** the work unit carries exactly those toolsets' tools, deduped in ref order, together with the mode that composes them with the agent's own

#### Scenario: The mode travels with the work unit
- **WHEN** a pipeline binds `toolsets` in `overwrite` mode
- **THEN** the work unit records that mode, so the runtime replaces the agent's declared tools rather than extending them

#### Scenario: An absent mode composes as merge
- **WHEN** a conversation's `toolsets` binding carries no mode
- **THEN** its work unit dispatches as `merge`, never as `overwrite` — an unset field must not strip what the agent declared

#### Scenario: Bound configs are the whole MCP
- **WHEN** a pipeline binds two MCPConfigs whose servers share a key
- **THEN** the compiled `mcp.json` contains the union, the later ref winning the shared key

#### Scenario: No binding means no contribution
- **WHEN** a conversation carries no `toolsets` and no `mcpConfigs` binding
- **THEN** the wiring contributes no tools and the runtime gets an empty `mcp.json`

#### Scenario: Raw configs refuse to combine
- **WHEN** a binding names a raw-form MCPConfig alongside another config
- **THEN** the conversation surfaces a condition naming the conflict instead of mounting a partial result

### Requirement: The effective allowlist is enforceable without the agent's cooperation
The tool access a conversation's wiring grants SHALL be enforceable at a point the agent does not control. Where the runtime composes and passes `--allowedTools` to a CLI running beside the agent, that composition SHALL be understood as CONFIGURATION of a cooperating agent, not as a boundary: an agent able to execute commands can reach a bound MCP server directly and call anything that server registers. An installation SHALL therefore be able to make the same access decision binding on a non-cooperating agent, and the two SHALL derive from the SAME wiring — the bound toolsets and their mode — so that what is configured and what is enforced can never disagree.

Documentation of capability resolution SHALL NOT describe the CLI allowlist as the boundary on an agent's reach without naming this distinction.

#### Scenario: A shell-capable agent is still bound by its toolset
- **WHEN** a conversation whose bound toolsets grant only read tools uses a shell to call a mutating tool on a bound MCP server directly
- **THEN** the call is refused, because the wiring's access decision is enforced outside the agent

#### Scenario: Configured and enforced access come from one source
- **WHEN** a pipeline's bound toolsets change and a new work unit is dispatched
- **THEN** both the allowlist passed to the runtime and the access enforced against the agent reflect the same change, with no separate configuration to keep in step

#### Scenario: Enforcement is not claimed where it does not exist
- **WHEN** an installation has not enabled enforcement outside the agent
- **THEN** the documented guarantee is the configured allowlist of a cooperating agent, and is described as such
