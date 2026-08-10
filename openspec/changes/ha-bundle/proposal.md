# Proposal: ha-bundle

## Why

The Home Assistant agent setup exists only as hand-applied CRs mirrored in `config/samples/`, and the bundle change that was meant to package it has been overtaken twice: it still declares `allowedTools` and `mcp.configRefs` on the `AgentProfile` (both deleted by `capabilities-are-wiring`), and it still assumes `POST /task` (deleted, not renamed). Rewriting it is cheaper than patching it.

The rewrite also changes the shape. A Home Assistant install has two genuinely different agent jobs — asking the house questions and administering it — and they want different privilege. Packaging one `ha-engineer` profile with one alert lane models neither. What the bundle should ship is a **privilege split**: a user-facing agent that reads state, an ops agent that can act, and the wiring that keeps "act" a deliberate step.

## What Changes

**A new signal adapter, `signal-ha/`** (own module, dependency-free, like every other adapter): it watches Home Assistant's log/event stream over the HA API and normalizes problems into signals. Configuration follows `signal-k8s-events` deliberately — ordered first-match-wins `rules` with Prometheus-style `for:` dwell, plus Alertmanager-style `route` inhibition — because that vocabulary was worked out for exactly this problem and a second spelling would be a second thing to learn. Cursor state via `/signal/state`, `SignalAdapter.spec.kubernetesAccess: false` (its data source is HA, not the cluster).

**The `ha-bundle` subchart**, self-gated and off by default, rendering components that each gate independently:

- **`SignalSource ha-logs`** + the adapter CR + its credential projection.
- **`MCPConfig ha-api`** — the Home Assistant MCP server, rendered only when its endpoint is configured. Server key fixed, the way `k8s-bundle` fixes `kubernetes`.
- **Two `MCPToolset`s split by risk** — `ha-observability` (read state, history, logbook) and `ha-admin` (call services, change configuration) — enumerated, not wildcarded, for the same reason `k8s-bundle` enumerates: one prefix spans both halves and defeats the split.
- **`AgentProfile ha-user`** — the house's user. Identity plus API connectivity env only; rendered when EITHER the MCP endpoint or the read-scoped API credential is configured.
- **`AgentProfile ha-ops`** — the administrator. Rendered only when the admin API credential is configured, which is its prerequisite.
- **Two `Pipeline`s**, behind `pipelines.enabled` (**default true**):
  - `ha-control` — profile `ha-user`, claims the console and telegram chat sources named in values, delivers to those channels, binds `ha-observability`.
  - `ha-ops` — profile `ha-ops`, claims `ha-logs`, delivers to the same channels, binds `ha-observability` + `ha-admin`.

**BREAKING to a standing rule**: `openspec/specs/pipeline-model/spec.md` currently states that no subchart may render a `Pipeline`. This change relaxes it — a subchart MAY render wiring when it is gated by an explicit flag and every cross-component reference is a values-supplied name that the template omits when empty. `k8s-bundle` and `telegram-bundle` keep shipping none; the rule stops being absolute and becomes conditional, with the conditions written down.

**Two corrections to the shape as sketched**, both forced by the current model rather than by preference:

- **Profiles carry no tooling.** "ha-user has mcpconfig as tools" is expressed as the Pipeline binding `toolsets`/`mcpConfigs` — an `AgentProfile` has no `allowedTools` and no `mcp`. Profiles carry identity, role prompt, and connectivity env.
- **Both lanes serve one surface, but only one lists it as a source.** Claiming and addressing are independent mechanisms: a claim decides who answers an UNADDRESSED message, while `/<pipeline> <task>` resolves by name with no claim check and no Ready check, and its reply lands in the originating thread regardless. Both pipelines are therefore reachable from one shared console or telegram surface — no second surface is needed. What listing the same chat source on BOTH would do is put the younger Pipeline at `Ready=False, reason=SourceConflict`: it would still answer when addressed, but it would drop out of the `/agents` listing (Ready-only) and read as broken wherever pipelines are displayed. So `ha-control` lists the chat sources and `ha-ops` lists `ha-logs` only — listing them on `ha-ops` grants nothing and costs discoverability. Escalating to the admin agent is then an explicit act, which is what a privilege split should feel like.

## Capabilities

### New Capabilities

- `ha-signal-adapter`: the `signal-ha` module — what it watches, how `rules`/`route` behave, its cursor, its credential, and the loop rule it inherits.
- `ha-bundle`: the subchart — components and their gates, the profile split, the toolset risk split, conditional rendering, and the two pipelines behind their flag.

### Modified Capabilities

- `pipeline-model`: "Chart-managed wiring is declared once, at the top" becomes conditional — a subchart MAY ship wiring under an explicit flag when its cross-component references are values-named and omitted when unset.

## Impact

- **New module** `signal-ha/` (Go, no dependencies outside the repo) plus its image build line in `CLAUDE.md`.
- **New subchart** `chart/charts/ha-bundle/` (Chart.yaml, values.yaml, templates); parent values gain `ha-bundle:`; chart minor bump.
- **Specs**: `pipeline-model` delta; two new capability specs.
- **Docs**: new `docs/ha-bundle.md` (per the bundle-page routing rule), `docs/contracts.md` if the adapter needs contract notes, `CLAUDE.md` map + terminology + the relaxed invariant, `CHANGELOG.md`.
- **Security note that must be written down**: an admin API token in profile `env` plus a shell tool reaches Home Assistant regardless of what the toolsets allow. The risk split is real for the MCP path and advisory for the credential path — the same asymmetry `k8s-bundle` documents for `kubectl` versus the MCP server's own identity.
- **Non-goals**: creating Secrets (referenced by name only, as always); shipping HA itself; a Pipeline for any bundle other than this one; changing how `/<pipeline>` addressing works.
- **Scope note**: the adapter is a module and the bundle is packaging. They are separable — if a smaller first cut is wanted, the bundle can ship against `signal-vmalertmanager` (HA logs → VictoriaLogs → vmalert → Alertmanager) and the adapter can follow. Called out here because the adapter is the larger half of this change.
