---
title: Prometheus
permalink: /integrations/prometheus/
description: >-
  A bundle that turns firing alerts into answered conversations. Configure it,
  choose which alerts reach it, and bind its parts into your own pipelines.

next:
  eyebrow: Next
  title: An assistant for your home
  body: >-
    The same shape for Home Assistant — its logs start work, and two agents
    split by what they are allowed to touch.
  url: /agent-ops-operator/integrations/home-assistant/
---

**An alert says a threshold was crossed. It does not say why.** This bundle
receives firing alerts and hands an agent the metrics behind them, so what
reaches you has the investigation already done.

![Two lanes meet on the route: firing alerts your Alertmanager sends, and the metrics the query server exposes through the toolset you bind.]({{ '/assets/img/integrations/prometheus-light.svg' | relative_url }}){: .ao-diagram}

**It is named for the payload and the query API, not for one backend.** Any
Prometheus Alertmanager can post to it, and VictoriaMetrics answers the same
query API — MetricsQL is a PromQL superset, so one server serves both.

## What you get

| You get | Which means |
|---|---|
| **An investigated alert** | The agent queries the metric that fired, shows its evidence, and recommends rather than claims to have fixed anything. |
| **One conversation per alert** | Not one per firing repetition — the fingerprint collapses recurrences. |
| **Read-only metrics access** | Six query tools, all read-only. There is no acting posture to choose. |
| **Parts you can reuse** | The source, the profile and the tooling are ordinary CRs. Bind them from your own pipelines — see [Adopt it](#adopt-it). |

## Turn it on

**Three flags, because this bundle can render nothing you have not pointed at
something.**

```sh
helm upgrade --install agent-ops oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator \
  -n agent-ops --create-namespace \
  --set prometheus.enabled=true \
  --set prometheus.alertmanager.defaultSource.enabled=true \
  --set prometheus.mcp.enabled=true \
  --set prometheus.mcpServers.enabled=true \
  --set prometheus.mcpServers.backend=http://vmsingle-vm.monitoring.svc:8429 \
  --set prometheus.pipelines.enabled=true
```

```powershell
helm upgrade --install agent-ops oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator `
  -n agent-ops --create-namespace `
  --set prometheus.enabled=true `
  --set prometheus.alertmanager.defaultSource.enabled=true `
  --set prometheus.mcp.enabled=true `
  --set prometheus.mcpServers.enabled=true `
  --set prometheus.mcpServers.backend=http://vmsingle-vm.monitoring.svc:8429 `
  --set prometheus.pipelines.enabled=true
```

> **`defaultSource.enabled` is not optional if you want the route to answer.**
> Without it the bundle renders a `Pipeline` that claims no source at all — it
> installs, reports Ready, and nothing ever reaches it.
{: .ao-callout}

**Off by default, and demo mode never enables it.** Every component needs an
endpoint no demo cluster has.

### Point your Alertmanager at it

The post-install notes print this filled in with your Service URL and source
name:

```yaml
receivers:
  - name: agentops
    webhook_configs:
      - url: http://agentops-signal-alertmanager.agent-ops.svc:8080/webhook/alerts
        # the adapter drops non-firing alerts, so resolutions would be discarded
        send_resolved: false
route:
  receiver: agentops
```

Set `alertmanager.defaultSource.credentialsSecretRef` to require a bearer token,
and the notes print the matching `http_config.authorization` block beside it.

### Where the answers turn up

**Nowhere you can see, until you say where.** Unlike the Kubernetes bundle, this
route binds no channel — so start by reading the CR:

```sh
kubectl get conversations -o jsonpath='{.items[*].status.runs[*].result}'
```

```powershell
kubectl get conversations -o jsonpath='{.items[*].status.runs[*].result}'
```

Then bind somewhere you actually look:

```sh
--set prometheus.pipelines.channels={console}
```

```powershell
--set prometheus.pipelines.channels={console}
```

## The values you set

Everything below is under `prometheus:`.

### What exists

| Value | Default | Set it to |
|---|---|---|
| `enabled` | `false` | `true` to render the bundle |
| `alertmanager.enabled` | `true` | `false` if you post alerts through your own adapter |
| `alertmanager.defaultSource.enabled` | **`false`** | `true`, or the route claims nothing |
| `mcp.enabled` | `false` | `true` to give the agent metrics tools |
| `mcpServers.enabled` | `false` | `true` to deploy the query server |
| `mcpServers.backend` | **empty, required** | your Prometheus-API endpoint |
| `profile.enabled` | `true` | `false` if you bring your own agent |
| `pipelines.enabled` | `false` | `true` for a working route |

### What things are called

Rename any of these and your own pipelines reference the new name.

```yaml
prometheus:
  alertmanager:
    name: alertmanager                  # the SignalAdapter, and the routing
                                        # key a SignalSource selects it by
    defaultSource:
      name: alerts                      # the SignalSource your pipeline claims
  profile:
    name: alert-investigator            # the AgentProfile
  mcp:
    name: prometheus-api                # the MCPConfig
    toolset:
      name: prometheus-observability    # the toolset granting the query tools
  pipelines:
    name: alert-triage                  # the route
```

### The backend URL is never derived

Single-node VictoriaMetrics serves `/api/v1`, cluster mode serves
`/select/<accountID>/prometheus/api/v1`, and Prometheus serves `/api/v1` under
whatever external URL it was given.

**No template can guess among those**, and guessing wrong gives you a server
that starts and answers nothing. Already run a query MCP server? Point at it and
skip the workload:

```sh
--set prometheus.mcp.url=http://mcp-prometheus.agent-ops.svc:8080/mcp \
--set prometheus.mcpServers.enabled=false
```

```powershell
--set prometheus.mcp.url=http://mcp-prometheus.agent-ops.svc:8080/mcp `
--set prometheus.mcpServers.enabled=false
```

## Choose which alerts reach it

### "Only page the agent for what matters"

**Filter on the sender.** Everything you already do with Alertmanager routing
applies — the adapter answers whatever your receiver sends it:

```yaml
route:
  routes:
    - receiver: agentops
      matchers: [severity =~ "critical|warning"]
      continue: true
```

**`continue: true` matters.** Without it, an earlier route matching the same
alerts terminates and yours never fires.

### "Let the bundle configure my Alertmanager"

Running **VictoriaMetrics**? The adapter writes the receiver itself:

```sh
--set prometheus.alertmanager.registration.enabled=true
```

```powershell
--set prometheus.alertmanager.registration.enabled=true
```

It writes a `VMAlertmanagerConfig` pointing at its own endpoint, with
`continue: true` so your existing receivers keep their alerts.
`registration.matchers`, `groupWait`, `groupInterval`, `repeatInterval` and
`maxAlerts` shape it.

**This cannot work for vanilla Alertmanager**, and that is a property of
Alertmanager rather than a gap — its configuration is a file or a Secret, not an
object an adapter can write. Paste the receiver above instead.

**Registration failure never unserves the source.** The webhook stays live, the
Ready condition names the cause and the manual step, and it retries every 15s.

### "Resolutions are noise"

Already handled — **anything not `firing` is dropped**, which is why the printed
receiver sets `send_resolved: false`.

## Adopt it

Turn the bundle's own route off and declare your own:

```sh
--set prometheus.enabled=true --set prometheus.pipelines.enabled=false
```

```powershell
--set prometheus.enabled=true --set prometheus.pipelines.enabled=false
```

### Your agent, its metrics tools

```yaml
pipelines:
  - name: my-alert-agent
    profile: my-investigator      # YOUR profile, from your repository
    signalSources: [alerts]
    toolsets: [agentops-observe, prometheus-observability]
    mcpConfigs: [prometheus-api]
    channels: [team-chat]
```

### One agent for alerts *and* your cluster

The case no bundle can ship, because it spans two of them:

```yaml
pipelines:
  - name: ops
    profile: k8s-engineer
    signalSources: [alerts, cluster-events]
    toolsets: [agentops-observe, prometheus-observability, k8s-observability]
    mcpConfigs: [prometheus-api, k8s-api]
    channels: [team-chat]
```

An alert now arrives with both the metrics *and* the cluster available to
whoever investigates it.

### Metrics tools on a chat agent

```yaml
pipelines:
  - name: ask-the-metrics
    profile: alert-investigator
    signalSources: [team-chat]
    toolsets: [agentops-observe, prometheus-observability]
    mcpConfigs: [prometheus-api]
    channels: [team-chat]
```

### Run yours beside the bundle's

**Sources are shareable.** Leave `pipelines.enabled=true` and claim `alerts`
from your own route as well.

Each admitted alert then opens **one conversation per claiming pipeline**, each
with its own agent and tools. Two conversations means two sets of model credits,
and that is the only cost.

### What the bundle renders

Everything the flags above produce, so you know what a name refers to before you
bind it.

<!-- generated: renders bundle=prometheus -->
```text
# Ingest lane  (alertmanager.enabled)
SignalAdapter/alertmanager

# Profile  (profile.enabled)
AgentProfile/alert-investigator

# MCP tooling  (mcp.enabled)
MCPConfig/prometheus-api
MCPToolset/prometheus-observability

# MCP server  (mcpServers.enabled)
Deployment/agentops-mcp-prometheus
Service/agentops-mcp-prometheus

# Wiring  (pipelines.enabled)
Pipeline/alert-triage
```
<!-- /generated -->

- **The `SignalSource` appears only under `defaultSource.enabled`.** The chart
  renders the adapter CR — the reconciler owns the workload, the webhook Service
  and its listen address.
- **No account is rendered for the query server.** It reads an HTTP endpoint
  rather than the Kubernetes API, so it needs no RBAC at all.
- **The profile is behaviour only.** It has no repository, so its inline system
  prompt is the whole of its judgement.

## Going deeper

How signals are grouped, cooled down and turned into conversations:
[concepts]({{ '/concepts.md' | relative_url }}).
