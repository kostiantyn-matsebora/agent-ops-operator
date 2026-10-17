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

**Named for the payload and the query API, not one backend.** Any Prometheus
Alertmanager can post to it, and VictoriaMetrics answers the same query API —
MetricsQL is a PromQL superset, so one server serves both.

## What starts work

A firing alert, posted to the bundle's webhook. One conversation per alert —
the fingerprint collapses recurrences, not every repetition.

The agent queries the metric that fired through six read-only tools, shows
its evidence, and recommends rather than claims to have fixed anything.

## What it may reach

One agent, `alert-investigator`, with read-only query tools — there is no
acting posture to choose. The source, the profile and the tooling are
ordinary CRs, reusable from your own pipelines (see [Adopt it](#adopt-it)).

## What it costs to turn on

```sh
helm upgrade --install agent-ops oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator \
  -n agent-ops --create-namespace \
  --set prometheus.enabled=true \
  --set prometheus.alertmanager.defaultSource.enabled=true \
  --set prometheus.mcp.enabled=true \
  --set prometheus.mcpServers.enabled=true \
  --set prometheus.mcpServers.backend=http://vmsingle-vm.monitoring.svc:8429 \
  --set prometheus.wiringEnabled=true
```

```powershell
helm upgrade --install agent-ops oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator `
  -n agent-ops --create-namespace `
  --set prometheus.enabled=true `
  --set prometheus.alertmanager.defaultSource.enabled=true `
  --set prometheus.mcp.enabled=true `
  --set prometheus.mcpServers.enabled=true `
  --set prometheus.mcpServers.backend=http://vmsingle-vm.monitoring.svc:8429 `
  --set prometheus.wiringEnabled=true
```

> **`defaultSource.enabled` is not optional if you want the route to answer.**
> Without it the bundle renders a `Pipeline` that claims no source — it
> installs, reports Ready, and nothing ever reaches it.
{: .ao-callout}

**Off by default, and demo mode never enables it.** Every component needs an
endpoint no demo cluster has.

**The backend URL is never derived.** Single-node VictoriaMetrics serves
`/api/v1`, cluster mode serves `/select/<accountID>/prometheus/api/v1`, and
Prometheus serves `/api/v1` under whatever external URL it was given — no
template can guess among those. Already run a query MCP server? Point
`mcp.url` at it and skip the workload:

```sh
--set prometheus.mcp.url=http://mcp-prometheus.agent-ops.svc:8080/mcp \
--set prometheus.mcpServers.enabled=false
```

```powershell
--set prometheus.mcp.url=http://mcp-prometheus.agent-ops.svc:8080/mcp `
--set prometheus.mcpServers.enabled=false
```

Point your Alertmanager at the webhook the post-install notes print filled in:

```yaml
receivers:
  - name: agentops
    webhook_configs:
      - url: http://agentops-signal-alertmanager.agent-ops.svc:8080/webhook/alerts
        send_resolved: false   # the adapter drops non-firing alerts anyway
route:
  receiver: agentops
```

Running **VictoriaMetrics**? `--set prometheus.alertmanager.registration.enabled=true`
writes that receiver for you, as a `VMAlertmanagerConfig` with `continue: true`
so your existing receivers keep their alerts. Vanilla Alertmanager can't take
this path — its config is a file, not an object an adapter can write.

Only want certain alerts to reach it? Filter on the sender, same as any other
Alertmanager route — `continue: true` matters, or an earlier matching route
eats the alert first:

```yaml
route:
  routes:
    - receiver: agentops
      matchers: [severity =~ "critical|warning"]
      continue: true
```

Every value here, and the rest the chart takes, is in
[`chart/charts/prometheus/values.yaml`](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/chart/charts/prometheus/values.yaml)
— `helm show values` is the full list, and this page does not repeat it.

## Adopt it

Turn the bundle's own route off and declare your own:

```sh
--set prometheus.enabled=true --set prometheus.wiringEnabled=false
```

```yaml
pipelines:
  - name: ops
    profile: k8s-engineer
    signalSources: [alerts, cluster-events]
    toolsets: [agentops-observe, prometheus-observability, k8s-observability]
    mcpConfigs: [prometheus-api, k8s-api]
    channels: [team-chat]
```

One agent for alerts *and* your cluster — the case no single bundle ships,
because it spans two of them.

**Sources are shareable.** Leave `wiringEnabled=true` to run your own route
beside the bundle's instead — two conversations per alert, two sets of model
credits, and that's the only cost.

## Going deeper

How signals are grouped, cooled down and turned into conversations:
[concepts]({{ '/concepts.md' | relative_url }}).
