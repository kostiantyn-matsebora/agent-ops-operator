---
title: MCPConfig
permalink: /crds/mcp-config/
description: >-
  Every spec field of the MCPConfig kind — a named MCP server set compiled
  into the runtime's mcp.json.
---

An `MCPConfig` carries the servers — a named MCP server set compiled into
the runtime's `mcp.json`, or a complete hand-written one mounted raw.

Wiring binds it through `mcpConfigs` on `Pipeline`, `Coordinator` or
`AgentCapability`, applied in order with a later server key winning.

An instance sets exactly one of `servers`, `configMapRef` or `secretRef`.
The example below shows all three shapes side by side.

## Spec, at a glance

<!-- generated: crd-example kind=MCPConfig name=ha-api -->
```yaml
apiVersion: agentops.dev/v1alpha1
kind: MCPConfig
metadata:
  name: ha-api
spec:
  configMapRef:  # configMapRef / SecretRef mount a complete hand-written mcp.json (key …
    name: <name>
  secretRef:     # objectRef references another object by name (same namespace)
    name: <name>
  servers:       # servers defined inline
    <server>:
      type: sse   # sse | http | stdio
```
<!-- /generated -->

## Every field

<!-- generated: crd-fields kind=MCPConfig -->

| Field | Type | Required | Meaning |
|---|---|---|---|
| `configMapRef` | `object` |  | ConfigMapRef / SecretRef mount a complete hand-written mcp.json (key mcp.json) instead of compiling one. |
| `secretRef` | `object` |  | ObjectRef references another object by name (same namespace). |
| `servers` | `map[string]object` |  | Servers defined inline. |

<!-- /generated -->

Servers here, patterns in `MCPToolset` — composing the two halves is the
[Give your agent tools]({{ '/guides/toolsets/' | relative_url }}) guide.
