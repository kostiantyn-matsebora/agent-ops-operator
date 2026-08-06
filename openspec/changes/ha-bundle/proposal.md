# Proposal: ha-bundle

## Why

The Home Assistant agent setup — the `ha-engineer` profile, its observability MCP servers, the Home Assistant MCP connection, and the log-alert ingest lane — exists today only as hand-applied CRs (mirrored in `config/samples/`). Packaging it as an optional **subchart** makes the first real-world agent bundle installable, configurable, and versioned with the release: a template for how domain bundles ship on top of the operator.

## What Changes

- New subchart **`chart/charts/ha-bundle/`** (parent `Chart.yaml` dependency with `condition: ha-bundle.enabled`, **disabled by default**), rendering:
  - `MCPConfig` — the observability MCP set (VictoriaLogs + VictoriaMetrics SSE URLs from values);
  - `AgentProfile ha-engineer` — repository (URL/ref/SSH-auth secret name), agent role, tool allowlist, max turns, MCP = the observability configRef + an inline Home Assistant SSE server (URL + Authorization header from a secret ref), agent env (HA token secret ref) — all from values;
  - `SignalSource ha-logs` — the HA log-alert ingest lane: type defaults to the built-in `alertmanagerWebhook` (fed by the user's vmalert/alertmanager rules over HA logs), opaque config and grouping from values, `profileRef` → the bundle's profile, optional `channelRef`.
- The bundle references secrets by **name only** (repo key, HA tokens) — it creates none; prerequisites documented.
- Every object name and field is a value; `ha-bundle.enabled=false` (default) renders nothing.

## Capabilities

### New Capabilities

- `ha-bundle`: the subchart — contents, values surface, disabled-by-default gating, secret-by-reference rule, and adoption path for installs that hand-applied the same CRs.

### Modified Capabilities

<!-- none — chart packaging only; no operator, API, or contract changes -->

## Impact

- New `chart/charts/ha-bundle/` (Chart.yaml, values.yaml, templates); parent `chart/Chart.yaml` gains its first `dependencies` entry; parent values gain the `ha-bundle:` block; chart minor bump.
- `config/samples/` keeps the raw-CR form (it documents the API; the subchart documents the packaging).
- Live install caveat: home-data-center already runs these CRs kubectl-applied under the same intent — enabling the bundle there requires an adoption step (helm ownership) or keeping it disabled; documented in the migration plan.
- README: bundle section (values, prerequisites, adoption).
