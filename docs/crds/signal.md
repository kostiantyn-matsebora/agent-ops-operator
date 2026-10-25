---
title: Signal
permalink: /crds/signal/
description: >-
  The durable, persist-then-ack record of one inbound signal arrival and its
  per-claimant routing workflow.
---

A `Signal` is created before the manager acknowledges the arrival that
produced it. Every later routing decision — a retry, a crash recovery, a
leader change — reads it back, never process memory.

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
  adapter: <adapter>            # the serving SignalAdapter's name, copied from SourceRef's own …
  initiationId: <initiationId>  # the sender-minted correlation id — see NormalizedSignal.InitiationID
  sourceRef:                    # sourceRef names the SignalSource this signal arrived on (same namespace)
    name: <name>
  kind: <kind>                  # kind selects the input lane: alert (default), job, task or chat — the …
  labels:                       # attribution only — rendered on signal cards, feeding no routing …
    <label>: <value>
  occurrences: 1                # occurrences counts how many signal arrivals have landed on this CR — 1 …
  payload: <payload>            # this signal's content, handed to the agent
  reader: <reader>              # the opaque key of the person who sent this, for a chat signal only — …
  severity: critical   # critical | error | warning | info
  title: <title>                # title overrides the conversation title a routed claimant opens with
```
<!-- /generated -->

## Every field

<!-- generated: crd-fields kind=Signal -->

| Field | Type | Required | Meaning |
|---|---|---|---|
| `adapter` | `string` | **yes** | The serving SignalAdapter's name, copied from SourceRef's own spec.adapter at creation — a display/filter convenience, so a reader says what kind of signal this is without a live lookup of the (possibly since-renamed) source. |
| `initiationId` | `string` | **yes** | The sender-minted correlation id — see NormalizedSignal.InitiationID. |
| `sourceRef` | `object` | **yes** | SourceRef names the SignalSource this signal arrived on (same namespace). |
| `sourceRef.name` | `string` | **yes** | Name of the referenced object. |
| `kind` | `string` |  | Kind selects the input lane: alert (default), job, task or chat — the same closed vocabulary httpapi.NormalizedSignal.Kind and InputOrigin.SignalKind already carry as a plain string, unenforced by either's own CRD schema. |
| `labels` | `map[string]string` |  | Attribution only — rendered on signal cards, feeding no routing decision (the manager routes purely by InitiationID equality). |
| `occurrences` | `integer` |  | Occurrences counts how many signal arrivals have landed on this CR — 1 for the arrival that created it, bumped by every later absorb (design D-5). Defaults to `1`. |
| `payload` | `string` |  | This signal's content, handed to the agent. |
| `reader` | `string` |  | The opaque key of the person who sent this, for a chat signal only — see NormalizedSignal.Reader. |
| `severity` | `string` |  | This signal's own urgency rating, from the occurrence that created this Signal. One of `critical`, `error`, `warning`, `info`. |
| `title` | `string` |  | Title overrides the conversation title a routed claimant opens with. |

<!-- /generated -->

`spec.payload` holds the arrival's content only until the signal routes. On
routing, the payload is handed off as a `ConversationInput` — the
conversation's own record, never the `Signal`, is the routed copy.

A repeat whose `initiationId` matches a still-open `Signal` of the same
source is absorbed into it: `spec.payload` appends and `spec.occurrences`
bumps, rather than a second CR. A repeat matching only a terminal `Signal`
creates a fresh one, since a terminal record is an audit entry, never
reopened.

`status.claimants[]` is the per-claimant routing workflow (`Routing` →
`Routed` / `Failed` / `Skipped`), snapshotted once at the first routing
attempt. `status.phase` is the roll-up over it — `Pending`, `Routed`,
`PartiallyRouted`, `Dropped` or `Failed` — recomputed on every status
write, never authored on its own.

The phase vocabulary, the claimant states, the bounds and the GC windows
are [Concepts](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/concepts.md)'s
to state in full. No component but the manager writes this kind — every
other reader, the console included, holds `get`/`list`/`watch` at most.
