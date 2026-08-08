# toolset-composition-modes — design

## Context

Three facts, established by reading the code and the Claude Code documentation:

1. **`ToolingBinding.mode` was deleted on a wrong premise.** `capabilities-are-wiring` reasoned that with `AgentProfile.allowedTools` gone there was nothing to compose against. The intended counterpart was never the profile — it is the agent definition's `tools:` frontmatter in the checked-out repository.

2. **That frontmatter is inert today.** `internal/dispatch/templates/task.md:11` instructs the model to "read it and adopt that agent's role" — prose, not a subagent invocation. Claude Code's documented model is that a frontmatter `tools:` list scopes a subagent invoked through the Agent tool, while the main session is governed by `--allowedTools`. Nothing in this project invokes the agent as a subagent, so the agent's declared tools are read as narrative and enforced not at all.

3. **The runtime invents a capability.** `runtime-claude/runtime.js:127` passes `unit.allowedTools || 'Read'`. A conversation whose wiring declared nothing gets `Read` — a grant nobody wrote down.

Only the runtime can see the repository; the manager never checks it out. So any composition involving the agent definition must happen runtime-side, with the manager supplying the wiring's contribution and the mode.

## Goals / Non-Goals

**Goals:**

- Restore the vocabulary for "extend what this agent declares" versus "replace it".
- Make the agent definition's declared tools actually mean something.
- Remove the invented `Read` default without making empty allowlists hang a headless run.

**Non-Goals:**

- No change to how toolsets or MCPConfigs are declared, ordered, or deduped.
- Not reintroducing capabilities on `AgentProfile`.
- Not changing which component resolves refs — the manager still reads the CRs; only the final composition moves.
- No new CRD kinds.

## Decisions

### D1: Verify the layering before encoding it — a gate, not a task

The claim that `--allowedTools` governs the main session while frontmatter `tools:` scopes subagents is **inferred from Claude Code's architecture and is not documented**. Everything below depends on it.

Task 1.1 runs the real binary against a repo containing an agent definition with a restrictive `tools:` list, once with a broader `--allowedTools` and once without, and records what is actually enforced. If the frontmatter turns out to govern the main session too, `merge` may need no runtime work at all; if `--allowedTools` strictly wins, the runtime must do the union itself as designed below. Encoding the wrong one produces agents whose declared tools are silently ignored — the exact defect this change exists to fix.

### D2: The mode travels to the runtime; the manager stops claiming a final answer

```go
type ToolingBinding struct {
    // +kubebuilder:validation:Enum=merge;overwrite
    // +kubebuilder:default=merge
    Mode string `json:"mode,omitempty"`
    Refs []ObjectRef `json:"refs"`
}
```

`WorkUnit` gains `toolsMode`. `EffectiveAllowedTools` keeps computing the wiring's contribution — the concatenation of the bound toolsets — but that stops being the final allowlist and becomes one input to it. The doc comments must say so; today they read as though the manager decides.

`merge` is the default because it is the additive, less surprising of the two: a Pipeline that grants a toolset extends the agent rather than silently stripping whatever the agent declared for itself.

### D3: The runtime composes, because only it can

`runtime-claude` resolves `.claude/agents/<agent>.md` — the same path the lane templates already name — parses its YAML frontmatter, and takes `tools:` if present:

```
overwrite  ->  --allowedTools <wiring toolsets>
merge      ->  --allowedTools <agent tools ∪ wiring toolsets>   (dedup, agent's order first)
```

A missing agent file, or one with no `tools:`, contributes nothing, so `merge` degrades to the wiring's list alone — which is exactly today's behavior and keeps profiles without repositories working.

Parsing YAML frontmatter in the runtime is a small, contained addition: the delimiter is `---`, the field is a list of strings, and the runtime already reads the checkout. It does NOT become a general YAML consumer — anything it cannot parse is treated as "no declared tools" and logged, never as a failure that blocks the run.

### D4: Empty means empty, and must not hang

The invented `Read` goes. But omitting `--allowedTools` is not the alternative: per the documentation, a headless `-p` run in the default permission mode fires permission prompts, which in a pod means hanging until the idle TTL kills it.

So the runtime always passes `--allowedTools` with whatever composed, even empty, plus `--permission-mode dontAsk` so unlisted tools are denied outright rather than prompted for. An agent whose wiring declared nothing and whose definition declares nothing then genuinely has nothing — which is the operator's choice to make, expressed faithfully instead of quietly upgraded to `Read`.

This is a behavior change for any existing deployment relying on the `Read` default. It is the point of the change, and the new image tag is where it lands.

## Risks / Trade-offs

- [The whole design rests on undocumented CLI layering] → D1 makes verification the first task and a gate on the rest. If the assumption fails, the change narrows to D4 (removing the invented default) and the mode question reopens with evidence.
- [Removing the `Read` default silently disarms someone's working agent] → Deliberate and BREAKING; new image tag, release note. The alternative is keeping a grant nobody declared.
- [The runtime gains frontmatter parsing] → Contained to one field, failure-tolerant by design (unparseable = no declared tools), and it is the only component that can see the file.
- [`dontAsk` changes the permission posture for every runtime] → It is the correct posture for a headless pod, where a prompt is a hang. Making it explicit removes an implicit dependency on the default mode.
- [Mode is per-binding, so `toolsets` and `mcpConfigs` can disagree] → Intended; they compose against different things and an operator may reasonably extend servers while replacing tools.

## Open Questions

- Whether `mcpConfigs` needs a mode at all. MCP servers come from the compiled `mcp.json`, which the agent definition does not contribute to — so `merge` and `overwrite` may be indistinguishable there, exactly the reason the field was dropped. Resolve during D1: if the agent definition cannot contribute servers, `mcpConfigs` should stay mode-less and only `toolsets` regains one.
