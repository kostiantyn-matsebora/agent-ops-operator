## Context

See proposal.md — Why. Three things already true shape this:

- **`chart/templates/builtin-toolsets.yaml`** renders `global.builtinToolsets`'s
  three buckets (`observe`, `shell`, `edit`) as `MCPToolset` CRs through one
  generic loop over `(list "observe" "shell" "edit")` — no per-bucket
  template.
- **Neither bundle's own route naming matches the request literally.** The
  kubernetes bundle's own rendered admin route defaults to `k8s-operate`
  (`pipelines.admin.name`); `k8s-ops` is the name every worked example in
  `docs/concepts.md` and `docs/configuration.md` gives an INSTALL-DECLARED
  Pipeline built from the bundle's `k8s-engineer` profile plus its
  `k8s-observability`/`k8s-admin` MCPToolsets — the bundle's own wiring is off
  by default (`k8s-bundle`'s "off by default" requirement), so most real
  installs following the docs use the hand-declared route, not the bundle's.
  `ha-ops`, by contrast, IS the home-assistant bundle's own rendered route
  name (`pipelines.ops.name`), also off by default. This change treats
  "k8s-ops" as the kubernetes bundle's admin-tier route under EITHER path —
  the bundle's own (`k8s-operate`) and the documented hand-wired convention
  (`k8s-ops`) both gain the new toolset, so the request is satisfied
  whichever one an install actually uses.
- **Egress mediation forwards non-MCP traffic untouched.**
  `runtime-egress-mediation`'s "Non-MCP egress is forwarded untouched"
  requirement already covers the LLM API and repository access; a
  `WebSearch` call from the runtime CLI is the same class of traffic (not an
  MCP call), so it needs no new mediation path and creates no bypass of the
  MCP-scoped proxy — the boundary for this tool is the CLI's own
  `--allowedTools` enforcement, exactly as it already is for `Read`/`Edit`/`Bash`.

## Goals / Non-Goals

**Goals:**
- One new risk-split bucket, wired only onto the two routes already trusted
  to act on their target system.
- No change to any runtime's implementation, the egress proxy, or network
  policy — this is chart wiring exercising an existing, runtime-interpreted
  tool name.

**Non-Goals:**
- Constraining what `WebSearch` may reach (domain allow/deny lists). The CLI
  offers none today; if it later does, that is a values-in-profile-or-Pipeline
  question for a separate change.
- Implementing `WebSearch` in the `ollama` or `copilot` runtimes. Both report
  the tool unavailable per the existing runtime-interpreted contract, which
  is correct behavior, not a gap this change closes.
- Adding web search to `k8s-observe`/`ha-control`. Read-only routes stay
  read-only; an operator who wants it there binds the toolset themselves —
  nothing prevents that, this change simply does not default it on.

## Decisions

**A fourth bucket, not a special case.** `agentops-websearch` / `[WebSearch]`
joins `global.builtinToolsets` exactly like the other three: same defaults
shape, same values-extendable tool list, rendered by extending
`builtin-toolsets.yaml`'s loop from `(list "observe" "shell" "edit")` to
`(list "observe" "shell" "edit" "websearch")`. No new template, no new CR
kind — the whole value of the existing mechanism is that a new built-in name
costs one list entry.

**Bound on the admin/ops route only, via each bundle's existing per-route
tool-list variables.** Kubernetes' `pipelines.yaml` already separates
`$readTools` (shared by both routes) from `$writeTools` (admin-only,
concatenated in for the admin route); appending the new toolset name to
`$writeTools` scopes it to `k8s-operate` alone. Home-assistant's
`pipelines.yaml` separates `$baseTools` (both routes) from `$opsOnlyTools`
(ops-only); appending there scopes it to `ha-ops` alone. Alternative — add it
to the shared list — was rejected: it would hand `k8s-observe`/`ha-control`
open internet access neither bundle's spec claims for them, for a route this
change was never asked to touch.

**The install-declared `k8s-ops` documentation examples get the same entry.**
Since the kubernetes bundle's own wiring is off by default and the documented
pattern is a hand-declared Pipeline, leaving those examples unrevised would
mean the one target most readers actually build reads as unchanged by this
proposal. `docs/concepts.md`, `docs/configuration.md` and the commented
example in `chart/values.yaml` each add `agentops-websearch` to the
admin-tier example's `toolsets` list.

**No egress-proxy or network-policy change.** `runtime-egress-mediation`
already states non-MCP egress passes through unmediated — the same rule that
lets the LLM API and git operations work today. `WebSearch` is native CLI
egress, not an MCP call, so it is already in the "forwarded untouched"
category; this change grants the CLI PERMISSION to use it
(`--allowedTools`), which is the cooperating-agent boundary this whole
catalog already relies on for every other built-in.

## Risks / Trade-offs

- [`k8s-observability`/`k8s-admin`-style reviewers might expect a domain
  allowlist on web search, since the MCP-bound tools in the same bundle are
  tightly scoped] → documented as a Non-Goal; the CLI's `WebSearch` has no
  such control today, and the risk is named in the reference docs rather than
  implied to be bounded.
- [Only the reference `claude` runtime implements `WebSearch`; an install
  running `k8s-operate`/`ha-ops` on `ollama` or `copilot` gets a granted tool
  that always reports unavailable] → this is the existing, intentional
  runtime-interpreted contract (`builtin-toolset-catalog`'s own scenario for
  it), not a regression; documentation should say so plainly rather than
  implying the tool works everywhere.
- [Chart upgrade default-adds a tool to any install that already enabled
  either bundle's admin/ops wiring] → both routes are OFF by default per
  each bundle's own "off by default" requirement, so this only reaches
  installs that already opted into the admin-capable route; the CHANGELOG
  entry states the addition plainly so an upgrading operator can decide to
  unset it.
