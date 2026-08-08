# builtin-toolsets

## Why

`mcp-toolset-crd` made tool access a wiring concern, but only for MCP tools — built-in tools (`Read`, `Grep`, `Glob`, `Bash`, `Edit`, …) are still an opaque comma string on every AgentProfile. That leaves the model half-applied and the granular boundary unreachable: withholding shell access from one route means editing the profile, which changes every route using it. The concrete case is the k8s bundle, where `rbac.mode: full` grants an LLM-driven agent cluster-admin with `Bash` in its allowlist and no restraint above RBAC. Making built-ins named, chart-shipped toolsets turns "this route observes but does not execute" into a Pipeline stanza.

## What Changes

- The chart ships a small set of **built-in `MCPToolset` CRs**, split by risk rather than one blob (a single `builtin-tools` toolset would gain nothing over the string it replaces): `agentops-read` (`Read`, `Grep`, `Glob`), `agentops-shell` (`Bash`), `agentops-edit` (`Edit`, `Write`). Values-toggleable and values-extendable; names are values-overridable.
- **`POST /task {"pipeline": ...}` propagates the pipeline's tooling bindings**, not just its `channelRefs`. It copied channels and silently omitted toolsets/mcpConfigs — an asymmetry with no defensible reading, and the mechanism by which a task-lane conversation gets capabilities at all.
- **No profile-side API.** An earlier draft added `AgentProfile.spec.toolsets` and a two-layer resolution fold; it was implemented, then reverted (user direction 2026-08-08: *"neither toolsets nor mcps should be a part of profile, they should be a part of pipeline, since it's a glue"*). The per-route boundary never needed it — a Pipeline binding `overwrite` already withholds tools the profile grants, shipped and tested in `mcp-toolset-crd`. The justification was circular: it solved a breakage caused by tool-less profiles, which nothing required.
- `AgentProfile.spec.allowedTools` is untouched — profiles keep declaring tools inline until the follow-up moves capabilities off them entirely.

## Capabilities

### New Capabilities

- `builtin-toolset-catalog`: the chart-shipped built-in toolsets — their risk-based split, the values surface that toggles and extends them, and the guarantee that a fresh install still yields a working agent without any profile naming tools inline.

### Modified Capabilities

- `mcp-toolset-model`: `POST /task` with an explicit pipeline carries that pipeline's tooling bindings alongside its channel set. Allowlist resolution itself is unchanged — one layer, profile string ⊕ the conversation's binding.

## Impact

- **API**: none. (The reverted draft added one optional field; nothing shipped.)
- **Resolution**: `internal/httpapi` only — `handleTask` propagates the named pipeline's bindings.
- **Chart**: new built-in toolset templates + values under `global.builtinToolsets` (`global.` because subcharts read no other parent scope); manager RBAC on `mcptoolsets` unchanged (read-only).
- **Docs**: README built-in catalog section; CLAUDE.md terminology under `MCPToolset`; `config/samples/samples.yaml`.
- **Active-change overlap**: `all-in-one-crd` (38 tasks, active) materializes `MCPToolset` CRs from inline Pipeline blocks and widens manager RBAC on `mcptoolsets` — it must extend its inlining to `AgentProfile.toolsets` too, or explicitly scope it out. Coordinate merge order; no semantic conflict, but the two changes both touch the toolset resolution path.
- **Follow-ups this unblocks**: `k8s-mcp-tooling`, then `runtime-drop-kubectl`. And a larger one the revert surfaced — **moving `allowedTools` and `spec.mcp` off `AgentProfile` entirely**, so the profile is identity (repo, role, prompts, env, limits) and the Pipeline is capability. That collapses machinery rather than adding it: `ToolingBinding.mode` becomes meaningless with nothing to compose against, `mcpcompile.Compile`/`CompileOverlaid` become one function over a list, `RawMergeError` and the raw-form-can't-merge rule disappear, and the profile-owned `agentops-mcp-<profile>` ConfigMap goes away. Its cost is that conversations with no pipeline get no capabilities — consistent with unclaimed sources dropping signals, but it changes the bare `POST /task` demo, which needs a deliberate answer.
