## [13.3.0] — 2026-08-28

**A third runtime: `agentops-runtime-copilot` 0.1.0, shipped as the `copilot`
bundle.** GitHub Copilot driven through its SDK, in process.

The first vendor to own its own tool vocabulary, its own agent-definition
format and its own session store. Every one of those is translated inside
the runtime.

A Pipeline binds the same toolsets it binds for claude. No manager, CRD or
toolset change was needed.

### Added

- `chart/charts/copilot/` — OFF by default. `copilot.enabled: true` with a
  GitHub token (`copilot.credentialsSecret.token`, or a Secret you manage)
  renders one `AgentRuntime` named `copilot` through the parent's shared
  renderer, inheriting `global.agentops.runtimeDefaults`. A route selects it
  with `pipelines[].runtimeRef: copilot`. `copilot.model` and
  `copilot.maxAiCredits` are optional.
- The runtime translates the composed allowlist into Copilot's two layers:
  `availableTools` (`Read` → `view`, `Bash` → `bash`, `mcp__s__t` →
  `mcp:s-t`) and a permission callback that decides every call. An unmapped
  pattern is withheld and logged. `mcp__<server>__*` is refused rather than
  widened to every server. `Bash(kubectl:*)` is enforced per invocation. An
  empty allowlist stays empty — Copilot's "no `tools:` means everything" never
  applies — and a denial is fed back to the model, never a hang.
- It reads `.github/agents/<agent>.agent.md`, Copilot's location, with the same
  frontmatter shapes and `merge` / `overwrite` composition as the reference
  runtime. Bound MCP servers reach Copilot from the same `mcp.json`, with
  secret placeholders resolved in the pod.
- The context id is minted by the runtime, so `runtimeContextId` is a handle
  it chose. State lives under `$HOME/.copilot/session-state/<id>/`, declared to
  `context-sync` by the bundle. A resume whose state is gone re-checks, then
  FAILS with `continuity: unavailable` and a readable reason.
- `docs/runtimes/copilot.md`, and the Copilot chip on the landing page.
- CI runs `node --test` for the Node runtimes (`runtimes/claude`,
  `runtimes/copilot`) when their directories change. Neither suite ran in CI
  before.

**Nothing changes for an install that does not enable the bundle.** The
rendered manifest is identical.
