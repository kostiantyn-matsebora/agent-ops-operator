# capabilities-are-wiring

## Why

`AgentProfile` still carries `allowedTools` and `spec.mcp` — capability declarations sitting on the identity object. Every other capability moved to the Pipeline (`mcp-toolset-crd`), and the project's stated rule is that wiring lives ONLY there: `SignalSource` carries no profile, `Channel` carries no default profile, unclaimed things are inert. The profile is the last exception, and it is why the model reads as half-applied — a Pipeline can override tools, but only because it composes against a base the profile shouldn't have had.

Removing that base **collapses machinery rather than adding it**, which is unusual enough to be the main argument: the merge/overwrite mode, the two compile entry points, the raw-form conflict, and the profile-owned ConfigMap all exist solely to reconcile profile capabilities with wiring capabilities. With one source, they stop being necessary.

## What Changes

- **BREAKING**: `AgentProfile.spec.allowedTools` and `AgentProfile.spec.mcp` are removed. The profile becomes identity only — repository, agent role, prompts, env, limits, runtime selection.
- **`ToolingBinding.mode` is removed.** With nothing to compose against, `merge` and `overwrite` are the same thing; both bindings become ordered ref lists.
- **`internal/mcpcompile` collapses**: `Compile` and `CompileOverlaid` become one function over an ordered list of `MCPConfigSpec`. `RawMergeError` and the "raw form cannot merge" rule are deleted outright — there is no base to merge onto.
- **The profile-owned ConfigMap `agentops-mcp-<profile>` is removed.** Every conversation with MCP compiles into its own `agentops-mcp-conv-<name>`, GC'd with it. One code path instead of two.
- **The raw mcp.json escape hatch moves to `MCPConfig`** (`configMapRef`/`secretRef` as alternatives to `servers`), with an exclusivity rule: a raw config may not be combined with others. Dropping the escape hatch entirely is the alternative — see the design's open questions.
- **Capability resolution for pipeline-less conversations is the change's central decision.** `POST /task` without a pipeline, and `/<profile>` commands, currently fall back to the profile's own tools; after this change there is nothing to fall back to. The design proposes **capability-only Pipelines** (a Pipeline naming a profile with no sources and no channels = that profile's baseline capabilities) and records the two rejected alternatives. This needs sign-off before implementation — it decides whether the five-minute demo still works.
- Chart, bundles, and samples move their profile capabilities onto Pipelines; the demo gains its capability Pipeline.

## Capabilities

### New Capabilities

- `profile-is-identity`: the AgentProfile carries no capabilities; what an agent MAY do comes exclusively from the Pipeline routing it, including for conversations with no routing pipeline.

### Modified Capabilities

- `mcp-toolset-model`: bindings lose `mode` and become ordered ref lists; allowlist resolution is the bound toolsets alone; MCP compilation is the bound configs alone, always into a conversation-owned ConfigMap; the raw-form merge conflict ceases to exist.
- `pipeline-model`: the Pipeline is the sole source of an agent's capabilities, and a Pipeline with neither sources nor channels is a legal, meaningful object — a profile's baseline capability declaration.
- `k8s-bundle`: the bundle's profile declares no tools; its capabilities come from the rendered Pipelines, including a capability Pipeline for the `POST /task` demo lane.
- `vm-bundle`: the documented "edit your profile" alternative for attaching vm tooling is removed — the Pipeline stanza becomes the only way, since profiles can no longer carry `mcp.configRefs`.

## Impact

- **API (BREAKING)**: `AgentProfileSpec.AllowedTools`, `AgentProfileSpec.MCP`, `ToolingBinding.Mode` removed; `MCPConfigSpec` gains the raw forms. Deepcopy + CRD regen. **CRD field removal deletes data on existing objects** — the migration must be run before upgrade, not after.
- **Controller**: `ensureMCPConfigMap` loses its profile branch entirely; `Merges()` and its call sites go.
- **Compile/dispatch**: `internal/mcpcompile` roughly halves; `dispatch.EffectiveAllowedTools` becomes a concatenation of the bound toolsets.
- **Chart**: `k8s-bundle` profile + capability Pipeline; `vm-bundle` docs; parent values; `config/samples/samples.yaml`.
- **Docs**: README (the resolution table shrinks to one row per binding; migration section), CLAUDE.md (terminology under `AgentProfile`, `Pipeline`, `MCPToolset`).
- **Active-change overlap**: `ha-bundle` ships a profile with `configRefs` + `allowedTools` and must move them onto its Pipeline. `all-in-one-crd` inlines `Pipeline.spec.toolsets` and assumes `ToolingBinding.mode` exists. `k8s-mcp-tooling` binds tooling on the bundle's Pipeline and composes cleanly. Sequence this before `all-in-one-crd`, or reconcile there.
- **Supersedes**: the pre-1.0 cleanup `mcp-toolset-crd` noted (converting `allowedTools` to a list) — the field goes away instead.
