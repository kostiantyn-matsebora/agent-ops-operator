---
title: Home Assistant
permalink: /integrations/home-assistant/
description: >-
  A bundle that gives your house two agents — one that uses it, one that repairs
  it. Configure them, tune what reaches you, and bind their parts yourself.

next:
  eyebrow: Next
  title: Ask it from chat
  body: >-
    The same shape for Telegram — where a person types, and where every agent
    answers back.
  url: /agent-ops-operator/integrations/telegram/
---

**Not everything worth watching is a cluster.** Home Assistant logs its own
failures — an integration that stopped setting up, a device that will not
answer, a token that expired — and nobody reads those either.

This bundle reads them, and gives you **two agents**: one that can use the
house, one that can repair it.

![A Home Assistant log record is re-checked by the rules, and opens a conversation on one of two routes: one that uses the house, one that repairs it.]({{ '/assets/img/integrations/home-assistant-light.svg' | relative_url }}){: .ao-diagram}

## What you get

| You get | Which means |
|---|---|
| **An everyday agent** | Ask it to run a scene, dim the lights, check a sensor. Reached by an ordinary chat message. |
| **A repair agent** | Reached by name, `/ha-ops <task>`, and never by accident. It reconfigures integrations and reads the logs. |
| **Log failures that reach you** | A record that recurs, or an integration still failing, opens a conversation. A one-off retry does not. |
| **A split you can actually rely on** | The two agents hold **different Home Assistant credentials**, so the boundary is the token, not a setting. |

**The split is use versus fix, not read versus act.** Home Assistant has no
read-only role, so neither credential merely looks. Both agents act — what
separates them is the job and the reach it needs.

## Turn it on

```sh
helm upgrade --install agent-ops oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator \
  -n agent-ops --create-namespace \
  --set home-assistant.enabled=true \
  --set home-assistant.homeAssistant.endpoint=https://ha.example.org \
  --set home-assistant.homeAssistant.credentials.controlToken=<everyday-token> \
  --set home-assistant.homeAssistant.credentials.operatorToken=<admin-token> \
  --set home-assistant.pipelines.enabled=true
```

```powershell
helm upgrade --install agent-ops oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator `
  -n agent-ops --create-namespace `
  --set home-assistant.enabled=true `
  --set home-assistant.homeAssistant.endpoint=https://ha.example.org `
  --set home-assistant.homeAssistant.credentials.controlToken=<everyday-token> `
  --set home-assistant.homeAssistant.credentials.operatorToken=<admin-token> `
  --set home-assistant.pipelines.enabled=true
```

**Off by default, and demo mode never enables it.** Every component needs an
endpoint and a token no demo cluster has.

### The one decision: which two users to mint

| Token | Belongs to | Mint it as |
|---|---|---|
| the **control** token | the everyday agent, and the MCP path | the account that **uses** the house |
| the **operator** token | the repair agent, and the log lane | an **admin** Home Assistant user |

**A token inherits its user's rights, and no toolset can narrow a credential.**
That is why the split is two users rather than two settings.

> **The operator token is a prerequisite twice over.** Without it neither the
> repair agent nor its route renders — and the **log lane needs it too**, because
> `subscribe_events` is admin-only. A control token connects, passes auth, and is
> then refused the subscription, which surfaces as `Ready=False,
> reason=Unreachable` and reads like a network problem.
{: .ao-callout}

Prefer Secrets you already manage? Use `credentials.controlSecret` and
`.operatorSecret` instead of the inline tokens, and `credentials.ingestSecret`
where the log lane should use a third.

### Where the answers turn up

**Nowhere you can see, until you say where.** Neither route binds a channel:

```sh
--set home-assistant.pipelines.channels={console}
```

```powershell
--set home-assistant.pipelines.channels={console}
```

Every answer is on the CR regardless:

```sh
kubectl get conversations -o jsonpath='{.items[*].status.runs[*].result}'
```

```powershell
kubectl get conversations -o jsonpath='{.items[*].status.runs[*].result}'
```

### How each agent is reached

| Agent | Reached by |
|---|---|
| the everyday one | an ordinary chat message on a surface it claims |
| the repair one | `/ha-ops <task>`, **by name** — it claims no chat source, so nothing reaches it by accident |

## The values you set

Everything below is under `home-assistant:`.

### What exists

| Value | Default | Set it to |
|---|---|---|
| `enabled` | `false` | `true` to render the bundle |
| `homeAssistant.endpoint` | **empty, required** | your Home Assistant URL |
| `logsAdapter.enabled` | `true` | `false` for tools without the log lane |
| `logsAdapter.source.create` | `true` | `false` to declare your own `SignalSource` |
| `mcp.enabled` | `true` | `false` for agents with no reach into the house |
| `mcp.toolsets.actions.enabled` | `true` | `false` for an install that may look and never touch |
| `adminMcp.enabled` | `false` | `true` **with** `adminMcpServer.enabled` — see below |
| `profiles.user.enabled` | `true` | `false` to drop the everyday agent |
| `profiles.ops.enabled` | `true` | `false` to drop the repair agent |
| `pipelines.enabled` | `false` | `true` for working routes |

### What things are called

Rename any of these and your own pipelines reference the new name.

```yaml
home-assistant:
  logsAdapter:
    source:
      name: ha-logs                     # the SignalSource your pipeline claims
  profiles:
    user:
      name: ha-user                     # the everyday AgentProfile
    ops:
      name: ha-operator                 # the repair AgentProfile
  mcp:
    name: ha-api                        # the everyday MCPConfig
    toolsets:
      observe:
        name: ha-observability          # state, weather, list contents
      actions:
        name: ha-actions                # lights, climate, media, scenes
  adminMcp:
    toolset:
      name: ha-admin                    # the repair toolset
  pipelines:
    control:
      name: ha-control                  # the everyday route
    ops:
      name: ha-ops                      # the repair route, and the command
                                        # that reaches it
```

### Repairing the house is one switch made of two flags

Neither renders alone — the config refuses without a server to reach, and the
server's workload is gated on the config:

```sh
--set home-assistant.adminMcp.enabled=true \
--set home-assistant.adminMcpServer.enabled=true
```

```powershell
--set home-assistant.adminMcp.enabled=true `
--set home-assistant.adminMcpServer.enabled=true
```

**Of the 78 tools that server registers, 52 ship.** The 26 withheld ones restart
Home Assistant, manage backups, delete registry objects or install software.

Add any of them back by restating `adminMcp.toolset.tools` — Helm replaces lists
rather than merging them.

> **The admin server authenticates by URL-path secrecy.** There is no token on
> the MCP endpoint, so anything that can reach the Service and knows the path can
> drive the house. The Service is ClusterIP, which bounds that to the cluster —
> on a shared one, set `adminMcpServer.path` to a high-entropy value.
{: .ao-callout}

Already run an MCP server for your house — a HACS component, say? Point
`adminMcp.url` at it and leave `adminMcpServer.enabled` off.

## Tune what reaches you

Everything below goes under `home-assistant.logsAdapter.source`. The vocabulary
is the [cluster events
one]({{ '/integrations/kubernetes/' | relative_url }}#tune-what-reaches-you) —
what differs is listed after the recipes.

### "Only tell me about integrations that actually broke"

```yaml
logsAdapter:
  source:
    rules:
      - matchers: ['level="ERROR"']
        for: 5m
      - matchers: []
        action: drop
```

`for` holds, then asks Home Assistant whether the integration is still failing.
Recovered means you never hear about it.

### "This one integration is always noisy"

```yaml
      - matchers: ['logger=~"homeassistant.components.zwave_js.*"']
        action: drop
```

### "Ignore anything that says it will retry"

```yaml
      - matchers: ['message=~".*Retrying.*"']
        action: drop
```

**`message` is the one matcher this lane adds.** Home Assistant records carry no
`reason` field, so level and logger alone cannot tell "will retry" from
"credentials rejected".

It is **match-only** and never becomes a label — a label carrying free text would
key conversation grouping on the exact wording.

### What differs from the cluster lane

| | |
|---|---|
| **`message` matchers** | available here, and nowhere else |
| **The re-check** | asks the integration's config entry state, then falls back to *was it still recurring as the window closed* — the last third of the wait, never under thirty seconds. A blip that logged for thirty seconds and stopped is churn |
| **No time axis** | `timeIntervals` and `muteTimeIntervals` are not implemented. An unknown key is a config error, not a window that silently never fires |

## Adopt it

Turn the bundle's own routes off and declare your own:

```sh
--set home-assistant.enabled=true --set home-assistant.pipelines.enabled=false
```

```powershell
--set home-assistant.enabled=true --set home-assistant.pipelines.enabled=false
```

### Your agent, your house

```yaml
pipelines:
  - name: my-house-agent
    profile: my-assistant         # YOUR profile, from your repository
    signalSources: [ha-logs, team-chat]
    toolsets: [agentops-observe, ha-observability, ha-actions]
    mcpConfigs: [ha-api]
    channels: [team-chat]
```

### One agent for the house *and* your cluster

```yaml
pipelines:
  - name: everything
    profile: my-assistant
    signalSources: [ha-logs, cluster-events, team-chat]
    toolsets: [agentops-observe, ha-observability, k8s-observability]
    mcpConfigs: [ha-api, k8s-api]
    channels: [team-chat]
```

### Keep the split, change the surfaces

The privilege split is worth keeping even when the rest is yours — two
pipelines, two profiles, two credentials:

```yaml
pipelines:
  - name: house
    profile: ha-user
    signalSources: [family-chat]
    toolsets: [agentops-observe, ha-observability, ha-actions]
    mcpConfigs: [ha-api]
    channels: [family-chat]
  - name: house-ops                # reached by /house-ops, claims no chat source
    profile: ha-operator
    signalSources: [ha-logs]
    toolsets: [agentops-observe, ha-observability, ha-actions, ha-admin, agentops-shell]
    mcpConfigs: [ha-api, ha-admin-api]
    channels: [family-chat]
```

**Give the repair route no chat source.** Listing one there makes every
unaddressed message on that surface ambiguous, and grants that route nothing it
does not already have.

### What the bundle renders

<!-- generated: renders bundle=home-assistant -->
```text
# Always
Secret/agentops-ha-operator

# Ingest lane  (logsAdapter.enabled)
SignalAdapter/home-assistant
SignalSource/ha-logs

# MCP tooling  (mcp.enabled)
MCPConfig/ha-api
MCPToolset/ha-actions
MCPToolset/ha-observability

# Profiles  (profiles.user.enabled / profiles.ops.enabled)
AgentProfile/ha-operator
AgentProfile/ha-user

# Admin MCP  (adminMcp.enabled / adminMcpServer.enabled)
Deployment/agentops-mcp-ha
MCPConfig/ha-admin-api
MCPToolset/ha-admin
Service/agentops-mcp-ha

# Wiring  (pipelines.enabled)
Pipeline/ha-control
Pipeline/ha-ops
```
<!-- /generated -->

- **The `SignalSource` appears only under `source.create`.**
- **The everyday agent renders only when the house is reachable** — a profile
  meant to use the house with no way to reach it could only apologise.
- **Neither route carries Kubernetes RBAC.** Neither touches that API.

## The reach is wider than the toolsets

The toolsets bound a route's **MCP** path. They are not the whole boundary.

Each agent's token sits in its profile's environment, so a route that also binds
a shell toolset reaches the entire Home Assistant REST API **as that user**.

| Path | Reach | Can it repair? |
|---|---|---|
| MCP | the tools the bound toolset names | no — Assist intents control devices and touch no configuration |
| REST, via a shell tool | everything that route's credential may do | yes — logs, config entries, reloads, service calls |

**That is not a leak to engineer away.** It is how the repair agent does its job,
and the reason the everyday one is given no shell.

`pipelines.restAccess` decides who gets it: unset means on for the repair route
and off for the everyday one, `true` means both, `false` means neither — which
leaves the repair lane mostly decorative.

## Going deeper

How signals are verified, grouped and turned into conversations:
[concepts]({{ '/concepts.md' | relative_url }}).
