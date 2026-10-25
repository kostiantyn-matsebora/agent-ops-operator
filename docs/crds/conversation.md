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
  profileRef:                           # objectRef references another object by name (same namespace)
    name: <name>
  causedBy:                             # set when this conversation was created by the `invoke` verb on another …
    entry: <entry>
    parent: <parent>
  channelRefs:                          # channelRefs — every listed channel mirrors the whole conversation (own …
  - name: <name>
  contextClaimName: <contextClaimName>  # contextClaimName / WorkspaceClaimName are the RESOLVED claims this …
  conversationId: <conversationId>      # the adopted initiationId of the signal that opened this conversation — …
  coordinatorRef:                       # coordinatorRef names the Coordinator this conversation is itself the …
    name: <name>
  escalationChannelRefs:                # the SNAPSHOT of the originating Coordinator's own `channelRefs`, taken …
  - name: <name>
  inputs:
  - id: <id>
    type: task   # task | alert | reply | recurrence | job
  mcpConfigs:                           # toolingBinding binds MCP configs to a wiring: an ordered set of refs
    refs:
    - name: <name>
  originReader:                         # originReader records WHO started this conversation, so their own read …
    channel: <channel>
    key: <key>
  pipelineRef:                          # pipelineRef names the Pipeline that ORIGINATED this conversation
    name: <name>
  runtimeRef:                           # runtimeRef / ServiceAccountName are the originating Pipeline's …
    name: <name>
  serviceAccountName: <serviceAccountName>
  signal:                               # what the ORIGINATING SIGNAL was, for attribution
    labels:
      <label>: <value>
    severity: critical   # critical | error | warning | info
    sourceRef:
      name: <name>
  title: <title>                        # bounded by MaxConversationTitle — see that constant's own comment for …
  toolsets:                             # toolsets / MCPConfigs are the originating Pipeline's tooling bindings, …
    refs:
    - name: <name>
  workspaceClaimName: <workspaceClaimName>
```
<!-- /generated -->

## Every field

<!-- generated: crd-fields kind=Conversation -->

| Field | Type | Required | Meaning |
|---|---|---|---|
| `profileRef` | `object` | **yes** | ObjectRef references another object by name (same namespace). |
| `causedBy` | `object` |  | Set when this conversation was created by the `invoke` verb on another conversation, rather than by a signal or a chat command. |
| `channelRefs` | `[]object` |  | ChannelRefs — every listed channel mirrors the whole conversation (own thread per channel, replies and acks fanned out). |
| `contextClaimName` | `string` |  | ContextClaimName / WorkspaceClaimName are the RESOLVED claims this conversation's runtime pods mount, snapshotted at creation exactly as RuntimeRef and ServiceAccountName are. |
| `conversationId` | `string` |  | The adopted initiationId of the signal that opened this conversation — the sender-minted correlation identity equality routing matches on. |
| `coordinatorRef` | `object` |  | CoordinatorRef names the Coordinator this conversation is itself the ROOT of — set only for a conversation the `invoke` verb (or an addressed/claimed signal) originated FROM a Coordinator, exactly as PipelineRef is set for one originated from a Pipeline. |
| `escalationChannelRefs` | `[]object` |  | The SNAPSHOT of the originating Coordinator's own `channelRefs`, taken at creation — set only alongside CoordinatorRef, and only for an UNCAUSED root (a member never binds it). |
| `inputs` | `[]object` |  |  |
| `mcpConfigs` | `object` |  | ToolingBinding binds MCP configs to a wiring: an ordered set of refs. |
| `originReader` | `object` |  | OriginReader records WHO started this conversation, so their own read watermark can be stamped the moment their thread is created — the person who typed the request has by definition seen it, and presenting it back to them as unread before any answer exists is the one case the watermark rule gets plainly wrong. |
| `pipelineRef` | `object` |  | PipelineRef names the Pipeline that ORIGINATED this conversation. |
| `runtimeRef` | `object` |  | RuntimeRef / ServiceAccountName are the originating Pipeline's execution wiring, snapshotted at creation exactly as Toolsets and MCPConfigs are. |
| `serviceAccountName` | `string` |  |  |
| `signal` | `object` |  | What the ORIGINATING SIGNAL was, for attribution. |
| `title` | `string` |  | Bounded by MaxConversationTitle — see that constant's own comment for why a cap belongs HERE despite the message-body rule against transport-shaped limits. |
| `toolsets` | `object` |  | Toolsets / MCPConfigs are the originating Pipeline's tooling bindings, snapshotted at creation like ChannelRefs and ProfileRef: materialized per-conversation state, NOT wiring — nothing sets them by hand, and re-wiring the pipeline affects only new conversations. |
| `workspaceClaimName` | `string` |  |  |

<!-- /generated -->

Most of these fields are snapshots and provenance — what each means for
capacity and queueing stays in
[Concepts](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/concepts.md).
