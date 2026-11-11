## Why

The project ships four runtimes, and three of them lock an adopter to one
vendor's model access: `claude` and `claude-cli` to Anthropic, `copilot` to
GitHub. `ollama` is the only model-agnostic one, and its harness is our own.

pi is a minimal coding harness that talks to many model
providers and embeds through an SDK. A route that runs on it gets a
model-agnostic runtime without the project owning the agent loop.

The same Pipelines, toolsets and channels should run on it by changing one
field, `runtimeRef`. That is the claim every other runtime already makes.

## What Changes

- Add `runtimes/pi/`, a Node runtime driving the pi SDK in-process.
  Modelled on `runtimes/copilot`: same work contract, same layout, own image.
  - **Tools.** Built-ins (`Read`, `Write`, `Edit`, `Bash`, `Grep`, `Glob`)
    are enabled only when the composed allowlist names them. No per-command
    narrowing: `Bash(kubectl:*)` grants nothing and never widens, as in
    `runtimes/ollama`. An empty allowlist stays empty.
  - **MCP.** Pi core has no MCP, so the image ships a small extension of our
    own. It reads `MCP_CONFIG`, connects only to the servers that file
    lists, and registers their tools as `mcp__<server>__<tool>`, filtered by
    the composed allowlist. The runtime starts no server of its own choosing.
    The egress proxy walls the MCP traffic exactly as for the other runtimes.
  - **Agent definition.** The runtime reads `.claude/agents/<agent>.md`,
    appends its body to the system prompt, then the work unit's
    `systemPrompt`, and merges its `tools:` per `toolsMode`. An absent or
    unreadable definition contributes nothing. This is the behaviour
    `agent-definition-adopted-by-runtime` specifies for the other runtimes.
  - **Context.** The runtime mints `runtimeContextId` and maps it to a pi
    session whose state lives under `$HOME`.
  - **Telemetry.** `turns[]` and `toolCalls[]` come from pi's own events,
    bounded, and omitted where unknown.
  - **Models.** Provider and credential arrive as env from a Secret, never
    through a new CRD field.
- Add `chart/charts/pi/`, an optional bundle, OFF by default, rendering one
  `AgentRuntime` through the parent's shared renderer.
- Add `docs/runtimes/pi.md` and its nav entry, claiming specification 2.0.2.
- **Spec edit, explicitly approved by the user for this change.** Add pi's
  definition path to `RT-3.4` in `docs/contracts/runtime.md`. A PATCH
  clarification, 2.0.1 to 2.0.2, changing no obligation.
- Bring every shipped runtime page's claimed version to 2.0.2 and walk the
  35-item conformance checklist for pi.
- Record six spikes against pi's SDK as design tasks, before any code
  depends on them.
- **BREAKING**: none. The bundle is off by default and no CRD field changes.

## Capabilities

### New Capabilities
- `pi-agent-runtime`: the fifth reference runtime, driving pi through its
  SDK. Its obligations under the work contract, tool enablement from the
  composed allowlist, the in-image MCP extension, the agent definition as
  system prompt, the runtime-minted context handle, telemetry, and the
  bundle that declares it.

### Modified Capabilities
- `agent-definition-tools`: the definition path stays the runtime's own
  fact, and `runtime-pi` joins `runtime-claude` and `runtime-ollama` in
  reading `.claude/agents/<agent>.md`.
- `runtime-contract`: the "specification change carries conformance"
  requirement names the shipped runtimes, and the list grows to include
  `runtimes/pi`. A new shipped runtime claims the current version and is
  walked against the checklist.

## Impact

**Code**:
- `runtimes/pi/` — new component. `runtime.js`, `vocabulary.js`, `tools.js`,
  `mcp.js` (the extension), `continuity.js`, `report.js`, their tests, a
  `package.json` with an exact pin and a lockfile, and a `Dockerfile`
  following copilot's CVE-pin pattern.
- `chart/charts/pi/` and the parent's runtime-entry helper beside
  `agentops.copilotRuntimeEntry`. Chart render tests.
- `.github/components.sh` discovers the directory. The published image is
  `agentops-runtime-pi`, released by tag `runtime-pi-v<semver>`.
- `.github/scripts/docs_diagrams.py` — a `RUNTIME_DIAGRAMS` entry.
- `platform/manager/test/conformance/` and the system pack — a pi lane.

**Reference docs** — made untrue by this change:
- `docs/contracts/runtime.md` — `RT-3.4`, the version line, the changelog
  row, and the conformance wording.
- `docs/concepts.md` — the runtime list and the pinned versions.
- `docs/configuration.md` — the `pi` bundle's values.
- `docs/CHANGELOG.md` — an Added entry, no upgrade steps.
- `.claude/rules/runtime-spec.md` and `.claude/rules/structure.md` — the
  shipped-runtime lists and the "three shipped runtimes" wording.

**Adopter site** — made untrue by this change:
- `docs/runtimes/pi.md` — new page, four questions in the runtime-page
  order, generated `renders` block.
- `docs/_data/nav.yml` — one entry in the Runtimes group.
- The four existing runtime pages — the claimed specification version.
- `docs/introduction.md` and `docs/index.md` — where they list the shipped
  runtimes.
- `docs/installation.md` — where it names the optional runtime bundles.
- `docs/guides/agent-runtime.md` — if it names the shipped set.
- `README.md` — the runtime list only, within its line budget.

**Not impacted**: every CRD, the manager's dispatch, the work-unit and
report shapes, and `platform/egress-proxy`. The runtime is a new client of
an unchanged contract.
