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
the hard ones. Both, on one install, is exactly what `Pipeline` → profile →
`runtimeRef` already expresses; the missing piece is a second runtime to point
at.

It is also the honest test of whether the work contract is vendor-neutral.
`runtime-claude` gets its agent loop, its tool execution, its permission model
and its session handles from claude-code. Ollama supplies none of those — it is
an inference endpoint. Everything above it has to be built, which is precisely
what makes it the implementation that finds out whether `runtimeContextId`,
`toolsMode`, `allowedTools` and the continuity reports describe a contract or
just describe claude-code.

## What Changes

- **A new module `runtime-ollama/`** — a Go implementation of the work contract
  in which the RUNTIME is the harness: it owns the agent loop, tool dispatch,
  the transcript, and the context handle. Ollama is called only to produce the
  next message.
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
  message transcript under its home volume, hands the manager an opaque
  `runtimeContextId`, declares `contextStorage: volume`, and reproduces the
  discipline the reference runtime pays for — a handle that resolves is
  continued, a handle that cannot be reached is re-checked before being believed
  (slow storage is an outage, not a loss), and a genuinely missing context FAILS
  the run with a readable reason rather than answering as a stranger wearing the
  conversation's name.
- **Chart values to declare it**: the parent chart gains an optional list of
  ADDITIONAL runtimes beside the `default` one, so an install can run both — the
  vendor runtime for hard lanes, this one for local lanes — selected per profile
  by `runtimeRef`. The chart does NOT deploy Ollama; it points at an endpoint
  the adopter already runs.
- **Docs**: a new `docs/runtimes.md` covering both shipped runtime images — what
  each supports, what each costs, how to choose, and how to write a third.

Not in scope: **BREAKING** nothing. No CRD field is added or changed, no manager
code changes — the work contract already carries `agent`, `allowedTools`,
`toolsMode`, `maxTurns`, `systemPrompt`, `runtimeContextId`, and accepts
`continuity` / `continuityReason` on report. Also out of scope: deploying Ollama
itself, GPU scheduling, model management, embeddings/RAG, and any per-profile
model field (the model is a property of the runtime, so a second model is a
second `AgentRuntime`).

## Capabilities

### New Capabilities

- `ollama-agent-runtime`: the local-model runtime image — the agent loop it
  owns, how it obtains and enforces tools (MCP plus native built-ins), how it
  stores and continues conversation context, how it reports runs, what it
  requires of the Ollama endpoint and the model, and how it is declared.

### Modified Capabilities

- `agent-runtime-ownership`: the parent chart may now render MORE than one
  `AgentRuntime` — the `default` one plus an operator-declared list — while the
  rule that no BUNDLE renders a runtime, a runtime ServiceAccount or a
  credential is unchanged. The current requirement's "exactly one `AgentRuntime`
  renders" scenario is what changes.
- `builtin-toolset-catalog`: the built-in tool names are RUNTIME-INTERPRETED,
  not manager-defined. With a second runtime implementing them natively, the
  catalog's requirement gains the rule that a runtime provides what it can and
  REPORTS what it cannot — an unimplemented built-in is visible in the run, not
  silently absent.

## Impact

- **New module**: `runtime-ollama/` (Go, own `go.mod`, `Dockerfile`, unit
  tests). Ninth module in the repository, second runtime image.
- **New dependency posture**: this module is NOT dependency-free. It takes
  `github.com/modelcontextprotocol/go-sdk` for the MCP client — the one part
  that is real protocol surface rather than plumbing. Consequence found while
  planning: that SDK requires **Go 1.25**, while every existing module targets
  1.23, so this module pins its own toolchain and builds in a `golang:1.25`
  container. The Ollama HTTP API is hand-rolled instead of imported: the
  `ollama/ollama` module requires Go 1.26 and drags gin, cobra, sqlite3 and
  bubbletea for what is two JSON structs.
- **Chart**: `chart/values.yaml` gains the additional-runtimes list;
  `chart/templates/runtime.yaml` (or a sibling) renders them. Existing installs
  render byte-identically when the list is empty.
- **Docs**: new `docs/runtimes.md`; `docs/contracts.md` work contract gains the
  second reference implementation; `README.md` install/kind text unchanged;
  `CLAUDE.md` gains the module in its map and the terminology it establishes.
- **Unchanged**: `api/v1alpha1/`, every CRD, `internal/` in its entirety,
  `runtime-claude/`. If this change needs a manager edit, the contract was not
  vendor-neutral and that finding is the more important outcome.
- **Operational reach**: none by default — nothing renders unless an operator
  declares a runtime in the new list.
