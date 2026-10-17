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
Like the others, its alerts and its own tooling are reachable only by a human
today.

This bundle receives a firing alert, hands an agent Pulse's own inventory,
metrics, alerts and storage history, and gives it exactly one Community-tier
way to act: an operator-state annotation.

![A firing Pulse alert reaches two routes through their own MCP pairs: one that only investigates, one that may also record a maintenance window or a criticality tag.]({{ '/assets/img/integrations/pulse-light.svg' | relative_url }}){: .ao-diagram}

## What you get

| You get | Which means |
|---|---|
| **An investigated alert** | The agent reads the resource's live state, its metrics and its history, and shows its evidence. |
| **One conversation per alert** | Not one per firing repetition — the fingerprint collapses recurrences. |
| **Two agents, one real wall** | `pulse-observer` only reads. `pulse-operator` can additionally record a maintenance window or a criticality tag. The wall is which Pulse API token each one's Secret carries, not a setting on either CR. |
| **No pretending to fix anything** | Pulse's governed plan/approve/execute flow is Pro-gated. Neither agent claims to have applied a fix — the operator's two tools are annotations, never a stand-in for one. |

## Turn it on

**Pulse sends outgoing webhooks only.** There is no receiver-registration
concept on its side — "registering" means this adapter calling *Pulse's own*
API to tell it where to post, reading the base URL and the API token off the
one default source this bundle renders.

```sh
helm upgrade --install agent-ops oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator \
  -n agent-ops --create-namespace \
  --set pulse.enabled=true \
  --set pulse.signal.defaultSource.enabled=true \
  --set pulse.signal.defaultSource.baseUrl=https://pulse.example.org \
  --set pulse.signal.credentials.token=<pulse-api-token> \
  --set pulse.mcpServer.enabled=true \
  --set pulse.mcp.observer.credentials.token=<read-scoped-token> \
  --set pulse.mcp.operator.credentials.token=<write-scoped-token> \
  --set pulse.pipelines.enabled=true
```

```powershell
helm upgrade --install agent-ops oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator `
  -n agent-ops --create-namespace `
  --set pulse.enabled=true `
  --set pulse.signal.defaultSource.enabled=true `
  --set pulse.signal.defaultSource.baseUrl=https://pulse.example.org `
  --set pulse.signal.credentials.token=<pulse-api-token> `
  --set pulse.mcpServer.enabled=true `
  --set pulse.mcp.observer.credentials.token=<read-scoped-token> `
  --set pulse.mcp.operator.credentials.token=<write-scoped-token> `
  --set pulse.pipelines.enabled=true
```

> **Registration failure never unserves the webhook.** `signal.defaultSource`
> renders and serves `POST /webhook/pulse-alerts` either way — a missing
> `baseUrl` or token just means Pulse itself was never told to post there, and
> the source's Ready condition names exactly that. Add either later and it
> heals with no pod restart.
{: .ao-callout}

**Off by default, and demo mode never enables it.** Every component needs a
Pulse instance and an API token no demo cluster has.

### The one decision: how many tokens to mint

| Token | Belongs to | Needs |
|---|---|---|
| the **ingest** token | registering the webhook (`POST /api/notifications/webhooks`) | a scope that can manage notification webhooks |
| the **observer** token | `pulse-observer`'s MCP path | `monitoring:read` |
| the **operator** token | `pulse-operator`'s MCP path | `monitoring:read` **and** `monitoring:write` |

**Pulse's API tokens are scope-able, and that is the real wall.** `mcp-pulse`
forwards whichever token a caller's bound `MCPConfig` supplied, unchanged, and
decides no reach of its own — Pulse's own API is what accepts or refuses an
operator-state write by that token's scope.

Prefer Secrets you already manage? Each credential also takes an
`existingSecret` name instead of a raw token — `signal.credentials`,
`mcp.observer.credentials`, `mcp.operator.credentials`.

### Where the answers turn up

**Nowhere you can see, until you say where.** Neither route binds a channel:

```sh
--set pulse.pipelines.channels={console}
```

```powershell
--set pulse.pipelines.channels={console}
```

Every answer is on the CR regardless:

```sh
kubectl get conversations -o jsonpath='{.items[*].status.runs[*].result}'
```

```powershell
kubectl get conversations -o jsonpath='{.items[*].status.runs[*].result}'
```

### How each agent is reached

Both routes claim the same alert source — wiring is many-to-many, so a firing
alert reaching two agents at once is the ordinary shape, not a conflict.

| Agent | Reached by |
|---|---|
| `pulse-observer` | the firing alert, or `/pulse-observer <task>` |
| `pulse-operator` | the firing alert, or `/pulse-operator <task>` |

## The values you set

Everything below is under `pulse:`.

### What exists

| Value | Default | Set it to |
|---|---|---|
| `enabled` | `false` | `true` to render the bundle |
| `signal.defaultSource.enabled` | `false` | `true`, or the route claims nothing |
| `signal.defaultSource.baseUrl` | **empty** | your Pulse instance's base URL |
| `signal.defaultSource.webhookUrl` | empty, chart renders the in-cluster Service URL | an externally reachable URL, when Pulse is outside the cluster |
| `signal.ingress.enabled` | `false` | `true` to expose the webhook path through your ingress controller |
| `signal.ingress.host` | **empty, required when enabled** | the hostname Pulse will call |
| `signal.credentials.token` | empty | the Pulse API token that registers the webhook, or `existingSecret` for a Secret you manage |
| `signal.credentials.signingSecret` | empty | the HMAC secret Pulse signs each webhook delivery with, so the adapter can verify it |
| `mcp.url` | empty, uses the deployed `mcp-pulse` Service | an `mcp-pulse` you already run elsewhere |
| `mcp.observer.enabled` | `true` | `false` to drop the read-only MCP pair |
| `mcp.observer.credentials.token` | empty | a `monitoring:read` token, or `existingSecret` |
| `mcp.operator.credentials.token` | empty | a `monitoring:read`+`monitoring:write` token, or `existingSecret` |
| `mcp.operator.enabled` | `true` | `false` to drop the write-capable MCP pair |
| `mcpServer.enabled` | `false` | `true` to deploy `mcp-pulse` |
| `mcpServer.backend` | **empty, required when `mcpServer.enabled`** | the Pulse endpoint the MCP server forwards to |
| `profiles.observer.enabled` | `true` | `false` to drop the investigating agent |
| `profiles.operator.enabled` | `true` | `false` to drop the annotating agent |
| `pipelines.enabled` | `false` | `true` for working routes |

### What things are called

Rename any of these and your own pipelines reference the new name.

```yaml
pulse:
  signal:
    name: pulse                       # the SignalAdapter, and the routing
                                       # key a SignalSource selects it by
    defaultSource:
      name: pulse-alerts               # the SignalSource your pipeline claims
  profiles:
    observer:
      name: pulse-observer             # the investigating AgentProfile
    operator:
      name: pulse-operator             # the annotating AgentProfile
  mcp:
    observer:
      name: pulse-observer-mcp        # the read-only MCPConfig
      toolset:
        name: pulse-observability
    operator:
      name: pulse-operator-mcp        # the write-capable MCPConfig
      toolset:
        name: pulse-admin
  pipelines:
    observer:
      name: pulse-observer            # the investigating route
    operator:
      name: pulse-operator            # the annotating route, and the
                                       # command that reaches it
```

**The server key behind both MCPConfigs is fixed at `pulse`**, and has no
values path — it is the `mcp__pulse__*` namespace the tool lists above name,
so renaming it would silently strip an agent's tools.

### Already run an mcp-pulse yourself?

Point `mcp.url` at it and skip the deployed workload:

```sh
--set pulse.mcp.url=http://mcp-pulse.agent-ops.svc:8080/mcp \
--set pulse.mcpServer.enabled=false
```

```powershell
--set pulse.mcp.url=http://mcp-pulse.agent-ops.svc:8080/mcp `
--set pulse.mcpServer.enabled=false
```

### Pulse installed outside the cluster

**Pulse commonly is NOT in-cluster.** It watches Proxmox, Docker or TrueNAS
hosts that may sit entirely outside Kubernetes, unlike Alertmanager, which
this repository's other bundles can assume runs alongside the cluster it
alerts on.

The two paths need different things from you:

| Path | Direction | Works unmodified? |
|---|---|---|
| MCP (agents reach Pulse) | out of the cluster | **Yes.** `signal.defaultSource.baseUrl` and `mcpServer.backend` already take any reachable URL — in-cluster or not |
| Webhook (Pulse reaches the adapter) | into the cluster | **No, by default.** The chart renders the adapter's own in-cluster Service DNS name, which an external Pulse cannot resolve |

Fix the webhook path with `signal.ingress` — it points an Ingress at the
SignalAdapter reconciler's own Service, so the chart does the exposing for
you if you already run an ingress controller:

```sh
helm upgrade --install agent-ops oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator \
  -n agent-ops --create-namespace \
  --set pulse.signal.ingress.enabled=true \
  --set pulse.signal.ingress.host=pulse-adapter.example.org \
  --set pulse.signal.ingress.tls.clusterIssuer=letsencrypt-prod \
  --set pulse.signal.defaultSource.webhookUrl=https://pulse-adapter.example.org/webhook/pulse-alerts
```

```powershell
helm upgrade --install agent-ops oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator `
  -n agent-ops --create-namespace `
  --set pulse.signal.ingress.enabled=true `
  --set pulse.signal.ingress.host=pulse-adapter.example.org `
  --set pulse.signal.ingress.tls.clusterIssuer=letsencrypt-prod `
  --set pulse.signal.defaultSource.webhookUrl=https://pulse-adapter.example.org/webhook/pulse-alerts
```

**Both lines matter.** The Ingress makes the path *reachable*. `webhookUrl`
is the separate step of telling Pulse to actually use it — without it the
chart's own unreachable in-cluster URL still gets registered, Ingress or
not.

No ingress controller? Expose the adapter's own `agentops-signal-pulse`
Service (port 8080) another way instead — a `LoadBalancer` Service, a
`NodePort`, or a tunnel you already run — and point `webhookUrl` at
whatever that gives you.

This chart does not render those forms. An Ingress has one shape this
chart can target uniformly. A `LoadBalancer`'s annotations and a tunnel's
whole setup are specific to your cloud or your tooling.

Left unset, both `signal.ingress.enabled` and `webhookUrl` keep today's
behavior — no change for a Pulse instance that already runs inside the
cluster.

If `global.agentops.networkPolicy` is on, also add the external Pulse's
address to `signal.webhookFrom` (raw `NetworkPolicyPeer` entries — an
`ipBlock` works here, not only a `namespaceSelector`):

```yaml
pulse:
  signal:
    webhookFrom:
      - ipBlock:
          cidr: 203.0.113.4/32
```

### TLS, and what a header can and cannot protect

**This chart accepts whichever scheme you supply, on every URL — it never
rejects `http://`.** A trusted internal network legitimately runs plain
HTTP, the same posture `home-assistant` already takes.

It DOES warn, in the install notes, wherever a connection carrying a real
credential is plain HTTP:

| Connection | What it carries | Risk if plain HTTP |
|---|---|---|
| `signal.ingress` with no TLS | the alert payload, any `webhookHeaders` | read in clear text by anyone on the path |
| `signal.defaultSource.baseUrl` | the Pulse API token, every registration call | a captured bearer token can simply be replayed |
| `mcpServer.backend` | the calling agent's own Pulse API token, every tool call | same — a bearer token is a static secret, not a signature |

**The inbound webhook itself is already the stronger case.** Pulse signs
each delivery (`X-Pulse-Signature`, HMAC-SHA256) rather than sending a
bearer token.

The signing secret itself never crosses the wire, only a per-request
digest of it. Even an eavesdropper who reads the signature learns nothing
reusable.

A bearer token, by contrast, IS the whole secret, sent again on every
call — exactly why the table above calls out `baseUrl` and the MCP
backend, not the webhook's own signature.

Terminate TLS on the Ingress with either form:

```sh
--set pulse.signal.ingress.tls.secretName=<secret>
# or
--set pulse.signal.ingress.tls.clusterIssuer=<cert-manager issuer>
```

### A static header for infra HMAC signing doesn't reach

`signal.defaultSource.webhookHeaders` registers a static header Pulse
attaches to every delivery — confirmed real against Pulse's own
`WebhookConfig.Headers` field. Useful when a WAF or gateway in front of
`signal.ingress` requires its own shared-secret header, independent of
whether `token`/`signingSecret` are also configured:

```yaml
pulse:
  signal:
    defaultSource:
      webhookHeaders:
        X-Gateway-Key: <shared secret your gateway checks>
```

This is a static value, not a signature — closer to the bearer-token risk
above than to HMAC signing. Pair it with TLS, same as the other two.

## What neither agent can do

Pulse's plan/approve/execute governed-action flow is **Pro-gated** and out of
reach on a Community-edition install. Neither profile claims it.

`pulse-operator`'s two tools — a maintenance window, a criticality tag — are
the only mutating actions either agent has. They are notes for a human or for
Pulse's own alerting, never a stand-in for an actual fix.

## Adopt it

Turn the bundle's own routes off and declare your own:

```sh
--set pulse.enabled=true --set pulse.pipelines.enabled=false
```

```powershell
--set pulse.enabled=true --set pulse.pipelines.enabled=false
```

### Your agent, Pulse's tools

```yaml
pipelines:
  - name: my-alert-agent
    profile: my-investigator        # YOUR profile, from your repository
    signalSources: [pulse-alerts]
    toolsets: [agentops-observe, pulse-observability]
    mcpConfigs: [pulse-observer-mcp]
    channels: [team-chat]
```

### Keep the split, change the surfaces

The privilege split is worth keeping even when the rest is yours — two
pipelines, two profiles, two tokens:

```yaml
pipelines:
  - name: estate
    profile: pulse-observer
    signalSources: [pulse-alerts, team-chat]
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

### Run yours beside the bundle's

**Sources are shareable.** Leave `pipelines.enabled=true` and claim
`pulse-alerts` from your own route as well.

Each admitted alert then opens **one conversation per claiming pipeline**,
each with its own agent and tools.

### What the bundle renders

<!-- generated: renders bundle=pulse -->
```text
# Always
SignalAdapter/pulse

# Ingest lane  (signal.defaultSource.enabled)
Secret/agentops-pulse-ingest
SignalSource/pulse-alerts

# Profiles  (profiles.observer.enabled / profiles.operator.enabled)
AgentProfile/pulse-observer
AgentProfile/pulse-operator

# MCP tooling  (mcp.observer.enabled / mcp.operator.enabled)
MCPConfig/pulse-observer-mcp
MCPConfig/pulse-operator-mcp
MCPToolset/pulse-admin
MCPToolset/pulse-observability
Secret/agentops-pulse-observer
Secret/agentops-pulse-operator

# MCP server  (mcpServer.enabled)
Deployment/agentops-mcp-pulse
Service/agentops-mcp-pulse

# Webhook exposure  (signal.ingress.enabled)
Ingress/agentops-signal-pulse

# Wiring  (pipelines.enabled)
Pipeline/pulse-observer
Pipeline/pulse-operator
ServiceAccount/agentops-pulse-observer
ServiceAccount/agentops-pulse-operator
```
<!-- /generated -->

- **The `SignalSource` appears only under `signal.defaultSource.enabled`.**
  The chart renders the adapter CR — the reconciler owns the workload, the
  webhook Service and its listen address.
- **No account is rendered for `mcp-pulse`** unless you name one. It reads
  Pulse's HTTP API, not the Kubernetes API, so it needs no RBAC.
- **Neither profile carries `allowedTools` or `mcp`.** What each agent may
  reach comes exclusively from the route binding it.

## Going deeper

How signals are grouped, cooled down and turned into conversations:
[concepts]({{ '/concepts.md' | relative_url }}).
