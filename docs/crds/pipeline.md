---
title: Pipeline
permalink: /crds/pipeline/
description: >-
  Every spec field of the Pipeline kind — the wiring binding sources,
  channels and a capability into one declared flow.
---

A `Pipeline` is the wiring: the signal sources it claims, the channels every
conversation is mirrored on, and the capability that answers — profile,
runtime, identity, tools and persistence, inline or through `capabilityRef`.

A signal admitted by a claimed source starts a conversation through it.

## Spec, at a glance

<!-- generated: crd-example kind=Pipeline name=k8s-observe -->
```yaml
apiVersion: agentops.dev/v1alpha1
kind: Pipeline
metadata:
  name: k8s-observe
spec:
  capabilityRef:                            # capabilityRef names an AgentCapability holding this pipeline's …
    name: <name>
  channelRefs:                              # every conversation of this pipeline is mirrored on all of these surfaces
  - name: <name>
  icon: <icon>                              # how this Pipeline is RECOGNISED in a list of them
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
  signalSourceRefs:                         # the sources feeding this pipeline
  - name: <name>
  toolsets:                                 # toolsets binds MCPToolset CRs contributing to the allowlist of this …
    refs:
    - name: <name>
```
<!-- /generated -->

## Every field

<!-- generated: crd-fields kind=Pipeline -->

| Field | Type | Required | Meaning |
|---|---|---|---|
| `capabilityRef` | `object` |  | CapabilityRef names an AgentCapability holding this pipeline's capability INSTEAD of inlining it above — mutually exclusive with the inline fields by CEL. |
| `channelRefs` | `[]object` |  | Every conversation of this pipeline is mirrored on all of these surfaces. |
| `icon` | `string` |  | How this Pipeline is RECOGNISED in a list of them. |
| `mcpConfigs` | `object` |  | MCPConfigs binds MCPConfig CRs supplying this capability's MCP servers, overlaid per server key in ref order (later wins). |
| `persistence` | `object` |  | Persistence declares WHERE this capability's conversations keep their state — the CONTEXT volume and the WORKSPACE volume, independently. |
| `profileRef` | `object` |  | The agent this capability answers as. |
| `runtimeRef` | `object` |  | RuntimeRef selects the AgentRuntime executing this capability's conversations. |
| `serviceAccountName` | `string` |  | The identity the runtime executes under, OVERRIDING the AgentRuntime's own `serviceAccountName`. |
| `signalSourceRefs` | `[]object` |  | The sources feeding this pipeline. |
| `toolsets` | `object` |  | Toolsets binds MCPToolset CRs contributing to the allowlist of this capability's conversations, plus the mode composing them with what the AGENT'S OWN DEFINITION declares (merge unions, overwrite replaces). |

<!-- /generated -->

One example, one table — the worked route, with the prose around it, is the
[Put an agent to work]({{ '/guides/pipeline/' | relative_url }}) guide.
