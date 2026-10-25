---
title: Coordinator
permalink: /crds/coordinator/
description: >-
  Every spec field of the Coordinator kind — wiring for a composition, a
  capability plus the agents its root conversation may invoke.
---

A `Coordinator` is wiring for a composition: its own capability plus the
named `agents[]` its root conversation may invoke, bounded by per-level
limits.

Like a `Pipeline` it claims sources and binds channels — and only the
uncaused root ever reaches a human channel.

## Spec, at a glance

<!-- generated: crd-example kind=Coordinator name=k8s-triage -->
```yaml
apiVersion: agentops.dev/v1alpha1
kind: Coordinator
metadata:
  name: k8s-triage
spec:
  agents:                                   # agents this Coordinator's own conversation may invoke
  - description: <description>
    name: <name>
  capabilityRef:                            # agentRef (`capabilityRef` in JSON) names an AgentCapability holding …
    name: <name>
  channelRefs:                              # the surfaces a conversation this Coordinator opens binds at creation, …
  - name: <name>
  limits:                                   # limits bounds this Coordinator's own conversation tree
    deadline: <deadline>
    maxAgents: 1
    maxTurns: 1
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
  signalSourceRefs:                         # the sources this Coordinator claims, exactly as a Pipeline's own — …
  - name: <name>
  toolsets:                                 # toolsets binds MCPToolset CRs contributing to the allowlist of this …
    refs:
    - name: <name>
```
<!-- /generated -->

## Every field

<!-- generated: crd-fields kind=Coordinator -->

| Field | Type | Required | Meaning |
|---|---|---|---|
| `agents` | `[]object` |  | Agents this Coordinator's own conversation may invoke. |
| `capabilityRef` | `object` |  | AgentRef (`capabilityRef` in JSON) names an AgentCapability holding the coordinating agent's capability INSTEAD of inlining it above. |
| `channelRefs` | `[]object` |  | The surfaces a conversation this Coordinator opens binds at creation, unconditionally — see the type doc comment. |
| `limits` | `object` |  | Limits bounds this Coordinator's own conversation tree. |
| `mcpConfigs` | `object` |  | MCPConfigs binds MCPConfig CRs supplying this capability's MCP servers, overlaid per server key in ref order (later wins). |
| `persistence` | `object` |  | Persistence declares WHERE this capability's conversations keep their state — the CONTEXT volume and the WORKSPACE volume, independently. |
| `profileRef` | `object` |  | The agent this capability answers as. |
| `runtimeRef` | `object` |  | RuntimeRef selects the AgentRuntime executing this capability's conversations. |
| `serviceAccountName` | `string` |  | The identity the runtime executes under, OVERRIDING the AgentRuntime's own `serviceAccountName`. |
| `signalSourceRefs` | `[]object` |  | The sources this Coordinator claims, exactly as a Pipeline's own — shareable, so a Pipeline and a Coordinator may claim the same source without conflict. |
| `toolsets` | `object` |  | Toolsets binds MCPToolset CRs contributing to the allowlist of this capability's conversations, plus the mode composing them with what the AGENT'S OWN DEFINITION declares (merge unions, overwrite replaces). |

<!-- /generated -->

One example, one table — the composition worked end to end is the
[Coordinate agents]({{ '/guides/coordinate-agents/' | relative_url }}) guide.
