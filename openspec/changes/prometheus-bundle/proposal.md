## Why

`vm-bundle` is named for a vendor it does not actually depend on. Its ingest
core — `signal-vmalertmanager` — parses the standard Alertmanager webhook
payload and nothing else: `alerts[]` with `status`, `fingerprint`, `startsAt`,
`labels`, `annotations`, `generatorURL`, firing-only, envelope fields ignored.
Any Prometheus Alertmanager can post to it today. The VictoriaMetrics name turns
a general capability into one that reads as unavailable to most installs.

The same is true one layer down. Verified live against VictoriaMetrics vmsingle
during this change's research: `/api/v1/query`, `/prometheus/api/v1/query` and
`/api/v1/labels` all answer with the Prometheus response envelope, and
`/api/v1/status/buildinfo` reports **version 2.24.0** — VM impersonates
Prometheus deliberately, for clients that version-gate. MetricsQL is a PromQL
superset. So a single Prometheus MCP server serves both backends, and the
VM-specific metrics server the bundle ships is redundant.

And like `k8s-bundle` before the wiring change, the bundle installs an ingest
path that **drops every alert at `Wired=False`** — it ships no profile and no
Pipeline, so an operator must supply both before anything answers.

## What Changes

- **BREAKING — the bundle is renamed.** `chart/charts/vm-bundle/` →
  `chart/charts/prometheus-bundle/`, values key `vm-bundle:` →
  `prometheus-bundle:`, spec `vm-bundle` → `prometheus-bundle`,
  `docs/vm-bundle.md` → `docs/prometheus-bundle.md`. VictoriaMetrics stops being
  the subject and becomes one supported backend.
- **BREAKING — the logs component is removed, not ported.** `mcp.vmlogs` and the
  `mcp-victorialogs` server workload go. VictoriaLogs speaks LogsQL over its own
  endpoints and no Prometheus server can query it, so there is nothing to
  generalize. The migration prints the `MCPConfig` to hand-apply, because
  removing it silently would cost a working install its log access.
- **BREAKING — one metrics MCP server, under the fixed key `prometheus`.**
  `mcp.vmmetrics` (server key `victoriametrics`) is replaced by a deployable
  generic Prometheus MCP server serving both backends, with its own `MCPToolset`.
  Allowlists naming `mcp__victoriametrics__*` stop resolving and must be
  restated.
- **The bundle ships an alert-investigator `AgentProfile`** — identity only, in
  the shape of `k8s-bundle`'s `k8s-engineer`: no repository, no capabilities, an
  inline `systemPrompt` role, and no execution substrate.
- **The bundle ships its own wiring**, a `pipelines` component defaulting OFF,
  rendering one Pipeline that claims its own alert source with its own profile,
  toolset and MCPConfig. Permitted by the chart-managed-wiring rule as relaxed by
  `k8s-bundle-wiring`, and only because of the profile above: channels are then
  the sole foreign name, values-supplied and omitted when unset.
- **VictoriaMetrics self-registration is KEPT, and labelled VM-only.** It writes
  a `VMAlertmanagerConfig` and therefore cannot work against vanilla
  Alertmanager, whose configuration is a file rather than a CR. The bundle says
  so plainly instead of leaving it looking backend-neutral.
- **Vanilla Alertmanager gets the printed receiver stanza** in `NOTES.txt`: the
  exact `receivers:` + `webhook_configs` block with the Service URL, the bearer
  form when the source carries `credentialsSecretRef`, and `send_resolved: false`
  because the adapter drops non-firing alerts.
- **`NOTES.txt` reports what the bundle wired**, and reports an install-declared
  Pipeline also claiming the alert source as a NOTE — never a render failure.
  Sources are shareable and the `sourceConflicts` guard is deleted.
- **Two stale statements are corrected.** The bundle spec and `docs/vm-bundle.md`
  both describe a `defaultSource.profileRef` and "the Pipeline claiming it" that
  no template renders; `Chart.yaml`'s description still advertises
  vmlogs/vmmetrics. The contradiction is resolved the way `k8s-bundle`'s was —
  one statement stands, conditioned on the new wiring flag.

Not in scope: any change to the `signal-vmalertmanager` Go module (its payload
handling is already vendor-neutral and correct), fan-out or shareable-source
semantics, and `k8s-bundle` or `telegram-bundle`.

## Capabilities

### New Capabilities

- `prometheus-bundle`: the renamed and reshaped subchart — vendor-neutral
  Alertmanager ingest, the VM-only registration sub-feature, one Prometheus
  metrics MCP component with its deployable server, the alert-investigator
  profile, and the bundle's own default-off wiring.

### Modified Capabilities

- `vm-bundle`: every requirement is REMOVED, superseded by `prometheus-bundle`.
  The capability does not survive the rename under two names.
- `documentation-structure`: the contributor routing rule names
  `docs/vm-bundle.md` as the destination for bundle content; it becomes
  `docs/prometheus-bundle.md`.

## Impact

- **Renamed**: `chart/charts/vm-bundle/` → `chart/charts/prometheus-bundle/`
  (Chart.yaml, values.yaml, `templates/alertmanager.yaml`, `templates/mcp.yaml`,
  `templates/mcp-servers.yaml`), `docs/vm-bundle.md` →
  `docs/prometheus-bundle.md`, `openspec/specs/vm-bundle/` →
  `openspec/specs/prometheus-bundle/`.
- **New**: `templates/profile.yaml`, `templates/pipelines.yaml` and the wiring
  helpers in the subchart's `_helpers.tpl` (the subchart has none today).
- **Modified**: `chart/Chart.yaml` (dependency name + condition) and
  `chart/Chart.lock`; `chart/values.yaml` (the pointer block); `chart/templates/
  NOTES.txt` (`index .Values "vm-bundle"`, the receiver stanza, the wiring
  report); `README.md` documentation index; `CLAUDE.md` (the map entry, and the
  wiring gotcha which currently names `vm-bundle` as a bundle shipping none);
  `internal/integration/charttemplate_test.go`; `CHANGELOG.md`.
- **Depends on** `k8s-bundle-wiring`, which relaxed the chart-managed-wiring
  requirement to permit a qualifying subchart to ship Pipelines. That change is
  implemented and awaiting archive; this one assumes its rule and adds no
  further relaxation.
- **Behavior change for existing installs**: every `vm-bundle.*` value must be
  restated under `prometheus-bundle.*` or the bundle silently renders nothing;
  any Pipeline allowlist naming `mcp__victoriametrics__*` or
  `mcp__victorialogs__*` stops resolving; log access must be re-added by hand.
  All three belong in the CHANGELOG's upgrade steps.
- **No effect on the manager, the CRDs, the console, or any Go module** — the
  adapter is untouched, and this is a chart that renames itself, drops a
  redundant component, and starts writing two CRs an operator used to write.
