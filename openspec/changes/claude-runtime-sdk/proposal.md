## Why

`runtime-claude` drives claude-code by spawning the CLI once per work unit.
That was the right first implementation, and three things have made it the
wrong one now.

**It re-establishes everything, every unit.** A runtime pod handles more than
one work unit in its life — queued inputs, a fast back-and-forth, a burst on
one conversation. Each of those spawns `claude` afresh with `--resume`.

That replays the conversation transcript off disk and re-launches every bound
MCP server. The cost grows with the conversation, so it is worst exactly where
the conversation is most valuable.

Nothing about the current design lets a second unit land in a session that is
already open.

**And the TTL is the adopter's dial, not ours.** The chart ships
`runtimeIdleTtlMinutes: 1`. But `AgentRuntime.spec.idleTtlMinutes` is
per-runtime, and a pod is cheap.

An adopter may well run ten minutes, or hours, for an agent with expensive
startup. At that end of the range a live session is not an optimisation — it
is the normal path, and cold resume is the exception.

**And one image cannot be light for everyone.** The CLI needs its own global
`@anthropic-ai/claude-code` install plus the installer script that unpacks its
native binary.

The SDK needs neither. It is a library dependency, invoked in-process, not a
second binary to spawn.

An image carrying both pays the CLI's weight whether or not a given pod ever
spawns it. No adopter actually gets the lighter option. Only shipping two
images does.

Separately, the CLI's permission surface is a set of flags with rules we have
had to reverse-engineer and document:

- `--allowedTools` is the sole authority.
- Never pass `--agent`.
- `--permission-mode dontAsk`, because a prompt in a pod is a hang.

The SDK exposes the same decisions as a `canUseTool` callback — the same shape
`runtime-copilot` needs. The two reference runtimes end up with ONE permission
story instead of two.

## What Changes

- **`runtime-claude` becomes an SDK-only runtime, and the CLI becomes its own
  runtime.** `runtimes/claude/` keeps its path, its image name
  (`agentops-runtime-claude`), and its place as the chart's default-eligible
  bundle — but it now drives `@anthropic-ai/claude-agent-sdk` in-process and
  drops the CLI install from its Dockerfile entirely. A new `runtimes/claude-cli/`
  component carries today's spawn-per-unit logic, moved verbatim, as its own
  image (`agentops-runtime-claude-cli`) and its own chart bundle
  (`chart/charts/claude-cli/`), off by default — the same pattern
  `runtimes/ollama` and `runtimes/copilot` already use.
- **No env var, no in-image switch.** Choosing CLI or SDK means choosing which
  `AgentRuntime` a Pipeline's `runtimeRef` points at, or which bundle a release
  enables. That replaces the single-image, `RUNTIME_CLAUDE_DRIVER`-selected
  design this change originally shipped as PR #424 — closed, superseded by
  this redesign, before merge.
- **A session is held open across work units within one pod**, in
  `runtimes/claude` only, via the SDK's streaming-input mode: the runtime
  pushes the next unit's prompt into the live session rather than starting a
  new process. Re-spawn and transcript re-replay disappear for every unit
  after the first, and bound MCP servers stay connected.
- **The live session is a CACHE, never the source of truth.** A pod that has
  just started, was evicted, was rescheduled, or lost its process resumes
  from `runtimeContextId` exactly as today. Nothing the manager sends or
  receives changes, and the work contract is untouched.
- **Permissions move from flags to `canUseTool`** in `runtimes/claude`,
  evaluated against the composed allowlist per invocation, with the
  empty-allowlist rule and the no-prompting rule preserved. The callback
  answers EVERY unresolved call and denies by default — `dontAsk` cannot be
  the backstop here, because that mode skips the callback entirely.
  `runtimes/claude-cli` keeps `--allowedTools`/`--permission-mode dontAsk`,
  unchanged.
- **The transcript keeps its current shape** in both components: `[init]` /
  `[claude]` / `[tool]` / `=== RESULT ===`, because those lines are what a
  human reads in VictoriaLogs.
- **`runtimes/claude` gets handling `runtimes/claude-cli` never needed**: MCP
  servers alive for hours (credential expiry, leaked children) and a process
  that dies mid-session, which must fall back to disk resume without the
  manager seeing anything different.

Not in scope: no CRD field, no manager change, no change to the credential or
to which vendor runs. The chart DOES change — see Impact.

`runtime-copilot` is a separate change. The only thing shared is the
permission-callback shape.

## Capabilities

### New Capabilities
- `runtime-session-reuse`: that a runtime MAY hold a vendor session open
  across work units within one pod, what that obliges it to preserve (the
  recorded context handle stays authoritative, a cold pod is indistinguishable
  to the manager), and what a long-lived session must handle that a
  short-lived one never did. `runtimes/claude` takes this freedom
  unconditionally. `runtimes/claude-cli` does not take it, the same as any
  other per-unit runtime.

### Modified Capabilities
- `agent-definition-tools`: the composed allowlist is currently expressed as
  something the runtime PASSES to its vendor. It SHALL be expressible as a
  per-invocation DECISION as well, so a runtime whose vendor offers a
  permission callback satisfies the same rules — empty means empty, and
  nothing may block on a prompt — without a flag that happens to spell them.
  `runtimes/claude` takes the callback form. `runtimes/claude-cli` keeps the
  flag form, already compliant, and needs no change to satisfy this.

## Impact

- **Changed**: `runtimes/claude/`.
  - `runtime.js` rewritten around `query()`. `tools.js` composition logic
    unchanged.
  - `Dockerfile` drops the global `@anthropic-ai/claude-code` install and
    adds `@anthropic-ai/claude-agent-sdk`.
- **New**: `runtimes/claude-cli/` — today's `runtime.js`, `tools.js`,
  `spin.js`, `report.js` and their tests, moved verbatim into a new directory
  with its own `Dockerfile` (today's `runtimes/claude/Dockerfile`, unchanged).
  `chart/charts/claude-cli/` — a new bundle (`Chart.yaml`, `values.yaml`,
  `templates/runtime.yaml`), off by default, mirroring
  `chart/charts/ollama/`.
- **Two image tags**: `agentops-runtime-claude` (new minor — SDK-only,
  lighter) and `agentops-runtime-claude-cli` (new component, starts fresh).
- **Published spec amendment**: `docs/contracts/runtime.md`'s RT-3 gains a
  PATCH clarification, scoped to `runtimes/claude` — the only shipped runtime
  taking the callback form. `runtimes/claude-cli` needs no clarification,
  since the flag form already satisfies RT-3 as written.
  - `docs/claude.md`, `docs/runtimes/ollama.md` and `docs/runtimes/copilot.md`
    move their claimed version to match, per `runtime-spec.md`'s rule that
    every shipped runtime's version moves together. `docs/runtimes/claude-cli.md`
    is a new page for the new component.
  - Session reuse itself still does NOT publish there — see design.md's
    Non-Goals.
- **Dependencies**: `@anthropic-ai/claude-agent-sdk` (bundles the Claude Code
  binary, needs Node 18+ — the image is Node 22). Auth is unchanged for both
  components — the bundled binary reads `CLAUDE_CODE_OAUTH_TOKEN`, which is
  what the chart already injects.
- **Unchanged**: `api/v1alpha1/`, `internal/`, the work contract itself, every
  adapter module.
