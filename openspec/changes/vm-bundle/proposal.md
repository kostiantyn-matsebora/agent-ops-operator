# vm-bundle

## Why

The VictoriaMetrics story is currently half-planned and half-private: the `vm-alert-manager-as-signal-source` change (merged into this one and retired) designed the webhook-receiving alert adapter but shipped it as loose parent-chart templates, and the VictoriaLogs/VictoriaMetrics MCP wiring that makes an agent able to actually *investigate* alerts exists only as a hand-written sample (`home-observability` MCPConfig). A `vm-bundle` subchart — following the `k8s-bundle` pattern — packages the full VictoriaMetrics experience as one unit: alerts flow in via the VM Alertmanager signal source, and the agent handling them gets `vmlogs`/`vmmetrics` MCP servers to query logs and metrics.

## What Changes

- New Helm subchart `chart/charts/vm-bundle/` (same rules as `k8s-bundle`: embedded subchart, self-gated templates, whole bundle `enabled: false` by default, per-component flags). Unlike `k8s-bundle` it is NOT enabled by demo mode — it requires an existing VictoriaMetrics stack whose endpoints only the operator can supply. Components:
  - **`alertmanager`**: the VM Alertmanager signal source — `SignalAdapter` CR (`type: vmAlertmanagerWebhook`, singleton), the `Service` exposing its webhook endpoint (selecting the reconciler's deterministic pod labels, numeric targetPort), and an optional default `SignalSource` gated on a configured `profileRef`.
  - **`mcp.vmlogs`**: an `MCPConfig` CR exposing a VictoriaLogs MCP server (server key `victorialogs`, `type: sse`, values-configured URL, optional auth headers with `valueFrom` secret refs) for AgentProfiles to reference via `mcp.configRefs`.
  - **`mcp.vmmetrics`**: same for VictoriaMetrics (server key `victoriametrics`).
- Everything absorbed from the merged `vm-alert-manager-as-signal-source` change (that change is retired): the `signal-vmalertmanager/` adapter module (webhook server at `/webhook/{source}`, firing-only normalization with verbatim/fallback fingerprints, opt-in bearer auth from projected source credentials), the `SourceVMAlertmanager = "vmAlertmanagerWebhook"` constant, image build target. The built-in in-process `alertmanagerWebhook` type stays untouched.
- **No manager/API/CRD schema changes** beyond the one constant — MCPConfig, credential projection, and the `/signal/*` contract are used as-is.

## Capabilities

### New Capabilities

- `vm-alertmanager-signal-adapter`: the webhook-receiving VM Alertmanager signal adapter (carried over from the merged change) — inbound webhook surface and source routing, Alertmanager payload normalization, opt-in bearer auth; chart packaging now lives in the bundle capability.
- `vm-bundle`: the subchart composition — component set and per-component flags, default-off enablement (no demo coupling), the webhook Service, MCPConfig CRs for vmlogs/vmmetrics with endpoint/auth values, and cross-component wiring (default SignalSource → operator profile → MCP refs).

### Modified Capabilities

None. The adapter work was already purely additive (new type string; `signal-adapter-lifecycle` field set untouched — the Service ships from the subchart on deterministic labels); MCPConfig CRs are ordinary chart-rendered CRs.

## Impact

- **New module**: `signal-vmalertmanager/` (unchanged scope from the merged change; dependency-free).
- **API**: one constant in `api/v1alpha1/signalsource_types.go`.
- **Chart**: new `chart/charts/vm-bundle/` subchart (templates: SignalAdapter + Service + optional SignalSource, two MCPConfig CRs); parent `values.yaml` gains a commented `vm-bundle:` override block; chart minor version bump (major already claimed by `k8s-bundle`; coordinate whichever lands second).
- **Docs**: README (VM bundle section: webhook URL for VMAlertmanager `webhook_configs`, MCP wiring into profiles via `configRefs` + `allowedTools` `mcp__victorialogs__*`/`mcp__victoriametrics__*`), CLAUDE.md (module map, build commands with `agentops-signal-vmalertmanager` image), `config/samples/samples.yaml`.
- **Merged change**: `openspec/changes/vm-alert-manager-as-signal-source/` is superseded and removed.
- **No conflicts** with the pending `k8s-bundle` change beyond both adding subcharts (shared `global` conventions; no file overlap).
