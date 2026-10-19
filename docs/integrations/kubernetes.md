---
title: Kubernetes
permalink: /integrations/kubernetes/
description: >-
  A bundle that turns cluster events into answered conversations. Configure it,
  tune what reaches you, and bind its parts into your own pipelines.

next:
  eyebrow: Next
  title: Answer your alerts
  body: >-
    The same shape for Prometheus and Alertmanager — a firing alert arrives
    with the investigation already done.
  url: /agent-ops-operator/integrations/prometheus/
---

**Your cluster already says what is wrong. Nobody reads it.** This bundle reads
the events, decides which are real, and gives an agent the tools to look.

![Two lanes meet on the route: cluster events filtered by your suppression rules, and the MCP server's tools filtered by the toolsets you bind.]({{ '/assets/img/integrations/kubernetes-light.svg' | relative_url }}){: .ao-diagram}

`signals/k8s-events` conforms to the [signal adapter
specification]({{ '/contracts/signal-adapter/' | relative_url }}), version
2.0.0.

## What starts work

A cluster event, once triage drops the churn. A probe failing twenty seconds
from Ready is noise. A pod still broken five minutes later is not — only the
second reaches an agent.

Rules under `kubernetes.eventsAdapter.source.rules` decide which, see
[Tune what reaches you](#tune-what-reaches-you).

The adapter mints an `initiationId` of `<source>@<namespace>/<workload>` —
the owning Deployment or similar, resolved through owner references, never
the Event object or its reason — and the manager matches it by equality, so
a workload's whole run of failures across pods and restarts stays one
conversation.

## What it may reach

The agent reads pods, logs, events and nodes, then names the cause. Read-only
until you say otherwise:

| | Off (default) | `allowMutations: true` |
|---|---|---|
| **It can** | read pods, logs, events, nodes | those, plus delete, scale, restart, exec |
| **The route is called** | `k8s-observe` | `k8s-operate` |

> **`allowPodExecution` is separate, and defaults off.** A pod-executing agent
> can read any Secret that pod mounts. With it off, the shipped role tells the
> agent pod execution is withheld, so a request to change a Deployment is
> declined with that reason rather than tried and refused by RBAC.
{: .ao-callout}

Want the acting route without a server that can act? Set both independently:

```sh
--set kubernetes.allowMutations=true --set kubernetes.mcpServers.readOnly=true
```

## Where it answers

**It answers in the console already** — the route binds it and claims the
composer, so replies work with nothing else set. Add a chat surface beside it
with `--set kubernetes.pipelines.channels={team-chat}` — it merges rather than
replacing the console.

## What it costs to turn on

```sh
helm upgrade --install agent-ops oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator \
  -n agent-ops --create-namespace \
  --set kubernetes.enabled=true \
  --set kubernetes.wiringEnabled=true
```

```powershell
helm upgrade --install agent-ops oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator `
  -n agent-ops --create-namespace `
  --set kubernetes.enabled=true `
  --set kubernetes.wiringEnabled=true
```

**No credential.** It authenticates to the cluster it runs in. `enabled`
renders the parts, `wiringEnabled` wires a working route — leave the second
off if you're [bringing your own](#adopt-it).

Already run a Kubernetes MCP server?

```sh
--set kubernetes.mcpServers.enabled=false \
--set kubernetes.mcp.url=http://my-k8s-mcp.agent-ops.svc:8080/mcp
```

Everything else the chart takes — renaming any part, `mcp.transport`,
`profile.podExecutionWithheldPrompt` — is in
[`chart/charts/kubernetes/values.yaml`](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/chart/charts/kubernetes/values.yaml),
and this page does not repeat it.

## Tune what reaches you

Everything below is `kubernetes.eventsAdapter.source.rules`, which replace the
whole list — restate what you want to keep. Matchers are Alertmanager's (`=`,
`!=`, `=~`, `!~`), anchored, so `reason=~"Failed"` won't match `FailedMount`.

**Keep a last rule with no matchers** — it catches the failure nobody
anticipated.

| Want | Rule |
|---|---|
| Drop probe churn | `{matchers: ['reason=~"ProbeWarning\|SandboxChanged"'], action: drop}` |
| Wait before reporting unhealthy | `{matchers: ['reason="Unhealthy"'], for: 10m, escalateAfterObjects: 3}` |
| Report OOM kills immediately | `{matchers: ['reason="OOMKilling"'], for: "0"}` |
| Ignore a namespace | `{matchers: ['app.kubernetes.io/part-of="scratch"'], action: drop}` |

`for` holds and re-checks — recovered or gone means you never hear about it.
`escalateAfterObjects` overrides the wait when several objects fail at once.
Only a Pod is re-checked directly — any other kind is judged by whether its
events are still arriving as the wait closes.

**A cordoned or maintenance-tainted node already suppresses every event on
its objects**, for as long as it stays that way — no config needed (needs
`rbac.clusterWide: true`, the default). A drain outliving
`drainingNodeMatchers`'s `drainingNodeBound` (default `1h`) gets reported
once instead of staying silent forever.

No node state to key off (a router power-cycling, an ISP window)? Mute by
clock instead, naming your timezone and the reasons — an unnamed zone drifts
an hour every DST change, and unnamed reasons silence an OOM kill too:

```yaml
eventsAdapter:
  source:
    route:
      timeIntervals:
        - name: nightly-restart
          times: [{startTime: "04:00", endTime: "04:20"}]
          location: Europe/Kyiv
      muteTimeIntervals:
        - name: nightly-restart
          matchers: ['reason=~"NodeNotReady|Unhealthy|FailedMount"']
```

## Adopt it

**The bundle's parts are ordinary CRs with stable names.** Turn its own route
off and declare your own:

```sh
--set kubernetes.enabled=true --set kubernetes.wiringEnabled=false
```

```yaml
pipelines:
  - name: my-cluster-agent
    profile: my-engineer          # YOUR profile, from your repository
    signalSources: [cluster-events]
    toolsets: [agentops-observe, k8s-observability]
    mcpConfigs: [k8s-api]
    channels: [team-chat]
```

Add `team-chat` to `signalSources` too for one agent across both the cluster
and chat — the bundle's own route sees only its own objects, so this is the
one case it can't ship for you.

Drop `cluster-events` from `signalSources` instead for an agent you can ask
about the cluster with nothing starting it.

**Sources are shareable.** Leave `wiringEnabled=true` and claim
`cluster-events` from your own route too — each admitted event then opens one
conversation per claiming pipeline, at the cost of one more set of model
credits.

## Going deeper

How events are verified, grouped and suppressed, and why an agent reaches the
cluster through a server with its own identity:
[concepts]({{ '/concepts.md' | relative_url }}).
