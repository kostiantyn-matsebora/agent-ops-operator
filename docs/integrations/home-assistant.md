---
title: Home Assistant
permalink: /integrations/home-assistant/
description: >-
  A bundle that gives your house two agents — one that uses it, one that repairs
  it. Configure them, tune what reaches you, and bind their parts yourself.

next:
  eyebrow: Next
  title: Investigate a monitoring alert
  body: >-
    The same shape for Pulse — one source, two agents split by what they may
    write.
  url: /agent-ops-operator/integrations/pulse/
---

**Not everything worth watching is a cluster.** Home Assistant reports its own
failures in its log, its config entries, its repairs and its entities —
nobody reads those either.

This bundle reads all of them, and gives you **two agents**: one that can use
the house, one that can repair it.

![A Home Assistant record is re-checked by the rules, then opens a conversation on one of two routes — one that uses the house, one that repairs it.]({{ '/assets/img/integrations/home-assistant-light.svg' | relative_url }}){: .ao-diagram}

`signals/ha` conforms to the [signal adapter
specification]({{ '/contracts/signal-adapter/' | relative_url }}), version
2.2.0.

## What starts work

A log error that recurs, an integration still failing, a repair Home
Assistant filed, a device reporting a fault — each opens a conversation. A
one-off retry does not.

The log listing is polled every 15 seconds. Nothing in Home Assistant needs
enabling for it.

The adapter mints an `initiationId` of `<source>@<integration>`, so every
dwell cycle of the same failing integration lands on one conversation —
the manager just matches the id, however many log lines it sampled in
between.

Three more surfaces beside the log are on by default (config-entry state,
repairs, fault sensors), a fourth — a pending-update digest — is one switch
away. Filter any of them the same way you'd filter cluster events:

```yaml
logsAdapter:
  source:
    rules:
      - matchers: ['level="ERROR"']
        for: 5m
      - matchers: []
        action: drop
```

Full filtering vocabulary is the [cluster events
recipe]({{ '/integrations/kubernetes/' | relative_url }}#tune-what-reaches-you) —
same engine, same `rules`.

One addition: a `message` matcher, since Home Assistant records carry no
`reason` field. Every record also carries a `surface` label (`log`,
`config-entry`, `repair`, `sensor`, `update`) a rule can match on.

## What it may reach

| Agent | Can |
|---|---|
| the everyday one (`ha-user`) | act in the house through Assist intents — run a scene, dim lights, check a sensor. Reached by an ordinary chat message. |
| the repair one (`ha-operator`) | reconfigure integrations and read the logs, through a separate admin MCP server. Reached only by name, `/ha-ops <task>`. |

**Both hold different Home Assistant credentials — that's the real wall, not
a toolset setting.** Home Assistant has no read-only role, so neither
credential merely looks.

`ha-ops` can also web-search (built in, on by default). The everyday agent
can't.

> **Mint the operator token as a dedicated admin user, never your own.** The
> log lane drops any record naming its own user, so a token minted from the
> household owner silences every error that mentions them. It's also a
> double prerequisite: without it neither the repair route nor the log lane
> works (`system_log/list` is admin-only), and the failure reads like a
> network problem (`Ready=False, reason=Unreachable`) rather than a
> permissions one.
{: .ao-callout}

**Home Assistant 2026.9 renamed its Assist tools with a domain prefix**
(`HassTurnOn` → `intent__HassTurnOn`, etc.). Both toolsets list each tool
under both names, so the half your instance doesn't register just sits
inert.

An override of `mcp.toolsets.*.tools` must add the prefixed names itself, or
an agent on 2026.9 gets none at all, silently. Check what your instance
registers with a `tools/list` call to its `/mcp_server/sse` endpoint.

**The repair agent's admin server needs two flags together**
(`adminMcp.enabled` + `adminMcpServer.enabled` — neither renders alone). It
ships 62 of 78 possible tools, withholding restarts, backups, and deletes of
anything a person wrote.

It authenticates by URL-path secrecy, no token. On a shared cluster, set
`adminMcpServer.path` to a high-entropy value.

**A shell toolset reaches further than MCP does.** MCP tools are Assist
intents and can't touch configuration. A route that also binds a shell tool
reaches the *entire* REST API as that route's credential instead — logs,
config entries, reloads, service calls.

That's by design for the repair route (`pipelines.restAccess`, on for it and
off for the everyday one by default), not a gap to close.

## What it costs to turn on

```sh
helm upgrade --install agent-ops oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator \
  -n agent-ops --create-namespace \
  --set home-assistant.enabled=true \
  --set home-assistant.homeAssistant.endpoint=https://ha.example.org \
  --set home-assistant.homeAssistant.credentials.controlToken=<everyday-token> \
  --set home-assistant.homeAssistant.credentials.operatorToken=<admin-token> \
  --set home-assistant.wiringEnabled=true
```

```powershell
helm upgrade --install agent-ops oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator `
  -n agent-ops --create-namespace `
  --set home-assistant.enabled=true `
  --set home-assistant.homeAssistant.endpoint=https://ha.example.org `
  --set home-assistant.homeAssistant.credentials.controlToken=<everyday-token> `
  --set home-assistant.homeAssistant.credentials.operatorToken=<admin-token> `
  --set home-assistant.wiringEnabled=true
```

**Off by default, and demo mode never enables it.** Every component needs an
endpoint and a token no demo cluster has.

Prefer Secrets you already manage, or want to rename what the chart renders?
Every value here, and the rest the chart takes, is in
[`chart/charts/home-assistant/values.yaml`](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/chart/charts/home-assistant/values.yaml)
— `helm show values` is the full list, and this page does not repeat it.

## Adopt it

Turn the bundle's own routes off and wire your own, keeping the privilege
split if it's worth keeping:

```sh
--set home-assistant.enabled=true --set home-assistant.wiringEnabled=false
```

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
unaddressed message on that surface ambiguous, and grants nothing the route
doesn't already have.

One agent wanting both the house and your cluster just lists both bundles'
sources, toolsets and MCPConfigs on a single pipeline.

## Going deeper

How signals are verified, grouped and turned into conversations:
[concepts]({{ '/concepts.md' | relative_url }}).
