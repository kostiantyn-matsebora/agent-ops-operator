---
title: MCPToolset
permalink: /crds/mcp-toolset/
description: >-
  Every spec field of the MCPToolset kind — a pure list of tool patterns,
  half the allowlist a conversation runs under.
---

An `MCPToolset` is a pure list of tool patterns — half the allowlist, which
the runtime composes with the agent definition's own `tools:`.

`Pipeline`, `Coordinator` and `AgentCapability` bind one through `toolsets`,
and the servers live in `MCPConfig`, never here.

## Spec, at a glance

<!-- generated: crd-example kind=MCPToolset name=ha-observability -->
```yaml
apiVersion: agentops.dev/v1alpha1
kind: MCPToolset
metadata:
  name: ha-observability
spec:
  tools:  # tools this toolset grants: MCP namespaces ("mcp__victorialogs__*") or …
  - <tool>
```
<!-- /generated -->

## Every field

<!-- generated: crd-fields kind=MCPToolset -->

| Field | Type | Required | Meaning |
|---|---|---|---|
| `tools` | `[]string` | **yes** | Tools this toolset grants: MCP namespaces ("mcp__victorialogs__*") or built-in tool names ("Bash"). |

<!-- /generated -->

One field, one table — granting tools and servers together is the
[Give your agent tools]({{ '/guides/toolsets/' | relative_url }}) guide.
