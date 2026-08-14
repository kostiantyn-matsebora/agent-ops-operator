## Why

agent-ops claims runtimes are pluggable — `AgentRuntime.spec.image` is the whole
extension point, and the work contract is written so an adopter can bring any
backend. One image has ever implemented it. Until a second vendor runs the same
contract, "bring your own runtime" is an assertion, not a fact: nothing has ever
forced the vendor-neutral vocabulary (`runtimeContextId`, `toolsMode`, an
allowlist that means what it says) to survive contact with a backend whose nouns
are different.

GitHub Copilot is the concrete case worth having: a second vendor with its own
credential, its own tool vocabulary, its own idea of where an agent definition
lives — and, through the Copilot SDK, a session id the CALLER supplies, which is
the first backend to exercise `runtimeContextId` as a genuinely opaque handle
rather than one claude-code happened to mint.

## What Changes

- **New module `runtime-copilot/`** — a second reference `AgentRuntime` image
  (Node + `@github/copilot-sdk`), self-contained and dependency-free like every
  other module here. It implements the same four-step work contract:
  long-poll `/work`, execute against the `/data/workspace` checkout, stream a
  transcript to stdout, `POST /work/done`, exit after the idle TTL.
- **Vendor vocabulary is translated inside the runtime**, not pushed up into the
  CRDs. `MCPToolset` patterns stay opaque and claude-flavoured cluster-wide
  (`Read`, `Bash(kubectl:*)`, `mcp__kubernetes__pods_list`); the copilot runtime
  maps them to Copilot's own (`view`, `shell(kubectl:*)`, `kubernetes(pods_list)`)
  at the point of use. One toolset vocabulary, translated once per vendor — the
  chart's `global.builtinToolsets` is untouched and a Pipeline binds the same
  toolsets whichever runtime serves it.
- **An unmapped pattern is DENIED, never passed raw and never dropped silently.**
  A pattern the mapper does not understand is logged and withheld: passing it
  through would hand Copilot a string it reads as some other tool, and dropping
  it quietly would widen or narrow a route without saying so.
- **Copilot's agent-definition default is inverted at the boundary.** Copilot
  reads `.github/agents/<agent>.agent.md`, where an omitted `tools:` means ALL
  tools; agent-ops means "declares nothing". The runtime SHALL disable default
  tooling and pass only the composed allowlist, so an empty composition stays
  empty here exactly as it is under claude.
- **The chart gains opt-in ADDITIONAL runtimes** — a values-driven list
  rendering more `AgentRuntime` CRs beside `default`, each with its own image and
  credential Secret. The substrate stays parent-owned (one runtime
  ServiceAccount, one home volume, one RBAC mode); a second runtime is a second
  VENDOR, not a second trust level.
- **Docs**: the work contract page gains a second reference implementation and
  the per-vendor obligations a second implementation makes visible.

Not in scope: no CRD field is added or changed, no manager behaviour changes, and
no Pipeline, profile or toolset needs editing to use a copilot runtime — a
profile's `runtimeRef` is the entire switch.

## Capabilities

### New Capabilities
- `copilot-agent-runtime`: the second reference runtime — its obligations under
  the work contract, how a Copilot-supplied session id carries `runtimeContextId`,
  the tool-vocabulary translation and its deny-on-unmapped rule, MCP config
  translation, and the continuity behaviour when Copilot's session state is gone.

### Modified Capabilities
- `agent-runtime-ownership`: the parent chart currently renders EXACTLY ONE
  `AgentRuntime`. It SHALL be able to render additional named runtimes from
  values while still owning all of them — one runtime ServiceAccount and one
  RBAC mode per release regardless of how many vendors are installed.
- `agent-definition-tools`: the definition's location and frontmatter default are
  currently stated as claude-code's (`.claude/agents/<agent>.md`). They SHALL be
  per-runtime facts, with one rule binding every runtime: an absent or
  unparseable declaration contributes NOTHING, and no vendor's "omitted means
  everything" default may leak through.

## Impact

- **New**: `runtime-copilot/` (module + Dockerfile + tests), a
  `kmatsebora/agentops-runtime-copilot` image.
- **Changed**: `chart/templates/runtime.yaml` and `chart/values.yaml` (an
  additional-runtimes list), `chart/CHANGELOG.md`, `docs/contracts.md`,
  `docs/concepts.md`, `README.md` (one line), `CLAUDE.md` (module map).
- **Dependencies**: `@github/copilot-sdk` (bundles the Copilot CLI; Node 20+).
  A GitHub credential with Copilot access, per runtime, as
  `COPILOT_GITHUB_TOKEN` — projected from a Secret, never read by the manager.
- **Unchanged**: `api/v1alpha1/` (no CRD change), `internal/` (no manager
  change), `runtime-claude/`, every adapter module.
