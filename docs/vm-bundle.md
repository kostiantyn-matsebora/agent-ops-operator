# VictoriaMetrics bundle (subchart)

The VictoriaMetrics subchart: alert ingestion and investigation tooling.


`chart/charts/vm-bundle/` packages the VM experience — **off by default,
never enabled by demo mode** (it consumes your VictoriaMetrics endpoints).
Components (individually toggleable once `vm-bundle.enabled=true`):

- **`alertmanager`** — the Alertmanager-webhook ingestion path: a
  `SignalAdapter` CR named `vm-alertmanager` (reference adapter
  `signal-vmalertmanager/`, accepts the standard Alertmanager webhook format)
  with `port: 8080` — the reconciler owns both the workload and the webhook
  Service; the chart ships no connectivity. Sources select it with
  `spec.adapter: vm-alertmanager`.

  With `registration.enabled=true` (plus the target
  `registration.vmalertmanager: {name, namespace}`) the **adapter configures
  the sender itself**: it writes a `VMAlertmanagerConfig
  agentops-<source>` — webhook receiver pointing at its own endpoint, route
  with `continue: true` so existing receivers keep their alerts — and the
  bundle renders the least-privilege Role/RoleBinding that makes it possible.
  The routing decision lives entirely in the source's `register` block
  (`matchers`, `groupWait`, `groupInterval`, `repeatInterval`, `maxAlerts`,
  `sendResolved`), so it can **replace** a hand-written receiver rather than
  sit beside one. Two things decide whether the replacement actually
  receives anything, and both live on the sender: vm-operator appends these
  routes *after* the ones in your base config, so an earlier route matching
  the same alerts needs `continue: true` or it terminates matching first;
  and it scopes them to their own namespace unless the VMAlertmanager sets
  `spec.disableNamespaceMatcher`.

  Registration failure never unserves the source: the webhook stays live and
  the source's Ready condition names the cause plus the manual step, retried
  every 15s so granting the permission heals it without a restart.

  Without registration, point VMAlertmanager at it yourself:

  ```yaml
  receivers:
    - name: agentops
      webhook_configs:
        - url: http://agentops-signal-vm-alertmanager.<ns>.svc:8080/webhook/<source>
          # recommended: create the source with credentialsSecretRef (Secret
          # key TOKEN) and configure the same bearer token here:
          # http_config: {authorization: {credentials: <token>}}
  ```

  `defaultSource.enabled=true` + `profileRef` renders a turnkey
  SignalSource **and the Pipeline claiming it** (wiring is pipeline-only —
  unclaimed sources drop signals with `Wired=False`). Migration from the
  built-in `alertmanagerWebhook` is per-source: create a new source with the
  new type, claim it in a pipeline, repoint `webhook_configs`, retire the old
  source — both paths can run in parallel during cutover.

- **`mcp.vmlogs` / `mcp.vmmetrics`** — `MCPConfig` CRs (`vm-logs`/`vm-metrics`)
  with fixed server keys `victorialogs`/`victoriametrics` (the keys ARE the
  tool namespaces). URLs point at your MCP servers; `headers` pass through
  with `valueFrom` secret refs for authenticated endpoints. Whenever either
  component is on, the bundle also renders the matching **`MCPToolset`**
  (`vm-observability`, name overridable via `mcp.toolset.name`) granting only
  the enabled components' tool namespaces.

- **`mcpServers`** (off by default) — optionally deploy the MCP server
  workloads themselves (upstream `ghcr.io/victoriametrics/mcp-victorialogs`
  / `mcp-victoriametrics` images in SSE mode; pin the tags in production).
  Each needs its `backend` (the VictoriaLogs/VictoriaMetrics instance URL);
  with the workloads deployed, empty `mcp.*.url` values default onto the
  deployed Services automatically.

The bundle ships **no profile** — `defaultSource.profileRef` names your own
alert-handling profile. The one manual wiring step is a stanza on the Pipeline
routing these alerts; the profile itself stays untouched, so pipelines that
share it are unaffected:

  ```yaml
  spec:
    mcpConfigs: {refs: [{name: vm-logs}, {name: vm-metrics}]}
    toolsets:   {refs: [{name: vm-observability}]}
  ```

Every route that should have these tools declares them. There is no
profile-side alternative and no default: profiles carry no capabilities, and a
Pipeline that declares none grants none.
