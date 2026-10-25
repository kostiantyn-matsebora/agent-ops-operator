---
title: AgentCapability
permalink: /crds/agent-capability/
description: >-
  Every spec field of the AgentCapability kind — the capability factored
  out, declared once and referenced by Pipelines and Coordinators.
---

An `AgentCapability` is the capability factored out — profile, runtime,
identity, tools and persistence, declared once and referenced by any
`Pipeline` or `Coordinator` through `capabilityRef`.

Unwired it is inert: nothing reaches it until wiring names it.

## Spec, at a glance

<!-- generated: crd-example kind=AgentCapability name=log-analyzer -->
```yaml
apiVersion: agentops.dev/v1alpha1
kind: AgentCapability
metadata:
  name: log-analyzer
spec:
  mcpConfigs:                               # MCPConfigs binds MCPConfig CRs supplying this capability's MCP …
    refs:
    - name: <name>
  persistence:                              # persistence declares WHERE this capability's conversations keep their …
    context:
      accessModes:
      - <accessMode>
      claimName: <claimName>
      size: <size>
      storageClassName: <storageClassName>
      volumeName: <volumeName>
    workspace:
      accessModes:
      - <accessMode>
      claimName: <claimName>
      size: <size>
      storageClassName: <storageClassName>
      volumeName: <volumeName>
  profileRef:                               # the agent this capability answers as
    name: <name>
  runtimeRef:                               # runtimeRef selects the AgentRuntime executing this capability's …
    name: <name>
  serviceAccountName: <serviceAccountName>  # the identity the runtime executes under, OVERRIDING the AgentRuntime's …
  toolsets:                                 # toolsets binds MCPToolset CRs contributing to the allowlist of this …
    refs:
    - name: <name>
```
<!-- /generated -->

## Every field

<!-- generated: crd-fields kind=AgentCapability -->

| Field | Type | Required | Meaning |
|---|---|---|---|
| `mcpConfigs` | `object` |  | MCPConfigs binds MCPConfig CRs supplying this capability's MCP servers, overlaid per server key in ref order (later wins). |
| `persistence` | `object` |  | Persistence declares WHERE this capability's conversations keep their state — the CONTEXT volume and the WORKSPACE volume, independently. |
| `profileRef` | `object` |  | The agent this capability answers as. |
| `runtimeRef` | `object` |  | RuntimeRef selects the AgentRuntime executing this capability's conversations. |
| `serviceAccountName` | `string` |  | The identity the runtime executes under, OVERRIDING the AgentRuntime's own `serviceAccountName`. |
| `toolsets` | `object` |  | Toolsets binds MCPToolset CRs contributing to the allowlist of this capability's conversations, plus the mode composing them with what the AGENT'S OWN DEFINITION declares (merge unions, overwrite replaces). |

<!-- /generated -->

The same fields a `Pipeline` may inline — sharing one capability across
wirings is worked through in the
[Coordinate agents]({{ '/guides/coordinate-agents/' | relative_url }}) guide.
