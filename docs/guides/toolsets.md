---
title: "Give your agent tools"
permalink: /guides/toolsets/
description: >-
  What an MCPToolset and an MCPConfig are, why they hang off the Pipeline and
  not the profile, and how to bind them without granting more than you meant.

next:
  eyebrow: Next
  title: "React to signals"
  body: >-
    Normalise your own transport into signals, and learn the loop hazard that
    made three independent breakers necessary.
  url: /agent-ops-operator/guides/signal-adapter/
---

**Capabilities are wiring.** An `MCPToolset` is a list of tool patterns — the
allowlist. An `MCPConfig` holds the MCP servers behind them. Both are bound from
the **Pipeline**, and an `AgentProfile` carries neither.

That is why the same agent, reached through two routes, has two different
reaches. What it may do is a property of the route it answered on, not of who
it is.

![An MCPConfig and an MCPToolset are bound from a Pipeline, and the runtime composes them with the agent definition's own tools to produce the allowlist.]({{ '/assets/img/guides/toolsets-light.svg' | relative_url }}){: .ao-diagram}

## Before you start

Binding capabilities is what you do when:

- Your agent answers and **can do nothing**. That is the default, and it is
  deliberate.
- It needs to reach a system — a cluster, a metrics store, a house.

It is **not** what you want when:

- You want the agent to answer somewhere new. That is `channelRefs`.
- You want to change **who** answers. That is `profileRef`.

{: .ao-callout}
> **This tier is pure YAML, and it grants more than an adapter's code ever
> could.** Nothing about being early in the order makes it small.

| Binding | What it grants |
|---|---|
| `agentops-shell` on a chat-addressable Pipeline | anyone who can type on that surface can run shell commands in the runtime pod |
| An admin-credentialled MCP server | every tool that server registers, at that credential's power |
| Both together | **the allowlist stops being the wall.** An agent with a shell can open a socket to a bound server and call anything on it |

`--allowedTools` configures a **cooperating** agent. For one that does not
cooperate there is
[`platform/egress-proxy`](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/platform/egress-proxy/),
an in-pod wall enabled by `global.agentops.runtimeDefaults.egressMediation.enabled`,
which is ON by default.

Review
[Capabilities are wiring](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/concepts.md#capabilities-are-wiring)
first.

## The overall shape

Four pieces, and the last one is not yours:

1. **An `MCPConfig`** — the servers, keyed by a name you choose.
2. **An `MCPToolset`** — the patterns. A pattern of `mcp__<key>__<tool>` refers
   to a server from step 1.
3. **`spec.toolsets` and `spec.mcpConfigs` on the Pipeline** — the binding, plus
   the `mode` that says how to compose.
4. **The runtime**, which combines your half with what the agent's own
   definition declares. It alone holds the checkout, so it alone can read that.

**Refs apply in order.** Tool lists concatenate with dedup, the first occurrence
keeping its position. Server keys overlay, and a later ref wins a collision.

## Declare the servers

<!-- generated: template kind=MCPConfig name=my-servers fields=servers.*.url,servers.*.headers comments=off -->
```yaml
apiVersion: agentops.dev/v1alpha1
kind: MCPConfig
metadata:
  name: my-servers
spec:
  servers:
    <server>:
      type: sse   # sse | http | stdio
      url: <url>
      headers:
      - name: <name>
```
<!-- /generated -->

Keys are yours to choose. `headers` takes a `valueFrom` so a credential reaches
the server without the manager ever reading it.

## Declare the allowlist

Patterns are **opaque strings** — an MCP namespace, a wildcard, or a built-in
tool name. Nothing validates them against a server.

<!-- generated: template kind=MCPToolset name=my-tools comments=off -->
```yaml
apiVersion: agentops.dev/v1alpha1
kind: MCPToolset
metadata:
  name: my-tools
spec:
  tools:
  - <tool>
```
<!-- /generated -->

The chart ships three built-in toolsets, split by risk:

| Toolset | Tools |
|---|---|
| `agentops-observe` | `Read` `Grep` `Glob` |
| `agentops-shell` | `Bash` |
| `agentops-edit` | `Edit` `Write` |

## Bind them to the Pipeline

<!-- generated: template kind=Pipeline name=my-route fields=toolsets.refs,toolsets.mode,mcpConfigs.refs comments=off -->
```yaml
apiVersion: agentops.dev/v1alpha1
kind: Pipeline
metadata:
  name: my-route
spec:
  profileRef:
    name: <name>
  toolsets:
    refs:
    - name: <name>
    mode: merge
  mcpConfigs:
    refs:
    - name: <name>
```
<!-- /generated -->

{: .ao-callout}
> **The profile carries no capabilities.** No `allowedTools`, no `mcp`. Looking
> for them there is the mistake that deleted this field once already.

## Choose the mode

`spec.toolsets.mode` composes your bound toolsets against the **`tools:`
frontmatter of `.claude/agents/<agent>.md`** in the profile's repository.

| Mode | Result |
|---|---|
| `merge` (default) | the union, the definition's own entries keeping their position |
| `overwrite` | the bound toolsets alone |

Two consequences worth knowing before you debug one of them:

- **A profile with no repository has no definition** to compose against, so
  `merge` degrades to the bound toolsets.
- **An empty allowlist means empty.** No fallback tool is substituted, and the
  run proceeds with nothing granted.

**`spec.mcpConfigs` has no mode.** A definition declares no MCP servers, so
there would be one behaviour wearing two names.

## Split by privilege

The chart's `home-assistant` splits its own tools in two, and the split is the
lesson. Observation:

<!-- generated: example preset=tier2 kind=MCPToolset name=ha-observability -->
```yaml
# Source: agent-ops-operator/charts/home-assistant/templates/mcp.yaml
apiVersion: agentops.dev/v1alpha1
kind: MCPToolset
metadata:
  name: ha-observability
  namespace: agent-ops
  labels:
    app.kubernetes.io/name: agentops-home-assistant
spec:
  tools:
    - mcp__homeassistant__GetLiveContext
    - mcp__homeassistant__HassGetState
    - mcp__homeassistant__HassGetWeather
    - mcp__homeassistant__HassGetCurrentDate
    - mcp__homeassistant__HassGetCurrentTime
    - mcp__homeassistant__todo_get_items
```
<!-- /generated -->

Action:

<!-- generated: example preset=tier2 kind=MCPToolset name=ha-actions -->
```yaml
# Source: agent-ops-operator/charts/home-assistant/templates/mcp.yaml
apiVersion: agentops.dev/v1alpha1
kind: MCPToolset
metadata:
  name: ha-actions
  namespace: agent-ops
  labels:
    app.kubernetes.io/name: agentops-home-assistant
spec:
  tools:
    - mcp__homeassistant__HassTurnOn
    - mcp__homeassistant__HassTurnOff
    - mcp__homeassistant__HassLightSet
    - mcp__homeassistant__HassSetPosition
    - mcp__homeassistant__HassClimateSetTemperature
    - mcp__homeassistant__HassMediaPause
    - mcp__homeassistant__HassMediaUnpause
    - mcp__homeassistant__HassMediaNext
    - mcp__homeassistant__HassVacuumStart
    - mcp__homeassistant__HassVacuumReturnToBase
    - mcp__homeassistant__HassBroadcast
    - mcp__homeassistant__HassCancelAllTimers
    - mcp__homeassistant__HassListAddItem
```
<!-- /generated -->

One server serves both:

<!-- generated: example preset=tier2 kind=MCPConfig name=ha-api -->
```yaml
# Source: agent-ops-operator/charts/home-assistant/templates/mcp.yaml
apiVersion: agentops.dev/v1alpha1
kind: MCPConfig
metadata:
  name: ha-api
  namespace: agent-ops
  labels:
    app.kubernetes.io/name: agentops-home-assistant
spec:
  servers:
    homeassistant:
      type: sse
      url: "https://ha.example.org/mcp_server/sse"
      headers:
        - name: Authorization
          valueFrom:
            secretKeyRef:
              name: agentops-ha-operator
              key: authorization
```
<!-- /generated -->

Then two routes at two privilege levels. The one anyone may reach:

<!-- generated: example preset=tier2 kind=Pipeline name=ha-control -->
```yaml
# Source: agent-ops-operator/charts/home-assistant/templates/pipelines.yaml
apiVersion: agentops.dev/v1alpha1
kind: Pipeline
metadata:
  name: ha-control
  namespace: agent-ops
  labels:
    app.kubernetes.io/name: agentops-home-assistant
spec:
  # Display only: how this route is recognised in a chat command menu or the
  # console's typeahead. Nothing routes on it.
  icon: "aops:home"
  profileRef:
    name: ha-user
  # Declared, not inherited: profiles carry no capabilities and nothing supplies
  # a default, so this stanza IS the agent's allowlist.
  toolsets:
    refs:
      - name: agentops-observe
      - name: ha-observability
      - name: ha-actions
  mcpConfigs:
    refs:
      - name: ha-api
```
<!-- /generated -->

And the one that also gets a shell:

<!-- generated: example preset=tier2 kind=Pipeline name=ha-ops -->
```yaml
# Source: agent-ops-operator/charts/home-assistant/templates/pipelines.yaml
apiVersion: agentops.dev/v1alpha1
kind: Pipeline
metadata:
  name: ha-ops
  namespace: agent-ops
  labels:
    app.kubernetes.io/name: agentops-home-assistant
spec:
  # Display only: how this route is recognised in a chat command menu or the
  # console's typeahead. Nothing routes on it.
  icon: "aops:operate"
  profileRef:
    name: ha-operator
  # Wiring is pipeline-only: without a claim a source reports Wired=False and
  # DROPS every signal it admits.
  signalSourceRefs:
    - name: ha-logs
  # Declared, not inherited: profiles carry no capabilities and nothing supplies
  # a default, so this stanza IS the agent's allowlist.
  toolsets:
    refs:
      - name: agentops-observe
      - name: ha-observability
      - name: ha-actions
      - name: agentops-shell
  mcpConfigs:
    refs:
      - name: ha-api
```
<!-- /generated -->

**Read the difference.** `ha-ops` adds `agentops-shell` and claims a log source.
It lists **no chat source**, so the only way to reach it is `/ha-ops <task>`,
typed by name. `ha-control` has no shell and claims nothing.

## What comes next

1. **[React to signals]({{ '/guides/signal-adapter/' | relative_url }})**
   — originate conversations from a transport nothing here serves yet.
2. **[MCPToolset and MCPConfig fields](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/cr-reference.md#mcptoolset)**
   — the full surface of both kinds.
3. **[Mediate the agent's egress](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/adr/0001-bound-component-reach.md)**
   — when the allowlist is not enough of a wall.
