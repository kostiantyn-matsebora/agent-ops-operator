## Why

The manager's lane prompts tell the model where to find its own role:
`task.md` and `investigate.md` render *"if `.claude/agents/{{AGENT_NAME}}.md`
exists in the checkout, read it and adopt that agent's role"*. That sentence
was written by looking at one vendor, and `copilot-runtime` is what made it
visibly wrong:

- **It names a vendor path in a vendor-neutral prompt.** The copilot runtime
  reads `.github/agents/<agent>.agent.md`; the line sends it looking in a
  directory its vendor never uses.
- **An empty `spec.agent` renders `.claude/agents/.md`.** The demo profile
  names no agent and has no repository, and every Copilot answer on the local
  install opened with an `agent role file not found` title — the model chasing
  a file the prompt told it about, then reporting the miss it was told to
  mention only when an agent was named.
- **It delegates a deterministic step to the model's judgement.** Whether the
  definition is adopted depends on the model choosing to `glob` for it.

The chain is already complete without the sentence: a `Pipeline` names a
profile, `AgentProfile.spec.agent` names a definition, and the RUNTIME — the
only component holding the checkout — already opens that file to read its
`tools:` frontmatter. The body of the same file is the role. Nothing in that
chain needs the model to be told a filename.

## What Changes

- **The runtime adopts the definition, not the model.** Each runtime reads the
  definition's BODY (everything below the frontmatter) beside the `tools:` it
  already reads, and appends it to the system message — `runtime-claude`
  through `--append-system-prompt`, `runtime-copilot` through
  `systemMessage.append`, `runtime-ollama` in its own system assembly. The
  unit's inline `systemPrompt` and the definition's body compose: the
  definition first, the profile's inline role after it, on every run
  including resumes.
- **The prompt names no file.** The "Adopt the agent role" section leaves
  `task.md` and `investigate.md`. What stays is the fallback posture — "act as
  a competent, cautious SRE/platform advisor within your tools" — as ordinary
  system text, with nothing to look for and nothing to report missing.
- **An empty `spec.agent` produces nothing.** No definition is looked up, no
  path is rendered, no mention is made.
- **Reference docs and the site** say where a role comes from in one place:
  the profile names it, the runtime reads it, the prompt is silent.

Not in scope: no CRD field changes; `AgentProfile.spec.agent` keeps selecting
which definition a repository with several provides; the frontmatter
composition rule is untouched.

## Capabilities

### New Capabilities
- (none)

### Modified Capabilities
- `agent-definition-tools`: gains the requirement that the definition's body
  is adopted by the RUNTIME into the system message, that the manager's
  prompts name no definition file, and that an unnamed agent produces no
  lookup and no mention.

## Impact

- **Manager**: `platform/manager/internal/dispatch/templates/task.md`,
  `investigate.md`; a test pinning that no rendered prompt contains
  `agents/`. New `manager` image tag, `appVersion` bump.
- **Runtimes**: `runtimes/claude/tools.js` + `runtime.js`,
  `runtimes/copilot/tools.js` + `runtime.js`, `runtimes/ollama/tools.go` +
  `agent.go` — each reads the body and appends it; unit tests in all three.
  Three runtime image tags; the chart's `claude`, `copilot` and `ollama`
  bundles move to them. Chart minor. **Depends on `copilot-runtime` landing
  first**: `runtimes/copilot/`, its bundle and `docs/runtimes/copilot.md` are
  that change's files, and this change edits them. Applied before it lands,
  the copilot items are deferred and `copilot-runtime` carries them.
- **Reference docs**: `docs/contracts.md` (the work contract's agent
  obligation: the runtime adopts the definition; the prompt is silent),
  `docs/concepts.md` (§ agent role file, § `toolsets.mode`), `docs/CHANGELOG.md`.
- **Adopter site**: `docs/guides/agent-runtime.md` (a runtime's obligations
  gain "append the definition's body"), `docs/guides/agent-profile.md` or the
  profile page that explains `spec.agent`, `docs/runtimes/copilot.md` and
  `docs/runtimes/ollama.md` ("The tools it gives" → also the role),
  `docs/claude.md`. The landing page, introduction, getting-started and
  installation say nothing about role files today and stay as they are.
- **Context rules**: `.claude/rules/wiring.md` (the definition paragraph),
  `.claude/rules/structure.md` (`internal/dispatch/` row).
- **Unchanged**: `api/v1alpha1/`, every CRD, `MCPToolset`, the chart's
  toolset catalog.
