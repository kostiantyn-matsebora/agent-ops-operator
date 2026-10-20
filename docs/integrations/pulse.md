---
title: Pulse
permalink: /integrations/pulse/
description: >-
  A bundle that turns a firing Pulse alert into an investigated conversation —
  and gives it two agents, split by what they may write.

next:
  eyebrow: Next
  title: Ask it from chat
  body: >-
    The same shape for Telegram — where a person types, and where every agent
    answers back.
  url: /agent-ops-operator/integrations/telegram/
---

**Pulse (rcourtman/pulse) watches Proxmox, Docker, Kubernetes and TrueNAS.**
This bundle receives a firing alert, hands an agent Pulse's own inventory,
metrics, alerts and storage history, and gives it exactly one Community-tier
way to act: an operator-state annotation.

![A firing Pulse alert reaches two routes through their own MCP pairs: one that only investigates, one that may also record a maintenance window or a criticality tag.]({{ '/assets/img/integrations/pulse-light.svg' | relative_url }}){: .ao-diagram}

## What starts work

A firing Pulse alert, posted to the bundle's own webhook. One conversation
per occurrence — by default the adapter mints `initiationId` from Pulse's
alert id plus its start time.

Pulse can reuse an alert id for what it itself calls a new occurrence. An id
alone would then continue an old conversation instead of opening a fresh one.

Set `spec.config.correlationStrategy: id` on the source for the old,
id-alone behavior instead: one conversation absorbing every re-fire of the
same alert id, however long ago it last resolved. A retry and a
firing/resolved pair of one occurrence land in the same conversation either
way.

**Pulse only sends webhooks, it has no inbound registration of its own.** So
"registering" means this adapter calling *Pulse's* API to tell it where to
post, and it keeps retrying until that succeeds.

A missing URL or token never unserves the webhook. Pulse is simply never
told to use it, and the source's Ready condition says so.

## What it may reach

Two agents, split by what they may write:

| Agent | Can |
|---|---|
| `pulse-observer` | Read live state, metrics, alerts, storage history. Nothing else. |
| `pulse-operator` | Everything `pulse-observer` can, plus record a maintenance window or a criticality tag. |

**The wall is which Pulse API token each agent's MCP pair carries**, not a
setting on either CR — Pulse's own scoped tokens (`monitoring:read` vs.
`monitoring:read`+`monitoring:write`) decide what a call may do, and
`mcp-pulse` forwards whichever token it was given, unchanged.

Pulse's governed plan/approve/execute flow is Pro-gated. Neither agent claims
to have fixed anything — the operator's two writes are notes for a human,
never a stand-in for a real fix.

## What it costs to turn on

Mint three scoped tokens — one per row above, plus one for registration
(needs a scope that can manage notification webhooks) — then:

```sh
helm upgrade --install agent-ops oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator \
  -n agent-ops --create-namespace \
  --set pulse.enabled=true \
  --set pulse.signal.defaultSource.enabled=true \
  --set pulse.signal.defaultSource.baseUrl=https://pulse.example.org \
  --set pulse.signal.credentials.token=<ingest-token> \
  --set pulse.mcpServer.enabled=true \
  --set pulse.mcp.observer.credentials.token=<read-scoped-token> \
  --set pulse.mcp.operator.credentials.token=<write-scoped-token> \
  --set pulse.wiringEnabled=true
```

```powershell
helm upgrade --install agent-ops oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator `
  -n agent-ops --create-namespace `
  --set pulse.enabled=true `
  --set pulse.signal.defaultSource.enabled=true `
  --set pulse.signal.defaultSource.baseUrl=https://pulse.example.org `
  --set pulse.signal.credentials.token=<ingest-token> `
  --set pulse.mcpServer.enabled=true `
  --set pulse.mcp.observer.credentials.token=<read-scoped-token> `
  --set pulse.mcp.operator.credentials.token=<write-scoped-token> `
  --set pulse.wiringEnabled=true
```

**Off by default, and demo mode never enables it.** Every component needs a
real Pulse instance and a real token.

**Pulse commonly runs outside the cluster** it watches. If so, also expose the
webhook and tell Pulse to use that exposure — both lines matter, since an
Ingress alone makes the path reachable without telling Pulse to call it:

```sh
--set pulse.signal.ingress.enabled=true \
--set pulse.signal.ingress.host=pulse-adapter.example.org \
--set pulse.signal.defaultSource.webhookUrl=https://pulse-adapter.example.org/webhook/pulse-alerts
```

No ingress controller? Expose `agentops-signal-pulse:8080` another way and
point `webhookUrl` at that instead — this chart renders only the Ingress
shape.

Already run an `mcp-pulse` yourself, or manage credentials as existing
Secrets? Every value here, and the rest the chart takes, is in
[`chart/charts/pulse/values.yaml`](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/chart/charts/pulse/values.yaml)
— `helm show values` is the full list, and this page does not repeat it.

## Adopt it

Turn the bundle's own routes off and wire your own, keeping the privilege
split if it's worth keeping:

```sh
--set pulse.enabled=true --set pulse.wiringEnabled=false
```

```yaml
pipelines:
  - name: estate
    profile: pulse-observer
    signalSources: [pulse-alerts]
    toolsets: [agentops-observe, pulse-observability]
    mcpConfigs: [pulse-observer-mcp]
    channels: [team-chat]
  - name: estate-ops              # reached by /estate-ops
    profile: pulse-operator
    signalSources: [pulse-alerts]
    toolsets: [agentops-observe, pulse-observability, pulse-admin]
    mcpConfigs: [pulse-observer-mcp, pulse-operator-mcp]
    channels: [team-chat]
```

**Sources are shareable.** Leave `wiringEnabled=true` instead to run your own
route beside the bundle's, both claiming `pulse-alerts` — each admitted alert
then opens one conversation per claiming pipeline.

## Going deeper

How signals are grouped, cooled down and turned into conversations:
[concepts]({{ '/concepts.md' | relative_url }}).
