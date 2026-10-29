---
title: Signal
permalink: /crds/signal/
description: >-
  The durable record of one published signal message and its per-claimant
  routing workflow.
---

A `Signal` is created before the manager acknowledges the message that
produced it, and routed afterwards by a separate consumer. Every routing
decision, whether a retry, a crash recovery or a leader change, reads it back
and never process memory.

Owned by the `SignalSource` named in `spec.sourceRef`, so deleting the
source takes its signals from any phase.

## Spec, at a glance

<!-- generated: crd-example kind=Signal name=cluster-events-sig-7f2a1 -->
```yaml
apiVersion: agentops.dev/v1alpha1
kind: Signal
metadata:
  name: cluster-events-sig-7f2a1
spec:
  adapter: <adapter>            # the serving SignalAdapter's name, copied from the source's …
  initiationId: <initiationId>  # the sender-minted correlation id, shared across the messages of one …
  sourceRef:                    # sourceRef names the SignalSource this signal arrived on (same namespace)
    name: <name>
  kind: <kind>                  # kind selects the input lane: alert (absent), job, task or chat — the …
  labels:                       # attribution only — rendered on signal cards, feeding no routing …
    <label>: <value>
  payload: <payload>            # this ONE message's content, handed to the agent
  reader: <reader>              # the opaque key of the person who sent this, for a chat signal only — …
  severity: critical   # critical | error | warning | info
  signalId: <signalId>          # the sender's id for this one MESSAGE, stable across every re-send of …
  title: <title>                # title overrides the conversation title a routed claimant opens with
```
<!-- /generated -->

## Every field

<!-- generated: crd-fields kind=Signal -->

| Field | Type | Required | Meaning |
|---|---|---|---|
| `adapter` | `string` | **yes** | The serving SignalAdapter's name, copied from the source's spec.adapter at creation — a display copy, so a reader says what kind of signal this is without a live lookup of the (possibly since-renamed) source. |
| `initiationId` | `string` | **yes** | The sender-minted correlation id, shared across the messages of one conversation on purpose. |
| `sourceRef` | `object` | **yes** | SourceRef names the SignalSource this signal arrived on (same namespace). |
| `sourceRef.name` | `string` | **yes** | Name of the referenced object. |
| `kind` | `string` |  | Kind selects the input lane: alert (absent), job, task or chat — the same closed vocabulary httpapi.NormalizedSignal.Kind and InputOrigin.SignalKind already carry as a plain string. |
| `labels` | `map[string]string` |  | Attribution only — rendered on signal cards, feeding no routing decision (the manager routes purely by InitiationID equality), except the reserved chat labels naming the originating surface. |
| `payload` | `string` |  | This ONE message's content, handed to the agent. |
| `reader` | `string` |  | The opaque key of the person who sent this, for a chat signal only — see NormalizedSignal.Reader. |
| `severity` | `string` |  | This signal's own urgency rating. One of `critical`, `error`, `warning`, `info`. |
| `signalId` | `string` |  | The sender's id for this one MESSAGE, stable across every re-send of it (design D-8). |
| `title` | `string` |  | Title overrides the conversation title a routed claimant opens with. |

<!-- /generated -->

`spec.payload` holds the message's content until it routes. On routing, the
payload is handed off as a `ConversationInput`: the conversation's own
record, never the `Signal`, is the routed copy.

One `Signal` is one message and a repeat `initiationId` is never folded into
an earlier one. The `Conversation` accumulates an `initiationId`. The optional
`spec.signalId` names the message: the `Signal`'s name derives from
`(source, signalId)`, so a re-send is stored once for the `Signal`'s life.

`status.claimants[]` is the per-claimant routing workflow (`Routing` →
`Routed` / `Failed` / `Skipped`), snapshotted once at the first routing
attempt. `status.phase` is the roll-up over it — `Pending`, `Routed`,
`PartiallyRouted`, `Dropped` or `Failed` — recomputed on every status
write, never authored on its own. A `Dropped` phase carries `status.reason`
(`unclaimed`, `ambiguous`, `cooldown`, `capacity`, `overflow` or `exhausted`).
The consumer and the leader's housekeeping are the only writers of status.

The phase vocabulary, the claimant states, the bounds and the GC windows
are [Concepts](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/concepts.md)'s
to state in full. No component but the manager writes this kind — every
other reader, the console included, holds `get`/`list`/`watch` at most.
