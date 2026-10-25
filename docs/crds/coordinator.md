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
  channelRefs:                              # the surfaces a ROOT conversation this Coordinator opens binds at …
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

One shape or the other: `capabilityRef` and the inline capability fields
are mutually exclusive by CEL. The sketch above shows both — a real CR
carries one.

## Every field

<!-- generated: crd-fields kind=Coordinator -->

| Field | Type | Required | Meaning |
|---|---|---|---|
| `agents` | `[]object` |  | Agents this Coordinator's own conversation may invoke. |
| `agents[].description` | `string` | **yes** | Description tells the COORDINATING AGENT what this entry IS and can do — its purpose and reach, what it cannot do, and what to hand it — so that a person's instruction matches it as readily as a signal does. |
| `agents[].name` | `string` | **yes** | How the `invoke` verb addresses this entry — unique within the Coordinator, never a Kubernetes identifier. |
| `agents[].capabilityRef` | `object` |  | CapabilityRef names the AgentCapability this entry invokes as an ordinary member — one with no `agents[]` of its own. |
| `agents[].capabilityRef.name` | `string` | **yes** | Name of the referenced object. |
| `agents[].coordinatorRef` | `object` |  | CoordinatorRef names another Coordinator this entry invokes AS A MEMBER. |
| `agents[].coordinatorRef.name` | `string` | **yes** | Name of the referenced object. |
| `capabilityRef` | `object` |  | AgentRef (`capabilityRef` in JSON) names an AgentCapability holding the coordinating agent's capability INSTEAD of inlining it above — mutually exclusive with the inline fields by CEL. |
| `capabilityRef.name` | `string` | **yes** | Name of the referenced object. |
| `channelRefs` | `[]object` |  | The surfaces a ROOT conversation this Coordinator opens binds at creation — reached only when the root escalates, and never bound by a caused member. |
| `channelRefs[].name` | `string` | **yes** | Name of the referenced object. |
| `limits` | `object` |  | Limits bounds this Coordinator's own conversation tree. |
| `limits.deadline` | `string` |  | How long after creation this Coordinator's own conversation may run before it is closed `budget-exceeded`. |
| `limits.maxAgents` | `integer` |  | The ceiling on how many agents this Coordinator's own conversation may invoke over its lifetime (`status.budget.agentsInvoked`). |
| `limits.maxTurns` | `integer` |  | The ceiling on `status.budget.turns`. |
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
| `signalSourceRefs` | `[]object` |  | The sources this Coordinator claims, exactly as a Pipeline's own — shareable, so a Pipeline and a Coordinator may claim the same source without conflict. |
| `signalSourceRefs[].name` | `string` | **yes** | Name of the referenced object. |
| `toolsets` | `object` |  | Toolsets binds MCPToolset CRs contributing to the allowlist of this capability's conversations, plus the mode composing them with what the AGENT'S OWN DEFINITION declares (merge unions, overwrite replaces). |
| `toolsets.refs` | `[]object` | **yes** | Applied in order: tool lists concatenate with dedup, the first occurrence keeping its position. |
| `toolsets.refs[].name` | `string` | **yes** | Name of the referenced object. |
| `toolsets.mode` | `string` |  | Mode composes this binding's tools with the agent definition's: merge unions them (the agent keeps what it declared, the wiring adds), overwrite passes the wiring's alone (the agent's declaration does not apply to this route). One of `merge`, `overwrite`. Defaults to `merge`. |

<!-- /generated -->

One example, one table — the composition worked end to end is the
[Coordinate agents]({{ '/guides/coordinate-agents/' | relative_url }}) guide.
