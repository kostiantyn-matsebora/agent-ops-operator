# mcp-toolset-crd

## Why

What an agent can do is currently welded to who the agent is: MCP servers and the `allowedTools` allowlist live only on the AgentProfile, so granting a profile different tools for different wirings means cloning profiles, and bundles (vm-bundle today, ha-bundle pending) can ship MCP server configs but must document a hand-edit of a shared profile as "the one manual step". Making tool access a wiring concern — a new `MCPToolset` CRD (a named list of tool patterns) plus per-pipeline binding of both toolsets and existing `MCPConfig` CRs, each with `merge`/`overwrite` modes — lets one profile serve differently-tooled pipelines and lets bundles deliver ready-made tool access an operator attaches with one Pipeline field. A downstream effect: runtimes become generic — differentiated only by vendor backend and trust level (ServiceAccount), so one AgentRuntime per vendor (claude, copilot, gemini, kiro…) typically suffices.

## What Changes

- New `MCPToolset` CRD: purely a named LIST of tool patterns (`spec.tools`, e.g. `mcp__victorialogs__*`, or built-in tool names like `Bash`). No server definitions — servers stay exclusively in `MCPConfig` CRs.
- `Pipeline.spec` gains two symmetric tooling bindings, each `{mode: merge|overwrite (default merge), refs: [...]}`:
  - **`toolsets`** → `MCPToolset` refs, governing the allowlist: `merge` unions the toolsets' tools onto the profile's `allowedTools`; `overwrite` replaces the profile's allowlist entirely.
  - **`mcpConfigs`** → `MCPConfig` refs, extending the runtime's MCP servers per wiring: `merge` overlays the referenced configs onto the profile's compiled MCP (per-server-key, later wins); `overwrite` ignores the profile's MCP entirely.
  - Pipeline `Ready` validates both ref sets. Many-to-many throughout: pipelines bind many toolsets/configs, each reusable across pipelines.
- Both bindings materialize onto the Conversation at creation (mode + refs snapshot — consistent with how profileRef/channelRefs already dematerialize the pipeline); ref CONTENT re-resolves at use time, so toolset/config edits reach existing conversations while pipeline re-wiring affects only new ones. Conversations with no originating pipeline (`POST /task`, `/profile` commands on unwired channels) keep the profile's own tools and MCP unchanged.
- Resolution wiring: conversations with an `mcpConfigs` binding compile into a **conversation-owned** MCP ConfigMap (`agentops-mcp-conv-<name>`, GC'd with the conversation); everything else keeps the shared profile-keyed ConfigMap byte-identical. The effective `allowedTools` is computed per work unit at `/work` dispatch (allowlist changes need no pod restart). `mcpcompile` gains multi-spec overlay.
- vm-bundle ships a ready-made `MCPToolset` (`vm-observability`: the two tool namespaces) — the operator's manual step shrinks to one Pipeline stanza binding `mcpConfigs: [vm-logs, vm-metrics]` + `toolsets: [vm-observability]`.
- Manager RBAC: `mcptoolsets` get/list/watch (lowercase plural).

## Capabilities

### New Capabilities

- `mcp-toolset-model`: the MCPToolset CRD (pure tool list) and the wiring-level tool-access model — both Pipeline bindings' merge/overwrite resolution against the profile, conversation materialization, per-conversation MCP compilation, dispatch-time allowlist computation, and the raw-form incompatibility rule.

### Modified Capabilities

- `pipeline-model`: the "Pipeline CRD declares the wiring" requirement gains `spec.toolsets` and `spec.mcpConfigs` — tool-access binding is wiring (refs + mode only; content stays in MCPToolset/MCPConfig CRs); the no-credentials/no-runtime-selection stance is unchanged and Ready validates the new refs.
- `vm-bundle`: ships the `vm-observability` MCPToolset; the documented operator step becomes the Pipeline tooling stanza (direct profile editing remains a documented alternative).

## Impact

- **API**: new `api/v1alpha1/mcptoolset_types.go`; `PipelineSpec.Toolsets`/`.MCPConfigs`; `ConversationSpec` mirrors both; deepcopy + CRD regen (`chart/files/crds/`).
- **Controller**: `conversation_controller.go` (`ensureMCPConfigMap` becomes conversation-aware; conversation-owned CM when an mcpConfigs binding applies), `pipeline_controller.go` (Ready validates both ref sets), conversation-creation sites (`internal/httpapi/signals.go` routeSignalGroup, `internal/chat/router.go` defaultProfile paths) snapshot the bindings.
- **Compile/dispatch**: `internal/mcpcompile/` overlay entry; `internal/dispatch/` + `/work` handler resolve effective allowedTools.
- **Chart**: new CRD file, manager RBAC rule, vm-bundle toolset template + values.
- **Docs**: README (toolset/config binding concept + Pipeline stanza + bundle usage), CLAUDE.md (terminology), `config/samples/samples.yaml`.
- **Active-change overlap**: `ha-bundle` (ships a profile with hand-wired configRefs/allowedTools — can adopt wiring-level bindings as a follow-up, no conflict); `adapters-pure-implementation` touches vm-bundle chart files (coordinate merge order, no semantic overlap).
