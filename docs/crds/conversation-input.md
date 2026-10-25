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
  conversationRef:  # objectRef references another object by name (same namespace)
    name: <name>
  payload: <payload>
  type: task   # task | alert | reply | recurrence | job
  labels:           # the originating signal's labels, kept beside the payload rather than …
    <label>: <label>
  severity: critical   # critical | error | warning | info
```
<!-- /generated -->

## Every field

<!-- generated: crd-fields kind=ConversationInput -->

| Field | Type | Required | Meaning |
|---|---|---|---|
| `conversationRef` | `object` | **yes** | ObjectRef references another object by name (same namespace). |
| `payload` | `string` | **yes** |  |
| `type` | `string` | **yes** | InputType classifies a work unit. |
| `labels` | `map[string]string` |  | The originating signal's labels, kept beside the payload rather than on the Conversation: they are per-input, they can be numerous, and this object exists precisely to hold the bulky half of an input. |
| `severity` | `string` |  | The originating signal's own severity rating for this occurrence, kept per-input the same way Labels is — each recurrence keeps its own, never retroactively rewriting the provenance. |

<!-- /generated -->

The smallest kind here — how the queue it feeds is drained, strictly one
unit at a time, stays in
[Concepts](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/concepts.md).
