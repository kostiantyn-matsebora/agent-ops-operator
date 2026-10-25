---
title: ConversationInput
permalink: /crds/conversation-input/
description: >-
  Every spec field of the ConversationInput kind — the queued input record
  kept out of line, beside its Conversation.
---

A `ConversationInput` is the queued input record — one work unit's payload
kept out of line, naming its `Conversation` through `conversationRef`.

The manager writes one per oversized input, keeping the originating signal's
labels and severity beside the payload rather than on the `Conversation`.

## Spec, at a glance

<!-- generated: crd-example kind=ConversationInput name=disk-pressure-input-1 -->
```yaml
apiVersion: agentops.dev/v1alpha1
kind: ConversationInput
metadata:
  name: disk-pressure-input-1
spec:
  conversationRef:    # conversationRef names the Conversation this input queues into
    name: <name>
  payload: <payload>  # the input's full text, out-of-line so a bulky work unit never bloats …
  type: task   # task | alert | reply | recurrence | job
  labels:             # the originating signal's labels, kept per-input beside the payload
    <label>: <value>
  severity: critical   # critical | error | warning | info
```
<!-- /generated -->

## Every field

<!-- generated: crd-fields kind=ConversationInput -->

| Field | Type | Required | Meaning |
|---|---|---|---|
| `conversationRef` | `object` | **yes** | ConversationRef names the Conversation this input queues into. |
| `payload` | `string` | **yes** | The input's full text, out-of-line so a bulky work unit never bloats the Conversation object itself. |
| `type` | `string` | **yes** | Type says what kind of input this is, which decides how it is dispatched. |
| `labels` | `map[string]string` |  | The originating signal's labels, kept per-input beside the payload. |
| `severity` | `string` |  | The originating signal's own severity rating for this occurrence. |

<!-- /generated -->

The smallest kind here — how the queue it feeds is drained, strictly one
unit at a time, stays in
[Concepts](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/concepts.md).
