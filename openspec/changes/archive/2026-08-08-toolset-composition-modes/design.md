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

## Evidence (D1 resolved — task 1.1)

Six runs of the real binary (`claude 2.1.226`, `-p --output-format stream-json`,
`--setting-sources project`) against a checkout containing
`.claude/agents/probe*.md`. The probe tool is `Write`, not `Bash`: a read-only
`Bash` command like `echo` is auto-approved by the sandbox heuristics regardless
of the allowlist, which makes it useless as a probe — the first two runs looked
like "`--allowedTools` does nothing" until `Write` showed otherwise.

| # | flags | definition declares | outcome |
|---|-------|---------------------|---------|
| a | `--allowedTools Bash,Read` + `dontAsk` | `Read, Glob` | `Bash` ran (inconclusive — `echo` is auto-approved) |
| f | `--allowedTools Read,Write` + `dontAsk`, no `--agent` | narrower / other | `Write` **succeeded** — the definition does NOT narrow the main session |
| d | `--allowedTools Read` + `dontAsk` | `Read, Write` | `Write` **denied** — the definition does NOT widen the main session |
| W-b | `--allowedTools Read`, default mode | — | `Write` denied ("haven't granted it yet"); the run **completed**, no hang |
| c | no `--allowedTools`, `--permission-mode dontAsk` | — | `Write` denied ("running in don't ask mode"); run completed |
| e | `--agent probe-write --allowedTools Read` + `dontAsk` | `Read, Write` | init `tools` = 2 (Read, Write) but `Write` **denied** |

Four conclusions:

1. **`--allowedTools` is the sole permission authority for the main session.**
   A definition's `tools:` neither widens (d) nor narrows (f) it. Without
   `--agent`, the file only registers a subagent — it appeared in the init
   event's `agents` list in every run and governed nothing else.
2. **`--agent <name>` intersects availability, and cannot grant.** Run e cut the
   session's tool set to the two the definition names, yet `Write` was still
   denied because the allowlist omitted it. So availability and permission are
   separate gates and both must pass.
3. **`merge` therefore needs the runtime-side union** — nothing in the CLI folds
   the definition's declaration into the session's permissions. D3 stands as
   designed: the runtime parses the frontmatter and passes the union via
   `--allowedTools`.
4. **`dontAsk` denies and returns.** It does not prompt and does not hang; the
   process exits with a result the manager can report. D4 stands. (The default
   mode also denies rather than hangs under `-p`, but `dontAsk` makes the
   posture explicit and hands the model an unambiguous denial message.)

**Constraint this adds:** the runtime must NOT pass `--agent <name>` while it
composes. Doing so re-imposes the definition's list as an availability
intersection, which would silently defeat `overwrite` and drop any `merge` tool
the definition did not declare. The lane templates adopt the role as prose and
pass no `--agent` — that is now load-bearing, not incidental.

### D5: `mcpConfigs` stays mode-less (task 1.2)

The agent-definition schema has no field for MCP *servers* — a `tools:` entry
may name `mcp__server__tool`, but servers reach the session only through
`--mcp-config`. There is nothing on the definition side for `mcpConfigs` to
compose against, so `merge` and `overwrite` would again be one behavior wearing
two names — the original reason the field was dropped, which holds here even
though it did not hold for `toolsets`. **Only `spec.toolsets` regains a mode.**

Task 1.3 does not apply: the frontmatter does not govern the main session on its
own, so the change proceeds in full rather than narrowing to section 4.
