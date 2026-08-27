## Why

agent-ops now ships two runtimes — `runtimes/claude/` wrapping a vendor CLI and
`runtimes/ollama/` owning the whole harness over an inference endpoint — and
the work contract survived both. What neither tests is the case a vendor SDK
presents: an agent loop the vendor owns, a TOOL VOCABULARY the vendor owns, an
agent-definition FORMAT the vendor owns, and a session id the CALLER supplies.
claude-code shares agent-ops' tool names by construction, and ollama needed no
translation because it implemented the built-ins itself. So "MCPToolset
patterns are opaque and translated at the boundary" has never actually been
exercised — one vocabulary has served every runtime.

GitHub Copilot is the concrete case: its SDK names tools `view`/`shell`/
`mcp:<server>-<tool>`, reads agents from `.github/agents/<agent>.agent.md`
where an omitted `tools:` means EVERY tool, and lets the caller choose the
session id — the first backend where `runtimeContextId` is a handle the runtime
mints rather than one scraped back out of a vendor's stdout. A third runtime
that disagrees with the first two on all of that is what settles whether the
vocabulary rules are a contract or a description of claude-code.

## What Changes

- **A new component `runtimes/copilot/`** — published as
  `agentops-runtime-copilot`, the name `.github/components.sh` derives from the
  path — Node plus `@github/copilot-sdk`, self-contained, no dependency on any
  other module here. It implements the work contract unchanged: long-poll
  `/work`, execute against the `/data/workspace` checkout, stream a transcript
  to stdout, `POST /work/done` with `continuity`/`continuityReason`, exit after
  the idle TTL.
- **Vendor vocabulary is translated inside the runtime**, never pushed up into
  the CRDs. `MCPToolset` patterns stay opaque and claude-flavoured cluster-wide
  (`Read`, `Bash(kubectl:*)`, `mcp__kubernetes__pods_list`); the copilot
  runtime maps them at the point of use, across Copilot's TWO layers —
  availability (`availableTools`) and per-invocation permission
  (`onPermissionRequest`). The chart's `global.builtinToolsets` is untouched and
  a Pipeline binds the same toolsets whichever runtime serves it.
- **An unmapped pattern is DENIED, never passed raw and never dropped silently.**
  Logged and withheld — the same "provide what you can, REPORT what you cannot"
  rule `builtin-toolset-catalog` gained with ollama. `mcp__<server>__*` is
  refused rather than widened to Copilot's all-servers `mcp:*`.
- **A narrowing shell pattern is HONOURED here**, where ollama grants nothing on
  it: `Bash(kubectl:*)` becomes `shell` available plus a permission callback
  that approves only `kubectl …`. Each runtime states what it can enforce.
- **Copilot's agent-definition default is inverted at the boundary.** The
  runtime reads `.github/agents/<agent>.agent.md`, composes with `toolsMode`
  exactly as `runtimes/claude/tools.js` does, and ALWAYS passes an explicit
  `availableTools` — `[]` when the composition produced nothing — so the
  vendor's "omitted means everything" never applies.
- **Context continuity implemented, not declined**: the runtime mints the
  session id, keeps state where the SDK does — `$HOME/.copilot/session-state/`
  under `/data/context` — declares `contextStorage: volume`, and reproduces the
  gone-versus-slow discipline both existing runtimes pay for: re-check before
  believing a miss, unreadable is not absent, confirmed absence FAILS the run
  with `continuity: unavailable` and a readable reason.
- **A bundle to declare it**: `chart/charts/copilot/`, OFF by default, the exact
  shape of `chart/charts/ollama/` — one `AgentRuntime` rendered through the
  parent's `agentops.renderRuntime` over `global.agentops.runtimeDefaults`,
  carrying only what names this vendor: the image, its `credentialsSecret`
  (`COPILOT_GITHUB_TOKEN`), its `contextSync.paths`, an optional model. The
  parent changes in `Chart.yaml` (dependency + condition), `values.yaml` (the
  documented section), and the `agentops.declaredRuntimes` bundle list — which
  is hand-listed and FAILED ollama's first render when missed. A route selects
  it with `pipelines[].runtimeRef: copilot`.
- **Docs**: `docs/runtimes/copilot.md` in the Runtimes nav group, the work
  contract's third implementation and the per-vendor obligations it makes
  visible, the landing page and README chip, the installation page's bundle
  row, `docs/CHANGELOG.md`.

Not in scope: **BREAKING** nothing. No CRD field is added or changed, no
manager code changes, no Pipeline, profile or toolset is edited to use it —
`Pipeline.spec.runtimeRef` is the entire switch (the profile's is deprecated).
Also out of scope: a Copilot-flavoured toolset catalog, a vocabulary field on
any CR, a runtime-kind discriminator in `internal/`.

## Capabilities

### New Capabilities
- `copilot-agent-runtime`: the third reference runtime — its obligations under
  the work contract, how a runtime-minted session id carries
  `runtimeContextId`, the two-layer tool-vocabulary translation and its
  deny-on-unmapped rule, MCP config translation with in-process secret
  resolution, the continuity behaviour when session state is gone, and how its
  bundle declares it.

### Modified Capabilities
- `agent-definition-tools`: the definition's location and frontmatter default
  are currently stated as claude-code's (`.claude/agents/<agent>.md`). They
  become per-runtime facts, with one rule binding every runtime: an absent or
  unparseable declaration contributes NOTHING, and no vendor's "omitted means
  everything" default may leak through.

`agent-runtime-ownership` and `runtime-declaration` are NOT modified: a bundle
may already ship a vendor's runtime and inherit the parent's defaults, and this
is the third instance of that rule. `builtin-toolset-catalog` is NOT modified:
unmapped-denies is its existing report-what-you-cannot rule applied to a
vocabulary boundary.

## Impact

- **New component**: `runtimes/copilot/` (Node, `package.json`, own
  `Dockerfile`, `node --test` suite), image `agentops-runtime-copilot` on
  `ghcr.io`, subject to the Trivy gate every image passes.
- **Dependency**: `@github/copilot-sdk`, which bundles the Copilot CLI (Node
  20+). A GitHub credential with Copilot access as `COPILOT_GITHUB_TOKEN`,
  projected from a Secret the bundle may create — the manager reads none.
- **Chart**: new `chart/charts/copilot/`, `chart/Chart.yaml` dependency,
  `chart/values.yaml` section, one entry in `agentops.declaredRuntimes`, the
  chart minor bump. Existing installs render byte-identically.
- **Reference docs**: `docs/contracts.md` (third implementation, the
  vocabulary-translation obligation), `docs/concepts.md` (capability resolution
  when the runtime's vocabulary differs), `docs/CHANGELOG.md`.
- **Adopter site**: new `docs/runtimes/copilot.md` (with a
  `renders bundle=copilot` marker and the `docs-generate.py` bundle entry),
  `docs/_data/nav.yml`, `docs/index.md` and `README.md` chips, `docs/installation.md`
  bundle row and section.
- **Context**: `.claude/rules/structure.md` (the component), `wiring.md` (the
  definition path is per-runtime), `docs/CLAUDE.md` (a second runtime page).
- **Unchanged**: `api/v1alpha1/`, every CRD, `platform/manager/internal/`,
  `runtimes/claude/`, `runtimes/ollama/`. If this change needs a manager edit,
  the contract was not vendor-neutral and that finding is the more important
  outcome.
- **Operational reach**: none by default — nothing renders unless an operator
  enables the bundle.
