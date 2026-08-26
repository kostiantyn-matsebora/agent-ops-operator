## Why

Every conversation this operator runs today ends at a vendor API. `AgentRuntime`
was designed for exactly this — "adopters bring their own agent backend" is in
the CRD's own doc comment — but the claim has never been tested: there is one
runtime image, and it wraps one vendor's CLI. A contract with a single
implementation is a design intention, not a contract.

A local-model runtime is the case that proves it, and the case adopters ask for
first. An operator that watches a cluster sends its own alerts, logs and object
YAML to whoever answers; plenty of installs cannot do that at any price, and
plenty more would rather not for the routine lanes — summarise this alert,
explain this event, draft this reply — while keeping the expensive vendor for
the hard ones. Both, on one install, is exactly what `Pipeline.spec.runtimeRef`
already expresses; the missing piece is a second runtime to point at.

It is also the honest test of whether the work contract is vendor-neutral.
`runtime-claude` gets its agent loop, its tool execution, its permission model
and its context handles from claude-code. Ollama supplies none of those — it is
an inference endpoint. Everything above it has to be built, which is precisely
what makes it the implementation that finds out whether `runtimeContextId`,
`toolsMode`, `allowedTools` and the continuity reports describe a contract or
just describe claude-code.

## What Changes

- **A new component `runtimes/ollama/`** — published as
  `agentops-runtime-ollama`, the name `.github/components.sh` derives from the
  path — a Go implementation of the work contract in which the RUNTIME is the
  harness: it owns the agent loop, tool dispatch, the transcript, and the
  context handle. Ollama is called only to produce the next message.
- **The agent loop**: chat → tool calls → execute → append → repeat, bounded by
  the unit's `maxTurns`, streaming a readable transcript to stdout the way
  `runtime-claude` streams claude's `stream-json`.
- **Tools, two sources, one allowlist**:
  - **MCP servers** from the compiled `mcp.json` the manager already mounts,
    connected over the official Go MCP SDK and advertised to the model as
    `mcp__<server>__<tool>`.
  - **Built-in tools implemented natively** — `Read`, `Grep`, `Glob`, `Bash`,
    `Edit`, `Write` — the exact vocabulary the chart's `agentops-observe` /
    `-shell` / `-edit` toolsets name, so those bindings mean the same thing on
    this runtime as on the reference one.
  - Composition is the contract's: the unit's `allowedTools` is one half, the
    agent definition's `tools:` frontmatter (read from the checkout) is the
    other, and `toolsMode` decides. **An empty allowlist stays empty** — no tool
    is offered to the model that the allowlist does not carry, and a pattern
    naming something this runtime cannot provide is REPORTED, never faked.
- **Context continuity implemented, not declined**: the runtime persists the
  message transcript under `$HOME` — which is the CONTEXT volume,
  `/data/context`, or pod-local storage under `context-sync` — hands the manager
  an opaque `runtimeContextId`, declares `contextStorage: volume`, and
  reproduces the discipline the reference runtime pays for — a handle that
  resolves is continued, a handle that cannot be reached is re-checked before
  being believed (slow storage is an outage, not a loss), and a genuinely
  missing context FAILS the run with a readable reason rather than answering as
  a stranger wearing the conversation's name.
- **A bundle to declare it**: `chart/charts/ollama/`, OFF by default, the exact
  shape of `chart/charts/claude/` — one `AgentRuntime` rendered through the
  parent's `agentops.renderRuntime` over `global.agentops.runtimeDefaults`,
  carrying only what names this vendor: the image, `OLLAMA_URL` /
  `OLLAMA_MODEL` and the tuning knobs as `env`, its own `contextSync` paths, and
  an optional credential. The parent chart changes only in `Chart.yaml`
  (dependency + condition). The bundle does NOT deploy Ollama; it points at an
  endpoint the adopter already runs. A route selects it with
  `pipelines[].runtimeRef: ollama`.
- **Docs**: a new `docs/runtimes/ollama.md`, the first page of a **Runtimes**
  nav group — a runtime is not an integration by the site's own rule, so it
  gets a page kind of its own — with what the runtime executes, what it needs,
  where its context lives, what it costs, and how to choose between the two
  images.
  The landing page and the README gain the Ollama chip, the amended "Runs
  Claude Code" claim and the local-model row; the installation page's bundle
  list and the site's navigation gain the entry.

Not in scope: **BREAKING** nothing. No CRD field is added or changed, no manager
code changes — the work contract already carries `agent`, `allowedTools`,
`toolsMode`, `maxTurns`, `systemPrompt`, `runtimeContextId`, and accepts
`continuity` / `continuityReason` on report. Also out of scope: deploying Ollama
itself, GPU scheduling, model management, embeddings/RAG, and any per-profile or
per-pipeline model field (the model is a property of the runtime, so a second
model is a second `AgentRuntime` — a `runtimes:` entry naming the same image).

## Capabilities

### New Capabilities

- `ollama-agent-runtime`: the local-model runtime image — the agent loop it
  owns, how it obtains and enforces tools (MCP plus native built-ins), how it
  stores and continues conversation context, how it reports runs, what it
  requires of the Ollama endpoint and the model, and how its bundle declares it.

### Modified Capabilities

- `builtin-toolset-catalog`: the built-in tool names are RUNTIME-INTERPRETED,
  not manager-defined. With a second runtime implementing them natively, the
  catalog's requirement gains the rule that a runtime provides what it can and
  REPORTS what it cannot — an unimplemented built-in is visible in the run, not
  silently absent.

`agent-runtime-ownership` is NOT modified. It already says a bundle may ship a
vendor's runtime and inherits the parent's defaults; this change is the second
instance of that rule, not a change to it.

## Impact

- **New component**: `runtimes/ollama/` (Go, own `go.mod`, own `Dockerfile`,
  unit tests). The second runtime image; count the rest from
  `.github/components.sh modules`, never from prose.
- **New dependency posture**: this module is NOT dependency-free. It takes
  `github.com/modelcontextprotocol/go-sdk` for the MCP client — the one part
  that is real protocol surface rather than plumbing. Consequence found while
  planning: that SDK requires **Go 1.25**, while every existing module targets
  1.23, so this module pins its own toolchain, carries its own Dockerfile
  (`golang:1.25` build stage — the shared `.github/docker/go-module.Dockerfile`
  is 1.23 and distroless, and this image needs git and a shell anyway), and is
  built locally in a SECOND persistent container beside `agentops-go`. The
  Ollama HTTP API is hand-rolled instead of imported: the `ollama/ollama` module
  requires Go 1.26 and drags gin, cobra, sqlite3 and bubbletea for what is two
  JSON structs.
- **Chart**: a new subchart `chart/charts/ollama/`, one dependency line in
  `chart/Chart.yaml`, `ollama.enabled: false`. Existing installs render
  byte-identically. The parent's `runtimes:` block and its defaults are
  untouched.
- **Docs**: new `docs/runtimes/ollama.md`; `docs/contracts.md` work contract gains the
  second reference implementation; `docs/installation.md` lists the bundle;
  `docs/_data/nav.yml` gains the page; `docs/CHANGELOG.md` records the chart
  minor; `README.md` unchanged. `.claude/rules/structure.md` gains the component
  and `.claude/rules/build-test.md` the 1.25 container.
- **Unchanged**: `api/v1alpha1/`, every CRD, `platform/manager/internal/` in
  its entirety, `runtimes/claude/`. If this change needs a manager edit, the
  contract was not vendor-neutral and that finding is the more important
  outcome.
- **Operational reach**: none by default — nothing renders unless an operator
  enables the bundle.
