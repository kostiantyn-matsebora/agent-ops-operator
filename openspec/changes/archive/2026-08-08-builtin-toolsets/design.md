# builtin-toolsets — design

## Context

`mcp-toolset-crd` (archived 2026-08-08) introduced `MCPToolset` and two `ToolingBinding` stanzas on `Pipeline`. Its spec explicitly allows toolsets to name built-in tools — "`overwrite` = the toolsets' tools alone, ignoring the profile's allowlist including built-ins, which is why toolsets may name built-ins" — but nothing ships such a toolset, and every profile still carries `allowedTools: "Read,Grep,Glob,Bash"` inline.

Resolution today (`dispatch.EffectiveAllowedTools`): a conversation with no `toolsets` binding gets `profile.Spec.AllowedTools` verbatim; with one, `merge` unions the comma-split profile string with each bound toolset, `overwrite` uses the toolsets alone. Bindings are snapshotted onto the Conversation at creation and only by pipeline-originated paths — `POST /task` and `/<profile>` commands carry none by design.

Two consequences block the granular boundary:

1. **Merge re-grants what a route tried to withhold.** A profile declaring `Bash` hands it to every route, whatever the pipeline binds, unless the pipeline uses `overwrite` — which then also discards the profile's MCP-tool entries, so `overwrite` is not a targeted instrument.
2. **Tool-less profiles break binding-less conversations.** The obvious fix (profiles stop declaring tools) gives `POST /task` and `/<profile>`-command conversations an empty allowlist. The k8s-bundle demo advisor is precisely a `POST /task` flow, so the flagship onboarding path would break.

`mcp-toolset-crd` named "no binding fields on AgentProfile" a non-goal, reasoning that "profiles already own tools natively". That reasoning holds only while `allowedTools` remains the profile's tool surface. This change removes that premise, so the non-goal is reopened deliberately rather than by accident.

## Goals / Non-Goals

**Goals:**

- Built-in tools become named, reviewable, chart-shipped `MCPToolset` CRs, split so a route can take observation without execution.
- A profile can be tool-less and still work on every path, including `POST /task` and `/<profile>` commands.
- Withholding shell from one route is a Pipeline edit, never a profile edit.
- Existing installs keep working untouched: profiles that declare `allowedTools` and nothing else behave exactly as today.

**Non-Goals:**

- Not removing or deprecating `AgentProfile.spec.allowedTools` — it stays the compatibility path and the floor. (Converting it to a list remains the pre-1.0 cleanup `mcp-toolset-crd` already noted.)
- No `mcpConfigs` binding on AgentProfile. Profiles already own MCP natively through the tri-form `spec.mcp`, which has no equivalent gap — this change fixes the allowlist asymmetry only.
- No per-tool RBAC, no runtime-side enforcement. The allowlist is passed to the runtime exactly as today; this changes where it is *declared*, not who enforces it.
- Not shipping a Kubernetes MCP server or touching `runtime-claude` — named follow-ups, out of scope here.

## Decisions

### D1: Risk-split toolsets, not one blob

```yaml
agentops-observe:  {tools: [Read, Grep, Glob]}     # observation
agentops-shell: {tools: [Bash]}                 # execution
agentops-edit:  {tools: [Edit, Write]}          # mutation of the workspace
```

(Implemented as `observe`/`shell`/`edit` keys under `global.builtinToolsets` — `global.` because a subchart reads no other parent scope, and k8s-bundle's profile must reference the same names the parent renders, so a rename propagates to both.)

Three CRs rather than one `builtin-tools`, because the whole value is composability: `toolsets: {refs: [agentops-observe]}` is a statement a reviewer can check, while a single blob is the comma string with extra steps. Names are values-overridable and the tool lists values-extendable (an operator adding `Skill` or `WebFetch` should not need a new CR kind). The chart renders them whenever `global.builtinToolsets.enabled` (default `true`) — they are inert objects, so shipping them by default costs nothing and makes the reference documentation live in the cluster.

`agentops-` prefixes keep them from colliding with operator-authored toolsets in the same namespace.

### D2: `AgentProfile.spec.toolsets`, resolved on every path

```go
// AgentProfileSpec addition
// +optional
Toolsets *ToolingBinding `json:"toolsets,omitempty"`
```

The same `ToolingBinding` struct as Pipeline, so the vocabulary is learned once. Its `mode` governs how the profile's toolsets compose with the profile's own `allowedTools` string:

- `merge` (default): `allowedTools` entries ∪ each toolset's tools.
- `overwrite`: the toolsets alone — the escape hatch for a profile migrating off the string entirely while something else still writes to it.

This is resolved wherever a profile is resolved, so it reaches `POST /task`, `/<profile>` commands, and pipeline-originated conversations alike. Unlike the Pipeline bindings it is NOT snapshotted onto the Conversation: the profile is re-read per work unit already, so profile-level tools follow profile edits with the same semantics tools have today.

### D3: Two-layer resolution, profile first

The effective allowlist becomes a fold, not a special case:

```
profileEffective = resolve(profile.allowedTools, profile.toolsets)
effective        = resolve(profileEffective,     conversation.toolsets)
```

Both steps use the existing `EffectiveAllowedTools` rules (dedup, first occurrence keeps position; `overwrite` discards the left side). The key property: a pipeline binding composes against the profile's *effective* tools, so `overwrite` at the pipeline still means "exactly these tools" and a route can drop shell by binding `[agentops-observe]` in `overwrite` mode while the profile keeps `agentops-shell` for its other routes.

Binding-less conversations simply stop after the first line — which is why the profile layer, not the pipeline layer, is what makes tool-less profiles viable.

### D4: `POST /task` propagates the named pipeline's bindings

`handleTask` already copies `p.Spec.ChannelRefs` when the request names a pipeline; it will also copy `Toolsets`/`MCPConfigs`. Copying the channel set but not the tooling has no defensible reading — the caller named a pipeline, and "which surfaces" and "which tools" are equally that pipeline's wiring. Requests with no `pipeline` field stay binding-less, unchanged.

This is a behavior change for existing `POST /task {"pipeline": ...}` callers: they now get the pipeline's tools too. That is the intended semantics, and it can only widen or replace tooling for a caller who explicitly opted into a pipeline.

### D5: Migration is opt-in, and the default install is pinned

`allowedTools` keeps working untouched; a profile that sets neither `toolsets` nor `allowedTools` gets an empty allowlist exactly as it does today. The chart's own profiles migrate in this change (k8s-bundle's `k8s-engineer` moves from the inline string to `toolsets: {refs: [agentops-observe, agentops-shell]}`), which is also the migration example the README documents.

An integration test pins the thing most likely to break silently: a default `helm install` + `POST /task` must still yield a work unit whose `allowedTools` contains `Read` and `Bash`. That assertion fails loudly if the toolsets stop rendering, get renamed, or stop resolving on the binding-less path.

## Risks / Trade-offs

- [Tool truth now spans profile string + profile toolsets + pipeline toolsets] → One fold, evaluated left to right, all three visible in CRs; README carries the worked example and `kubectl get mcptoolset` shows the content. The alternative — leaving built-ins unreachable from wiring — keeps the simpler model and the unusable boundary.
- [Reopening the AgentProfile non-goal invites "why not mcpConfigs too?"] → Answered in Non-Goals: `spec.mcp` has no equivalent gap, so symmetry here would be change for its own sake. Recorded so the asymmetry is a decision rather than an oversight.
- [Tool-less profiles are a footgun mid-migration] → A profile with neither field is legal and gets nothing, exactly as today; the pinned default-install test catches the chart's own profiles regressing, and the README migration note is explicit that removing `allowedTools` requires adding `toolsets` in the same edit.
- [`POST /task` binding propagation changes existing behavior] → Only for callers passing `pipeline`, who already asked for that pipeline's wiring; documented in the README API section.
- [Overlap with `all-in-one-crd`] → That change materializes toolsets from inline Pipeline blocks and must decide whether `AgentProfile.toolsets` is inlinable too. Recorded in that change's design as `D-interaction`, with a recommendation to scope it in: its own D7 rejects giving one field two inlining semantics, which is exactly what inlining the Pipeline binding but not the profile binding would create. No semantic conflict; `builtin-toolsets` landed first, so `all-in-one-crd` is the one that moves.
- [Chart ships three more CRs by default] → They are opaque string lists with no controller, no status, and no reconcile cost; `builtinToolsets.enabled: false` removes them for operators who want a bare install.

## Migration Plan

1. API + resolution first (additive, optional field, no behavior change until a profile sets `toolsets`).
2. Chart: toolset templates + values, then migrate k8s-bundle's profile off the inline string in the same version.
3. Docs and samples last, once the resolution table is settled.

Rollback = stop referencing `toolsets` on profiles; the field is optional and `allowedTools` never stopped working.

## Open Questions

- Whether `agentops-edit` should ship at all by default, given no chart-rendered profile binds it. Leaning yes: it costs nothing, and its absence would make the catalog look like "the tools we approve of" rather than a neutral vocabulary.
