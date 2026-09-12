## Why

The two flagship bundles ship an ops-capable route that troubleshoots a real
system — `k8s-operate` (the kubernetes bundle's admin route, commonly wired
as `k8s-ops` in an install's own `pipelines:`) and `ha-ops` (the
home-assistant bundle's own admin route). Both reach their target system
through an MCP server and, on `ha-ops`, a shell — but neither can look
anything up: an unfamiliar error message, a changelog for the version it's
running, a vendor's known-issues page. Read-only cluster or house access does
not cover that; it needs the open internet. The runtime already ships a
native `WebSearch` tool (verified in the reference `claude` runtime's CLI);
nothing in the wiring model grants it to anything today.

## What Changes

- The chart's built-in tool catalog (`global.builtinToolsets`, three
  risk-split buckets today: observe, shell, edit) gains a fourth:
  `agentops-websearch`, wrapping the single tool `WebSearch`. Rendered by the
  same generic loop as the other three — no new template, no new CR kind.
- The kubernetes bundle's admin route (`pipelines.admin`, default name
  `k8s-operate`) and the home-assistant bundle's ops route
  (`pipelines.ops`, default name `ha-ops`) each bind the new toolset. Neither
  bundle's read-only/control route (`k8s-observe`, `ha-control`) gains it —
  web search is scoped to the route already trusted to act, not to every
  route a profile serves.
- The worked examples showing an install-declared `k8s-ops`-style Pipeline
  (`docs/configuration.md`, `docs/concepts.md`, the commented example in
  `chart/values.yaml`) add `agentops-websearch` to the admin-tier example's
  toolsets list, since many installs wire this route themselves rather than
  enabling the bundle's own.
- **Not in this change**: any change to what the `claude` runtime's
  `WebSearch` tool can reach (no domain allow/deny list), and no change to
  `ollama` or `copilot` — both already report a granted-but-unimplemented
  tool per `builtin-toolset-catalog`'s existing contract, which is precisely
  what happens here until either implements it.

## Capabilities

### Modified Capabilities
- `builtin-toolset-catalog`: the risk split gains a fourth category —
  outbound web search — alongside observation, execution and workspace
  mutation.

### New Capabilities

(none — `k8s-bundle` and `ha-bundle` state no requirement about an exhaustive
tool list for either route, so binding one more toolset is additive and
needs no delta there)

## Impact

- `chart/templates/builtin-toolsets.yaml` — extend the rendered-key loop.
- `chart/values.yaml` — default `global.builtinToolsets.websearch`.
- `chart/charts/kubernetes/templates/pipelines.yaml`,
  `chart/charts/kubernetes/values.yaml` — bind the toolset on the admin route.
- `chart/charts/home-assistant/templates/pipelines.yaml`,
  `chart/charts/home-assistant/values.yaml` — bind it on the ops route.
- No manager, CRD, runtime or MCP-server code change: this is chart wiring
  onto an existing, runtime-interpreted tool name.

Documents this makes untrue, both halves:

- Reference docs: `docs/concepts.md`'s built-in toolset table (currently
  three rows) and `docs/configuration.md`'s worked Pipeline examples both
  gain the fourth row/entry. `docs/CHANGELOG.md` gets an `Added` entry (not
  breaking — additive, and both bundles' wiring defaults off).
- Adopter site: `docs/integrations/kubernetes.md` and
  `docs/integrations/home-assistant.md` — each has a `<!-- generated: renders
  bundle=... -->` block that must be regenerated once the admin/ops route's
  rendered `Pipeline` carries the new toolset ref, and each page's prose
  should say the admin-capable agent can now search the web.
