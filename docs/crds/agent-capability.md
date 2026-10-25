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
| `mcpConfigs.refs` | `[]object` | **yes** | Applied in order: MCP server keys are overlaid with the later ref winning a collision. |
| `mcpConfigs.refs[].name` | `string` | **yes** | Name of the referenced object. |
| `persistence` | `object` |  | Persistence declares WHERE this capability's conversations keep their state — the CONTEXT volume and the WORKSPACE volume, independently. |
| `persistence.context` | `object` |  | A conversation's accumulated context, the thing `runtimeContextId` is a handle into. |
| `persistence.context.accessModes` | `[]string` |  | AccessModes for that claim. |
| `persistence.context.claimName` | `string` |  | A PersistentVolumeClaim that ALREADY EXISTS. |
| `persistence.context.size` | `string` |  | Size requested by the claim the manager renders for VolumeName. |
| `persistence.context.storageClassName` | `string` |  | StorageClassName on that claim. |
| `persistence.context.volumeName` | `string` |  | A PersistentVolume the manager renders a claim on, bound to that volume by name with an EXPLICIT empty storage class — which is what disables dynamic provisioning. |
| `persistence.workspace` | `object` |  | The repository checkout, one subdirectory per conversation. |
| `persistence.workspace.accessModes` | `[]string` |  | AccessModes for that claim. |
| `persistence.workspace.claimName` | `string` |  | A PersistentVolumeClaim that ALREADY EXISTS. |
| `persistence.workspace.size` | `string` |  | Size requested by the claim the manager renders for VolumeName. |
| `persistence.workspace.storageClassName` | `string` |  | StorageClassName on that claim. |
| `persistence.workspace.volumeName` | `string` |  | A PersistentVolume the manager renders a claim on, bound to that volume by name with an EXPLICIT empty storage class — which is what disables dynamic provisioning. |
| `profileRef` | `object` |  | The agent this capability answers as. |
| `profileRef.name` | `string` | **yes** | Name of the referenced object. |
| `runtimeRef` | `object` |  | RuntimeRef selects the AgentRuntime executing this capability's conversations. |
| `runtimeRef.name` | `string` | **yes** | Name of the referenced object. |
| `serviceAccountName` | `string` |  | The identity the runtime executes under, OVERRIDING the AgentRuntime's own `serviceAccountName`. |
| `toolsets` | `object` |  | Toolsets binds MCPToolset CRs contributing to the allowlist of this capability's conversations, plus the mode composing them with what the AGENT'S OWN DEFINITION declares (merge unions, overwrite replaces). |
| `toolsets.refs` | `[]object` | **yes** | Applied in order: tool lists concatenate with dedup, the first occurrence keeping its position. |
| `toolsets.refs[].name` | `string` | **yes** | Name of the referenced object. |
| `toolsets.mode` | `string` |  | Mode composes this binding's tools with the agent definition's: merge unions them (the agent keeps what it declared, the wiring adds), overwrite passes the wiring's alone (the agent's declaration does not apply to this route). One of `merge`, `overwrite`. Defaults to `merge`. |

<!-- /generated -->

The same fields a `Pipeline` may inline — sharing one capability across
wirings is worked through in the
[Coordinate agents]({{ '/guides/coordinate-agents/' | relative_url }}) guide.
