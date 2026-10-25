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
| `configMapRef.name` | `string` | **yes** | Name of the referenced object. |
| `secretRef` | `object` |  | ObjectRef references another object by name (same namespace). |
| `secretRef.name` | `string` | **yes** | Name of the referenced object. |
| `servers` | `map[string]object` |  | Servers defined inline. |
| `servers.<name>.type` | `string` | **yes** | Type of transport: sse, http or stdio. One of `sse`, `http`, `stdio`. |
| `servers.<name>.args` | `[]string` |  | The command's arguments (stdio transport). |
| `servers.<name>.command` | `string` |  | Command and Args for stdio transport. |
| `servers.<name>.env` | `[]object` |  | Env for stdio servers. |
| `servers.<name>.env[].name` | `string` | **yes** | Name of the environment variable. |
| `servers.<name>.env[].value` | `string` |  | Variable references $(VAR_NAME) are expanded using the previously defined environment variables in the container and any service environment variables. |
| `servers.<name>.env[].valueFrom` | `object` |  | Source for the environment variable's value. The standard Kubernetes `EnvVarSource` -- set exactly one of `configMapKeyRef`, `secretKeyRef`, `fieldRef` or `resourceFieldRef`, each naming its source object and key. |
| `servers.<name>.headers` | `[]object` |  | Headers to send (sse/http). |
| `servers.<name>.headers[].name` | `string` | **yes** | The header or variable name the pair sets. |
| `servers.<name>.headers[].value` | `string` |  | The literal value. |
| `servers.<name>.headers[].valueFrom` | `object` |  | EnvVarSource represents a source for the value of an EnvVar. The standard Kubernetes `EnvVarSource` -- set exactly one of `configMapKeyRef`, `secretKeyRef`, `fieldRef` or `resourceFieldRef`, each naming its source object and key. |
| `servers.<name>.url` | `string` |  | URL for sse/http transports. |

<!-- /generated -->

Servers here, patterns in `MCPToolset` — composing the two halves is the
[Give your agent tools]({{ '/guides/toolsets/' | relative_url }}) guide.
