# capabilities-are-wiring — design

## Context

Three CRs describe an agent today: `AgentProfile` (who it is), `AgentRuntime` (what executes it), `Pipeline` (the wiring). Capabilities are split across two of them — `AgentProfile.spec.allowedTools` and `spec.mcp` on one side, `Pipeline.spec.toolsets` and `spec.mcpConfigs` on the other — and the machinery that reconciles the split is most of what `mcp-toolset-crd` shipped:

| Mechanism | Exists only because profiles carry capabilities |
|---|---|
| `ToolingBinding.mode` (`merge`/`overwrite`) | composes wiring capabilities against the profile's |
| `mcpcompile.Compile` + `CompileOverlaid` | one for the profile's tri-form base, one for base+overlays |
| `RawMergeError`, the raw-form-can't-merge rule | a raw profile `mcp.json` is opaque, so nothing can merge onto it |
| profile-owned `agentops-mcp-<profile>` ConfigMap | the profile's own compiled MCP needs somewhere to live |
| two-branch `ensureMCPConfigMap` | binding-less conversations mount the profile CM, bound ones their own |

Only `internal/controller/conversation_controller.go` reads `profile.Spec.MCP`, and only `internal/httpapi/server.go` reads `profile.Spec.AllowedTools`, so the code surface is small. The API surface is not: `spec.mcp` is public, documented as vm-bundle's alternative wiring path, used by the samples and by the active `ha-bundle` change.

The user's direction (2026-08-08): *"neither toolsets nor mcps should be a part of profile, they should be a part of pipeline, since it's a glue."*

## Goals / Non-Goals

**Goals:**

- One source of an agent's capabilities: the Pipeline routing it.
- Delete the reconciliation machinery the split required, rather than move it.
- Keep every existing behavior reachable — including the five-minute demo — through wiring rather than through profile fields.

**Non-Goals:**

- `AgentProfile.spec.env` stays. It carries the agent's own credentials (an HA token, an API key) as `valueFrom`; those are identity, not route capability, and moving them would put secrets-by-name into the wiring object.
- `maxTurns`, `resources`, `prompt`/`replyPrompt`, `runtimeRef` stay — limits, presentation, and execution selection are not capabilities.
- No change to how the allowlist is enforced. It is still an opaque string handed to the runtime; this changes only where it is declared.
- Not merging `AgentProfile` into `Pipeline`. Profiles stay separately addressable (`/<profile> <task>`) and reusable across routes — that reuse is the reason capabilities must leave them.

## Decisions

### D1: Capability-only Pipelines answer the pipeline-less paths

This is the decision the change hinges on. Three conversation paths have no routing pipeline:

1. `POST /task` with no `pipeline` field — **the documented five-minute demo**.
2. `/<profile> <task>` chat commands: the named profile is not the channel pipeline's profile, so the pipeline's capabilities would be the wrong agent's (decided in `mcp-toolset-crd`, still right).
3. `adoptThread` on an unwired channel — already refuses, no profile resolves.

After this change those conversations have no capabilities at all. Options considered:

**(a) Capability-only Pipelines — CHOSEN.** A `Pipeline` naming a `profileRef` with no `signalSourceRefs` and no `channelRefs` declares that profile's baseline capabilities. Pipeline-less conversations resolve it by profile name. Routing pipelines override per route, exactly as now.

Why: it is explicit — an operator declares a baseline rather than having one inferred — and it keeps the profile pure while leaving capability truth in a `kubectl get`-able wiring object. A Pipeline with no sources and no channels is already legal today (both fields are optional), so this is a semantic, not a schema, addition.

The obvious objection is that this looks like profile capabilities with one indirection. The difference is real: the declaration is a separate, separately-reviewable object that a route can override, and the profile stays reusable across routes with genuinely different capabilities — which is the whole reason capabilities left it.

**(b) Resolve the profile's oldest Ready routing Pipeline — REJECTED.** Symmetric with `PipelineForChannel`, and needs no new concept. Rejected because the ambiguity is worse here than for channels: the entire point of this model is one profile serving differently-tooled routes, so "which route's capabilities does `/task` get?" has no defensible default, and silently picking the oldest would hand the task lane whichever route happened to be created first.

**(c) Accept no capabilities — REJECTED.** Purest reading (unwired means inert, as for sources and channels), and it would break the demo the README leads with. Rejected as a default; note that (a) degrades to (c) when no capability Pipeline exists, which is the correct behavior for a genuinely unwired profile.

Exactly one capability-only Pipeline per profile is allowed; a second reports a condition and neither applies, mirroring the source-conflict rule rather than inventing a precedence.

### D2: Bindings lose `mode`

```go
type ToolingBinding struct {
    Refs []ObjectRef `json:"refs"`
}
```

With one source of capabilities there is nothing to merge against, so `merge` and `overwrite` name the same behavior. Keeping a field whose two values are indistinguishable would be worse than removing it — it would read as a control that does nothing.

Composition across refs is unchanged and still ordered: later refs win on server-key collisions, tool lists concatenate with dedup.

### D3: `mcpcompile` collapses to one entry point

```go
func Compile(configs []agentopsv1alpha1.MCPConfigSpec) (Result, error)
```

`Compile`/`CompileOverlaid` merge into this. The env-placeholder machinery for secret-backed headers is untouched — the manager still reads no Secrets. `RawMergeError` and the raw-form conflict are deleted, along with the `ToolingResolved` condition's `IncompatibleMCPForm` reason.

`ensureMCPConfigMap` loses its profile branch: a conversation with an `mcpConfigs` binding compiles into `agentops-mcp-conv-<name>`; one without mounts nothing and the runtime gets an empty `mcp.json`, which is what a capability-less conversation should get.

### D4: The raw escape hatch moves to MCPConfig, exclusively

`MCPConfigSpec` gains `configMapRef`/`secretRef` alongside `servers`. A raw config is exclusive: binding one alongside any other config is an error surfaced on the conversation, because a hand-written `mcp.json` is opaque and cannot be combined. That is the same rule as today, relocated — the difference is it now applies to a bound config rather than to the profile, so it never blocks a merge that the operator did not ask for.

Dropping the escape hatch entirely is the alternative (see Open Questions): it exists for operators with a hand-maintained `mcp.json`, and `MCPConfig` CRs cover most of that need.

### D5: Migration must precede the upgrade

Removing a field from a CRD **deletes that data from every stored object** on the next write. An operator who upgrades first and migrates second loses their profiles' tools and MCP config silently.

The migration is therefore documented as a pre-upgrade step, with a `kubectl` recipe that lists every profile carrying `allowedTools` or `mcp` so the operator can see exactly what must move. The chart's `NOTES.txt` prints the same check post-install. Deprecating the fields for one minor version before removal is the safer alternative and is worth considering if this ships close to a release.

## Risks / Trade-offs

- [Silent data loss on upgrade] → D5: pre-upgrade migration recipe, `NOTES.txt` check, and BREAKING in the release notes. The strongest mitigation is a deprecation release first, which is the recommendation if timing allows.
- [Capability-only Pipelines are a new concept to learn] → It is a semantic, not a schema, addition — a Pipeline with no sources or channels already renders. The README documents it as "a profile's baseline", and the alternative (b) trades this for a silent, ambiguous default.
- [`/<profile>` commands lose tools unless a capability Pipeline exists] → Correct under the model, and D1 makes the fix a one-object declaration. Documented explicitly, since it is the path most likely to surprise.
- [vm-bundle loses its documented profile-edit alternative] → It becomes Pipeline-only, which is what the bundle's README already recommends; the alternative existed for setups predating wiring-level bindings.
- [Churn across active changes] → `ha-bundle` and `all-in-one-crd` both assume profile capabilities or `ToolingBinding.mode`. Named in the proposal; sequencing this first is cheaper than reconciling twice.
- [The change deletes tests that pin real behavior] → The raw-form-merge tests and the byte-identical profile-ConfigMap tests describe behavior that ceases to exist. They are deleted deliberately, with the replacement assertions named in tasks, per the repo rule that dispatch/compile semantics change by changing tests on purpose.

## Migration Plan

1. **Before upgrading**: run the audit recipe; for each profile carrying capabilities, create or extend the Pipeline(s) routing it, and add a capability-only Pipeline for profiles reachable via `POST /task` or `/<profile>`.
2. Upgrade. The removed fields are pruned; capabilities now resolve from wiring.
3. Chart users: the bundles migrate themselves; only hand-authored profiles need step 1.

Rollback is a chart downgrade plus re-adding the profile fields from the audit output — which is why the audit is a recipe the operator keeps, not a one-shot.

## Open Questions

- **D1 needs sign-off before implementation.** Capability-only Pipelines are the proposal; (b) and (c) are recorded with their reasons. This decides whether a bare `POST /task` still works.
- Keep or drop the raw `mcp.json` escape hatch (D4). Leaning keep-and-relocate, since removing two things in one breaking change makes the migration harder to reason about.
- Whether to ship a deprecation release before removal (D5). Leaning yes if a release is imminent, no if the API group is still pre-1.0 and users are few — the `agentops.dev/v1alpha1` version already signals instability.
