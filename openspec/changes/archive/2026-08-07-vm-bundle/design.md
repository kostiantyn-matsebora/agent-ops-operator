# vm-bundle — design

> **Rebase note 2026-08-07 (`pipeline-only-wiring` landed after this was written):** SignalSource no longer carries `profileRef`/`channelRef` — wiring lives exclusively on `Pipeline`, and unclaimed sources drop signals (`Wired=False`). The `defaultSource` component therefore renders a SignalSource **plus a Pipeline** claiming it (`profileRef` and optional `channels` land on the Pipeline). Version numbers below are also stale: `k8s-bundle` has not landed, current chart is 1.4.0 → this change ships 1.5.0.

## Context

This change merges and supersedes `vm-alert-manager-as-signal-source`, whose design stands except for packaging: the webhook-receiving adapter no longer ships as parent-chart templates but as a component of a `vm-bundle` subchart, alongside two MCPConfig components. The bundle follows the conventions set by the `k8s-bundle` change (embedded subchart under `chart/charts/`, self-gated templates, default-off, per-component flags).

Key existing machinery: `MCPConfig` CRs are named MCP server sets (`spec.servers: {key: {type, url, headers[]}}`, headers support `valueFrom` secret refs — compiled by `mcpcompile` into mcp.json + env, manager reads no Secrets) referenced by AgentProfiles via `spec.mcp.configRefs`; the user-facing tool namespace is `mcp__<serverKey>__*` in `allowedTools`. The existing `home-observability` sample already wires `victorialogs`/`victoriametrics` SSE servers exactly this way — the bundle productizes that pattern.

## Goals / Non-Goals

**Goals:**

- One subchart for the VictoriaMetrics experience: alert ingestion (VMAlertmanager webhook → conversations) + investigation tooling (vmlogs/vmmetrics MCP servers for the handling profile).
- Same bundle rules as `k8s-bundle`: off by default, components individually toggleable, cross-references values-resolvable.
- Carry the merged change's adapter scope intact (module, constant, normalization, auth, Service-on-deterministic-labels).

**Non-Goals:**

- No demo coupling: `global.demo.enabled` does NOT enable this bundle — it depends on operator-supplied VM endpoints (and typically MCP server deployments) that a demo cluster doesn't have. Only `vm-bundle.enabled` turns it on.
- No profile shipped in this bundle: the SignalSource's `profileRef` and the MCP `configRefs` point at an operator-owned profile (e.g. `k8s-bundle`'s `k8s-engineer` or a custom SRE profile). Shipping a VM-flavored profile is a possible follow-up.
- Not deploying VictoriaMetrics or VMAlertmanager themselves — the bundle consumes their endpoints. ~~Nor the MCP server workloads~~ **(amended by user request 2026-08-07: each MCP component gains an optional `deploy` sub-block — off by default — rendering the upstream MCP server (ghcr.io/victoriametrics/mcp-victorialogs / mcp-victoriametrics) as a Deployment+Service in SSE mode pointed at a required `backend` URL; when deployed and `url` is left empty, the MCPConfig defaults to the deployed Service's SSE URL).**
- Built-in `alertmanagerWebhook` untouched (spec-pinned); no `SignalAdapterSpec` schema changes.

## Decisions

### D1: Same subchart mechanics as k8s-bundle, minus the demo gate

`chart/charts/vm-bundle/`, embedded (no `dependencies:` entry), every template gated on `.Values.enabled` alone. Deliberate divergence from `k8s-bundle`'s `enabled OR global.demo.enabled`: demo mode must stay meaningful on a bare cluster, and a VM bundle without reachable VM endpoints would render broken CRs (MCPConfigs pointing nowhere, an adapter no VMAlertmanager posts to). Values:

```yaml
enabled: false
alertmanager:
  enabled: true
  image: { repository: kmatsebora/agentops-signal-vmalertmanager, tag: ... }
  resources: {}
  service: { port: 8080 }        # targetPort fixed 8080 (adapter LISTEN_ADDR default)
  defaultSource:
    enabled: false
    name: vm-alerts              # SignalSource + Pipeline CR name
    profileRef: ""               # required — rendered onto the PIPELINE (see rebase note)
    channels: []                 # optional Channel names for the Pipeline (mirroring)
    grouping: {}
mcp:
  vmlogs:
    enabled: true
    name: vm-logs                # MCPConfig CR name
    url: ""                      # required when enabled, e.g. http://mcp-victorialogs.<ns>.svc/sse
    headers: []                  # optional, passthrough incl. valueFrom secret refs
  vmmetrics:
    enabled: true
    name: vm-metrics
    url: ""
    headers: []
```

`mcp.*.url` empty while the component is enabled → render-time `fail` with a message naming the value (a silently unreachable MCP server is the worst failure mode — agents just lose tools).

### D2: Two MCPConfig CRs with pinned server keys

`vmlogs` and `vmmetrics` render as two separate `MCPConfig` CRs (independently toggleable, independently referencable via `configRefs`) rather than one combined config. Server keys are fixed `victorialogs` / `victoriametrics` — NOT values-configurable — because the key is the tool namespace (`mcp__victorialogs__*`) baked into profiles' `allowedTools`; a configurable key would silently break tool allowlists. `type: sse` with values-passthrough `headers` (the `MCPServer` header entries support `valueFrom.secretKeyRef`, so authenticated VM endpoints work without the manager touching Secrets). CR names default `vm-logs`/`vm-metrics`, values-overridable.

Profile wiring is documented, not automated: an operator adds `configRefs: [{name: vm-logs}, {name: vm-metrics}]` and the `mcp__victorialogs__*`/`mcp__victoriametrics__*` allowlist entries to whichever profile handles VM alerts — the same profile named in `defaultSource.profileRef`. (Mutating another bundle's profile from this one would create cross-subchart coupling; rejected.)

### D3: Carried over from the merged change (now authoritative here)

The `vm-alert-manager-as-signal-source` change directory is retired; its design substance carries forward unchanged:

- **New type, built-in untouched**: `SourceVMAlertmanager = "vmAlertmanagerWebhook"`; the in-process `alertmanagerWebhook` endpoint and behavior stay as spec-pinned. Migration between them is per-source.
- **Webhook server**: adapter listens on `:8080` (`LISTEN_ADDR` via `spec.env`), `POST /webhook/{source}`, sources learned by polling `GET /signal/sources?type=vmAlertmanagerWebhook` (15s, cron pattern), 404 for unknown sources (VMAlertmanager retries cover the poll lag), 1 MiB body cap.
- **Normalization (built-in-path parity)**: firing-only filter; fingerprint verbatim from Alertmanager, sorted-label-hash fallback when absent (`/signal/inbound` rejects empty); raw label passthrough; title `"🔍 " + alertname` (+ ` — namespace`); per-alert JSON payload (labels/annotations/startsAt/generatorURL); no `kind` → alert lane. No adapter-side grouping — manager-side from `spec.grouping`.
- **Opt-in bearer auth**: source with `credentialsSecretRef` → adapter requires `Authorization: Bearer` matching projected `AGENTOPS_CRED_<SOURCE>_TOKEN` (constant-time), 401 otherwise; uncredentialed sources accept anonymous posts (parity with built-in, ClusterIP-only).
- **Service from the chart, zero machinery changes**: subchart Service `agentops-signal-<name>` selecting `agentops.dev/signal-adapter: <name>` with numeric targetPort 8080; the pod-label contract gets pinned by an integration-test assertion.

### D4: Version coordination with k8s-bundle

Both pending changes add subcharts and touch parent `values.yaml`/README. `k8s-bundle` claims the 2.0.0 major (breaking demo values move). This change is additive: if it lands after `k8s-bundle`, it's a minor bump (2.1.0); if it somehow lands first, 1.3.0 — the changes share only conventions, not files, so order is free. The `global.demo` convention introduced by `k8s-bundle` is simply not consumed here.

## Risks / Trade-offs

- [MCP endpoints are deployment-specific with no sane defaults] → Required-value `fail` at render; README shows the two common shapes (in-cluster `mcp-victorialogs`/`mcp-victoriametrics` Services, external URLs with auth headers).
- [Split-brain profile wiring (source routes to a profile that lacks the MCP refs)] → Documented as the one manual step, with a complete worked example (profile + configRefs + allowedTools + defaultSource.profileRef); the agent still functions without MCP, just with less to investigate with.
- [Label-contract drift breaks the Service selector] → Same mitigation as the k8s-bundle events RBAC: integration test pins `agentops.dev/signal-adapter` as chart-consumed.
- [Unauthenticated default webhook] → Same posture as the built-in endpoint, ClusterIP-only; credentialed setup documented as recommended.
- [Adapter restart drops in-flight webhooks] → VMAlertmanager retry + at-least-once + manager cooldown; singleton Recreate keeps the window to seconds.

## Migration Plan

Purely additive: new module, one constant, one gated subchart. Nothing renders until `vm-bundle.enabled=true`. Users of the built-in `alertmanagerWebhook` migrate per-source at their own pace (new SignalSource with the new type — type is immutable — repoint VMAlertmanager's `webhook_configs`, retire the old source); both paths can run in parallel during cutover. Rollback = disable the flag; the reconciler removes the adapter workload with the CR, helm removes the Service and MCPConfigs.

## Open Questions

- None blocking. A VM-flavored profile component (mirroring `k8s-bundle.profile`) is noted as a follow-up if the manual profile wiring proves annoying in practice.
