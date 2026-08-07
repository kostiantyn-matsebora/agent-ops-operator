# mcp-toolset-crd — design

## Context

Tool access today: `AgentProfileSpec.MCP` (tri-form: inline servers / `configRefs` to MCPConfig CRs / raw ConfigMap-Secret; merge order raw < configRefs < inline, per-server-key whole-server replacement) compiled by `internal/mcpcompile.Compile` into a **profile-keyed, profile-owned** ConfigMap `agentops-mcp-<profile>` mounted into runtime pods; `AgentProfileSpec.AllowedTools` (opaque comma-separated string) flows per-work-unit at dispatch time (`dispatch.Next`, called from the `/work` handler which fetches the profile from `conv.Spec.ProfileRef`).

Wiring: `Pipeline` is THE wiring CR (sources[] × channels[] + profileRef), spec-pinned to "carry no credentials, config, or runtime selection". Pipelines are **dematerialized at conversation creation**: `routeSignalGroup` and the router's `defaultProfile` paths copy `profileRef`/`channelRefs` onto the Conversation and forget the pipeline; there is no `pipelineRef` anywhere, and reverse lookup is ambiguous (channels are shareable). Two creation paths have no pipeline at all: `POST /task` and `/profile` commands on unwired channels.

Bundles can ship MCPConfig CRs (vm-bundle does) but wiring them in is a documented manual profile edit that then applies to every pipeline using that profile.

The division of labor in this change (per user direction): **`MCPConfig` keeps sole ownership of server definitions; `MCPToolset` is purely a list of tools; the Pipeline binds both to the wiring** — toolsets govern the allowlist, MCPConfig refs extend the runtime's MCP.

**Consequence — runtimes become generic (user observation 2026-08-07):** with tooling moved to wiring and persona/repo already on profiles, `AgentRuntime` differentiates only by vendor backend (image + credential env — claude, copilot, gemini, kiro…) and by trust level (`serviceAccountName`, the runtime's security identity). The steady state is one runtime per vendor × trust level — commonly one per vendor. The SA deliberately stays runtime-level and does NOT move to wiring: a Pipeline selecting a privileged SA would turn pipeline-edit rights into privilege escalation; RBAC review concentrates on runtimes.

## Goals / Non-Goals

**Goals:**

- Named, reusable toolsets (tool-pattern lists) and per-wiring MCP config attachment, both bound through Pipeline many-to-many, so one profile serves differently-tooled pipelines.
- Two modes per binding: `merge` (extend the profile) and `overwrite` (replace it).
- Bundles ship a toolset + MCPConfigs; attaching both is a single Pipeline stanza.
- Toolset/config-less behavior byte-identical to today (ConfigMap names, work units, profile semantics all unchanged).

**Non-Goals:**

- No server definitions on MCPToolset — that would duplicate MCPConfig's job.
- No per-ref modes, no toolset nesting, no per-source or per-channel overrides — mode and refs are per-pipeline per-binding.
- No binding fields on AgentProfile (profiles already own tools natively) and none user-facing on Conversation (the fields are materialized state).
- Not retrofitting ha-bundle/k8s-bundle in this change (follow-ups).

## Decisions

### D1: MCPToolset CRD — a pure tool list

```go
type MCPToolsetSpec struct {
    // Tools this toolset grants: MCP namespaces ("mcp__victorialogs__*") or
    // built-in tool names ("Bash") — any allowlist entry is legal.
    Tools []string `json:"tools"`
}
```

No status conditions: there is nothing to resolve (patterns are opaque to the manager, passed through to the runtime like `allowedTools` today). A LIST (unlike the profile's comma string) because merging needs element-wise union/dedup; the profile's string stays untouched for compatibility and is split only at resolution time.

### D2: Two symmetric bindings on Pipeline

```go
// PipelineSpec additions
// +optional
Toolsets *ToolingBinding `json:"toolsets,omitempty"`     // refs → MCPToolset
// +optional
MCPConfigs *ToolingBinding `json:"mcpConfigs,omitempty"` // refs → MCPConfig

type ToolingBinding struct {
    // +kubebuilder:validation:Enum=merge;overwrite
    // +kubebuilder:default=merge
    Mode string `json:"mode,omitempty"`
    Refs []ObjectRef `json:"refs"`
}
```

One shared struct, two independent bindings with independent modes (an operator can overwrite the allowlist while merging servers, or vice versa). The Pipeline is the many-to-many join for both. This amends the pipeline-model's "no config" sentence deliberately: tool-access *binding* (refs + mode) is wiring — it says which capabilities this route grants — while all *content* stays in MCPToolset/MCPConfig CRs; the Pipeline still carries no credentials, no server definitions, no runtime selection. Pipeline `Ready` gains ref validation for both sets (same `MissingReferences` vocabulary).

### D3: Materialize both bindings on the Conversation; resolve refs lazily

`ConversationSpec` mirrors both `Toolsets`/`MCPConfigs` bindings, snapshotted at creation wherever a pipeline originates the conversation (`routeSignalGroup` — pipeline already in scope — and both router `defaultProfile` paths, which need the pipeline plumbed through alongside the profile name). `POST /task` and `/profile`-command conversations get neither → pure profile behavior. This follows the established materialization pattern (profileRef/channelRefs) and the spec sentence "Conversation fields are materialized per-conversation state, not wiring" — no `pipelineRef` is introduced, and reverse pipeline lookup (ambiguous by design) is never needed.

Refs are snapshotted, content is not: every use re-reads the MCPToolset/MCPConfig CRs, so fixing a config's URL or extending a toolset heals existing conversations, while re-wiring a pipeline (different refs/modes) affects only new conversations — the same edit semantics profiles already have.

### D4: Resolution semantics

**Allowlist** (from the `toolsets` binding; `T1..Tn` in ref order):
- `merge`: profile's `allowedTools` entries (comma-split) ∪ T1.tools ∪ … ∪ Tn.tools — dedup, first occurrence keeps position.
- `overwrite`: T1.tools ∪ … ∪ Tn.tools alone; the profile's allowlist — including built-ins like `Read`/`Bash` — is ignored, which is why toolsets may name built-ins.

**MCP servers** (from the `mcpConfigs` binding; `C1..Cn` in ref order):
- `merge`: profile's compiled MCP map ⊕ C1.servers ⊕ … ⊕ Cn.servers — per-server-key, later wins (bound configs override the profile on collision, mirroring the existing "inline wins" precedence direction).
- `overwrite`: C1 ⊕ … ⊕ Cn alone; the profile's `mcp` is ignored entirely.
- **Raw-form conflict**: a profile whose `mcp` uses `configMapRef`/`secretRef` cannot merge (the file is opaque). `merge` mode over such a profile is an error surfaced on the conversation's MCP condition/event naming the incompatibility; `overwrite` works (profile MCP is ignored). Never silently dropped — a half-merged config is the worst outcome.

The two bindings are independent: binding only `mcpConfigs` without a toolset granting `mcp__<key>__*` entries yields servers the agent can't call (and vice versa) — legal, documented, and exactly why bundles ship both halves.

### D5: Per-conversation ConfigMap only when an mcpConfigs binding applies

`ensureMCPConfigMap` becomes conversation-aware: conversations without an `mcpConfigs` binding keep the shared, profile-owned `agentops-mcp-<profile>` (existing name, owner, bytes — pinned by existing tests) — note a toolsets-only binding changes nothing MCP-side, since the allowlist never touches the ConfigMap. Conversations WITH an `mcpConfigs` binding compile into `agentops-mcp-conv-<conversation>` owned by the Conversation (ownerRef GC, same lifecycle as the runtime pod that mounts it). A profile-keyed-plus-binding name was rejected: two pipelines with different configs sharing one profile would clobber a shared CM, and hashing binding content into the name leaks garbage CMs on every edit. `mcpcompile` gains an overlay entry — `CompileOverlaid(base *MCPSpec, overlays []MCPConfigSpec, refs map[string]MCPConfigSpec, mode)` shape — reusing the existing compilation and env-placeholder machinery (secret-backed headers keep compiling to `valueFrom` env; the manager still reads no Secrets).

### D6: allowedTools resolves at dispatch

The `/work` handler already fetches the profile per work unit; it additionally fetches the conversation's toolsets and `dispatch.Next` receives the effective allowlist (computed per D4) instead of reading `profile.Spec.AllowedTools` directly. Per-work-unit resolution means toolset edits apply from the next work unit with no pod restart — matching today's profile-edit behavior. A toolset or config ref missing at resolution time fails the work unit / compilation visibly (conversation condition), consistent with missing-profile handling.

### D7: vm-bundle ships `vm-observability`

New template gated on the bundle + any mcp component: `MCPToolset vm-observability` with `tools` carrying the namespaces of enabled components only (`mcp__victorialogs__*` when vmlogs is on, `mcp__victoriametrics__*` when vmmetrics is on). The documented operator step becomes one Pipeline stanza:

```yaml
spec:
  mcpConfigs: { refs: [{name: vm-logs}, {name: vm-metrics}] }
  toolsets:   { refs: [{name: vm-observability}] }
```

Editing the profile directly remains a documented alternative. ha-bundle adoption is a named follow-up, not in scope.

## Risks / Trade-offs

- [Tool truth spread across profile + two bindings complicates debugging] → Effective access is deterministic from (profile, conversation's two bindings) — all visible in CRs; README documents the resolution table; `overwrite` is the escape hatch when layering gets confusing.
- [Server-key collisions between bound configs silently shadow (later wins)] → Same rule as existing MCP merging (documented, deterministic); bundles use fixed unique keys precisely so composition is collision-free.
- [Half-bound tooling (servers without allowlist or vice versa) yields dead tools] → Legal by design (the bindings are independent); documented with the bundle stanza always showing both halves.
- [Per-conversation CMs multiply objects] → Only for mcpConfigs-bound conversations, conversation-owned (GC'd), bounded by conversation cardinality.
- [Materialized bindings go stale vs pipeline edits] → Deliberate (D3), consistent with profile/channel materialization; conversations are short-lived by design.
- [`overwrite` toolsets can strip built-ins the lane templates assume] → Documented sharp edge; the mode exists to express minimal-tool agents; lane behavior degrades to text-only rather than breaking.
- [RBAC plural typo breaks informers silently] → Known gotcha (CLAUDE.md): `mcptoolsets` lowercase plural in `chart/templates/rbac.yaml`, covered by an envtest list.

## Migration Plan

Purely additive API: one new CRD, two optional Pipeline fields, mirrored Conversation fields, defaulted modes. No values or behavior change for anything existing — binding-less paths are pinned byte-identical (CM name/owner, WorkUnit fields). CRD regen + chart CRD file + RBAC rule land together; the vm-bundle toolset template is independent and can trail. Rollback = stop referencing the bindings (fields optional); orphaned per-conversation CMs GC with their conversations.

## Open Questions

- None blocking. Whether `AgentProfile.allowedTools` should become a list (deprecating the comma string) is deliberately out of scope — noted as a possible pre-1.0 API cleanup alongside the group rename.
