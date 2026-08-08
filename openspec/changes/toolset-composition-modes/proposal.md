# toolset-composition-modes

## Why

`ToolingBinding.mode` was removed in `capabilities-are-wiring` on the reasoning that with capabilities gone from `AgentProfile` there was nothing left to compose against. That was wrong. The thing a mode composes against is the **agent definition's own declared tools** — the `tools:` frontmatter of `.claude/agents/<agent>.md` in the profile's repository. `merge` means "add the wiring's toolsets to what the agent declares"; `overwrite` means "replace them". That distinction is independent of anything the profile carries, and removing the field removed the vocabulary for it.

It also turns out the frontmatter's `tools:` is inert today, so the mode never worked even before it was deleted. The built-in lane templates tell the model to *read* `.claude/agents/<name>.md` and adopt the role as prose; nothing invokes it as a subagent, and per Claude Code's documented model a frontmatter `tools:` list scopes a subagent while the main session is governed solely by `--allowedTools`. The agent's own declaration of what it may use is currently read as narrative and enforced not at all.

Separately, `runtime-claude` substitutes `--allowedTools Read` whenever the work unit carries none — inventing a capability nobody declared, which is the implicit behavior this project has been removing everywhere else.

## What Changes

- **`ToolingBinding.mode` returns** (`merge` | `overwrite`, default `merge`) on `Pipeline.spec.toolsets` and `spec.mcpConfigs`, with its real meaning: how the wiring's refs compose with what the **agent definition** declares.
- **The WorkUnit carries the mode**, so the runtime — the only component that can see the repository — performs the composition.
- **`runtime-claude` reads the agent definition's frontmatter** and composes: `overwrite` passes the wiring's tools alone; `merge` passes the union of the agent's declared tools and the wiring's.
- **The `|| 'Read'` fallback is removed.** An empty allowlist is passed through as empty, together with `--permission-mode dontAsk` so unlisted tools are denied rather than triggering permission prompts that would hang a headless run. Declaring nothing means having nothing — it must not silently mean `Read`.
- **BREAKING (runtime image)**: new `agentops-runtime-claude` tag; older runtimes ignore the mode and keep today's behavior.

## Capabilities

### New Capabilities

- `agent-definition-tools`: the contract between the runtime and the agent definition — that a declared `tools:` frontmatter is honoured, how the work unit's mode composes it with the wiring's toolsets, and that an empty result denies rather than defaults.

### Modified Capabilities

- `mcp-toolset-model`: bindings carry a mode again, composing against the agent definition rather than against any profile field; the allowlist the manager computes becomes the wiring's contribution, not the final answer.
- `pipeline-model`: `spec.toolsets` and `spec.mcpConfigs` each carry an independent `mode`.

## Impact

- **API**: `ToolingBinding.Mode` restored (optional, defaulted); deepcopy + CRD regen. Additive — existing Pipelines default to `merge`.
- **Dispatch**: `WorkUnit` gains the mode; `internal/dispatch` stops presenting its output as the final allowlist.
- **Runtime**: `runtime-claude/runtime.js` parses the agent definition's frontmatter, composes per mode, drops the invented default, adds `--permission-mode dontAsk`. New image tag.
- **Docs**: README capabilities section, CLAUDE.md, the work-contract description of `allowedTools`.
- **Gated on evidence**: the interaction between `--allowedTools` and frontmatter `tools:` is *inferred from Claude Code's architecture, not documented*. Task 1.1 verifies it against a real run before any of this is encoded — the wrong assumption here produces agents whose declared tools are silently ignored, which is the bug being fixed.
- **Depends on**: `pipeline-addressed-conversations` landing first (it removes the baseline this would otherwise have to reason about).
