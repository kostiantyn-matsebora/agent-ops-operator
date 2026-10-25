---
title: Conversation
permalink: /crds/conversation/
description: >-
  Every spec field of the Conversation kind — one incident or task, an agent
  session, a serial input queue and a thread per bound channel.
---

A `Conversation` is one incident or task: an agent session, a serial input
queue, and one thread per bound channel, all mirroring the same exchange.

A signal or a `/<pipeline> <task>` address opens one, and the manager writes
it — the wiring's refs are snapshotted on at creation, so a later re-wire
never moves an inflight pod's privilege.

## Spec, at a glance

<!-- generated: crd-example kind=Conversation name=disk-pressure -->
```yaml
apiVersion: agentops.dev/v1alpha1
kind: Conversation
metadata:
  name: disk-pressure
spec:
  profileRef:                               # objectRef references another object by name (same namespace)
    name: <name>
  causedBy:                                 # set when this conversation was created by the `invoke` verb on another …
    entry: <entry>
    parent: <parent>
  channelRefs:                              # channelRefs — every listed channel mirrors the whole conversation (own …
  - name: <name>
  contextClaimName: <contextClaimName>      # contextClaimName / WorkspaceClaimName are the RESOLVED claims this …
  conversationId: <conversationId>          # the adopted initiationId of the signal that opened this conversation — …
  coordinatorRef:                           # coordinatorRef names the Coordinator this conversation is itself the …
    name: <name>
  escalationChannelRefs:                    # the SNAPSHOT of the originating Coordinator's own `channelRefs`, taken …
  - name: <name>
  inputs:                                   # the serial work queue — one inflight unit at a time, each entry pruned …
  - id: <id>
    type: task   # task | alert | reply | recurrence | job
  mcpConfigs:                               # toolingBinding binds MCP configs to a wiring: an ordered set of refs
    refs:
    - name: <name>
  originReader:                             # originReader records WHO started this conversation, so their own read …
    channel: <channel>
    key: <key>
  pipelineRef:                              # pipelineRef names the Pipeline that ORIGINATED this conversation
    name: <name>
  runtimeRef:                               # runtimeRef / ServiceAccountName are the originating Pipeline's …
    name: <name>
  serviceAccountName: <serviceAccountName>  # the resolved identity this conversation's runtime pods run as, …
  signal:                                   # what the ORIGINATING SIGNAL was, for attribution
    labels:
      <label>: <value>
    severity: critical   # critical | error | warning | info
    sourceRef:
      name: <name>
  title: <title>                            # bounded by MaxConversationTitle — see that constant's own comment for …
  toolsets:                                 # toolsets / MCPConfigs are the originating Pipeline's tooling bindings, …
    refs:
    - name: <name>
  workspaceClaimName: <workspaceClaimName>  # the resolved workspace claim, materialized at creation exactly as …
```
<!-- /generated -->

## Every field

<!-- generated: crd-fields kind=Conversation -->

| Field | Type | Required | Meaning |
|---|---|---|---|
| `profileRef` | `object` | **yes** | ObjectRef references another object by name (same namespace). |
| `profileRef.name` | `string` | **yes** | Name of the referenced object. |
| `causedBy` | `object` |  | Set when this conversation was created by the `invoke` verb on another conversation, rather than by a signal or a chat command. |
| `causedBy.entry` | `string` | **yes** | The name of the parent Coordinator's `agents[]` entry this conversation was invoked through. |
| `causedBy.parent` | `string` | **yes** | The parent conversation's name (same namespace). |
| `channelRefs` | `[]object` |  | ChannelRefs — every listed channel mirrors the whole conversation (own thread per channel, replies and acks fanned out). |
| `channelRefs[].name` | `string` | **yes** | Name of the referenced object. |
| `contextClaimName` | `string` |  | ContextClaimName / WorkspaceClaimName are the RESOLVED claims this conversation's runtime pods mount, snapshotted at creation exactly as RuntimeRef and ServiceAccountName are. |
| `conversationId` | `string` |  | The adopted initiationId of the signal that opened this conversation — the sender-minted correlation identity equality routing matches on. |
| `coordinatorRef` | `object` |  | CoordinatorRef names the Coordinator this conversation is itself the ROOT of — set only for a conversation the `invoke` verb (or an addressed/claimed signal) originated FROM a Coordinator, exactly as PipelineRef is set for one originated from a Pipeline. |
| `coordinatorRef.name` | `string` | **yes** | Name of the referenced object. |
| `escalationChannelRefs` | `[]object` |  | The SNAPSHOT of the originating Coordinator's own `channelRefs`, taken at creation — set only alongside CoordinatorRef, and only for an UNCAUSED root (a member never binds it). |
| `escalationChannelRefs[].name` | `string` | **yes** | Name of the referenced object. |
| `inputs` | `[]object` |  | The serial work queue — one inflight unit at a time, each entry pruned once status.runs[].inputs[] records it. |
| `inputs[].id` | `string` | **yes** | ID identifies this input within the conversation — the id status.runs[].inputIds echoes once the input is worked. |
| `inputs[].type` | `string` | **yes** | InputType classifies a work unit. One of `task`, `alert`, `reply`, `recurrence`, `job`. |
| `inputs[].agent` | `string` |  | DEPRECATED and no longer written. |
| `inputs[].origin` | `object` |  | Where this input came from. |
| `inputs[].origin.kind` | `string` | **yes** | OriginKind says HOW an input reached the manager. One of `signal`, `channel`, `member`. |
| `inputs[].origin.entry` | `string` |  | The parent Coordinator's `agents[]` entry name the edge was invoked through. |
| `inputs[].origin.name` | `string` |  | The SignalSource or Channel the input came from, or — for OriginMember — the conversation on the other side of the `causedBy` edge this input crossed. |
| `inputs[].origin.sender` | `string` |  | The transport-side identity that typed this input, when the serving adapter supplied one. |
| `inputs[].origin.signalKind` | `string` |  | The originating signal's lane (alert \| job \| task \| chat) for `signal` origins, empty otherwise. |
| `inputs[].payload` | `string` |  | The input's text, inline. |
| `inputs[].payloadRef` | `object` |  | ObjectRef references another object by name (same namespace). |
| `inputs[].payloadRef.name` | `string` | **yes** | Name of the referenced object. |
| `inputs[].receivedAt` | `string` |  | When the manager accepted this input into the queue. |
| `mcpConfigs` | `object` |  | ToolingBinding binds MCP configs to a wiring: an ordered set of refs. |
| `mcpConfigs.refs` | `[]object` | **yes** | Applied in order: MCP server keys are overlaid with the later ref winning a collision. |
| `mcpConfigs.refs[].name` | `string` | **yes** | Name of the referenced object. |
| `originReader` | `object` |  | OriginReader records WHO started this conversation, so their own read watermark can be stamped the moment their thread is created — the person who typed the request has by definition seen it, and presenting it back to them as unread before any answer exists is the one case the watermark rule gets plainly wrong. |
| `originReader.channel` | `string` | **yes** | Channel names the bound channel whose key space Key belongs to. |
| `originReader.key` | `string` | **yes** | The reporting adapter's opaque reader key, matched only within Channel's own key space. |
| `pipelineRef` | `object` |  | PipelineRef names the Pipeline that ORIGINATED this conversation. |
| `pipelineRef.name` | `string` | **yes** | Name of the referenced object. |
| `runtimeRef` | `object` |  | RuntimeRef / ServiceAccountName are the originating Pipeline's execution wiring, snapshotted at creation exactly as Toolsets and MCPConfigs are. |
| `runtimeRef.name` | `string` | **yes** | Name of the referenced object. |
| `serviceAccountName` | `string` |  | The resolved identity this conversation's runtime pods run as, snapshotted at creation exactly as RuntimeRef is — never hand-set. |
| `signal` | `object` |  | What the ORIGINATING SIGNAL was, for attribution. |
| `signal.labels` | `map[string]string` |  | The signal's own context, as the adapter sent them — opaque attribution only, rendered on signal cards. |
| `signal.severity` | `string` |  | The originating signal's own severity rating, written once at creation from the signal that opened the conversation and never rewritten by a later recurrence. One of `critical`, `error`, `warning`, `info`. |
| `signal.sourceRef` | `object` |  | The SignalSource this conversation came from. |
| `signal.sourceRef.name` | `string` | **yes** | Name of the referenced object. |
| `title` | `string` |  | Bounded by MaxConversationTitle — see that constant's own comment for why a cap belongs HERE despite the message-body rule against transport-shaped limits. |
| `toolsets` | `object` |  | Toolsets / MCPConfigs are the originating Pipeline's tooling bindings, snapshotted at creation like ChannelRefs and ProfileRef: materialized per-conversation state, NOT wiring — nothing sets them by hand, and re-wiring the pipeline affects only new conversations. |
| `toolsets.refs` | `[]object` | **yes** | Applied in order: tool lists concatenate with dedup, the first occurrence keeping its position. |
| `toolsets.refs[].name` | `string` | **yes** | Name of the referenced object. |
| `toolsets.mode` | `string` |  | Mode composes this binding's tools with the agent definition's: merge unions them (the agent keeps what it declared, the wiring adds), overwrite passes the wiring's alone (the agent's declaration does not apply to this route). One of `merge`, `overwrite`. Defaults to `merge`. |
| `workspaceClaimName` | `string` |  | The resolved workspace claim, materialized at creation exactly as ContextClaimName is — never hand-set, empty means ephemeral. |

<!-- /generated -->

Most of these fields are snapshots and provenance — what each means for
capacity and queueing stays in
[Concepts](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/concepts.md).
