---
title: SignalSource
permalink: /crds/signal-source/
description: >-
  Every spec field of the SignalSource kind — an ingest lane served by a
  signal adapter, claimed by the wiring that answers it.
---

A `SignalSource` is an ingest lane served by the `SignalAdapter` its
`spec.adapter` names. A `Pipeline` or `Coordinator` claims it through
`signalSourceRefs`, and a signal it admits starts a conversation.

## Spec, at a glance

<!-- generated: crd-example kind=SignalSource name=cluster-events -->
```yaml
apiVersion: agentops.dev/v1alpha1
kind: SignalSource
metadata:
  name: cluster-events
spec:
  adapter: <adapter>     # adapter names the SignalAdapter serving this source — a REFERENCE, not …
  attributeFilters:      # attributeFilters narrows which signals this source admits, evaluated …
  - key: <key>
    operator: In   # In | NotIn | Exists | DoesNotExist
  config: {}             # config carries whatever the signal type needs
  cooldownHours: 6       # suppress a repeat of the same initiationId within this window — a re- …
  credentialsSecretRef:  # credentialsSecretRef names the Secret holding this source's transport …
    name: <name>
  logFiltered: false     # logFiltered governs whether a signal dropped by AttributeFilters is …
```
<!-- /generated -->

## Every field

<!-- generated: crd-fields kind=SignalSource -->

| Field | Type | Required | Meaning |
|---|---|---|---|
| `adapter` | `string` | **yes** | Adapter names the SignalAdapter serving this source — a REFERENCE, not an attribute: the named adapter's implementation defines and validates Config's schema. |
| `attributeFilters` | `[]object` |  | AttributeFilters narrows which signals this source admits, evaluated against the normalized signal's labels with Kubernetes label-selector semantics. |
| `attributeFilters[].key` | `string` | **yes** | The attribute label name to test. |
| `attributeFilters[].operator` | `string` | **yes** | Operator selects the test. One of `In`, `NotIn`, `Exists`, `DoesNotExist`. |
| `attributeFilters[].values` | `[]string` |  | The comparison set for In/NotIn, and must be empty for Exists/DoesNotExist. |
| `config` | `free-form` |  | Config carries whatever the signal type needs. |
| `cooldownHours` | `integer` |  | Suppress a repeat of the same initiationId within this window — a re-send, safe to drop under at-least-once delivery. Defaults to `6`. |
| `credentialsSecretRef` | `object` |  | CredentialsSecretRef names the Secret holding this source's transport credentials (e.g. an API key). |
| `credentialsSecretRef.name` | `string` |  | Name of the referent. |
| `logFiltered` | `boolean` |  | LogFiltered governs whether a signal dropped by AttributeFilters is still recorded as a signal.dropped activity event — unset means true. |

<!-- /generated -->

What `attributeFilters[].key` can name is the signal's `labels` — the map
the serving adapter sends with every signal, so a key no signal carries
never matches.

- A signal must satisfy EVERY requirement to be admitted.
- A filtered signal is dropped before cooldown and before any pipeline
  sees it.

The shipped adapters' label vocabulary:

| Vocabulary | Keys |
|---|---|
| Shared | `alertgroup`, `alertname`, `namespace`, `severity`, `source` |
| Kubernetes events adapter adds | `kind`, `name`, **`workload`** (the owning controller, e.g. `Deployment/api`, resolved through owner references), `node`, plus the involved pod's own labels |

**Adapter-defined keys are reserved.** A pod label named `name` or
`workload` is never allowed to overwrite one, so a signal's own identifying
context stays legible regardless of what the underlying object carries.

The same deference holds for `config`: its shape is whatever the named
adapter declares as
[`SignalAdapter`]({{ '/crds/signal-adapter/' | relative_url }})
`spec.configSchema`, and each integration page shows the values its adapter
takes.

One example, one table — a lane of your own, filters and cooldown included,
is the [React to signals]({{ '/guides/signal-adapter/' | relative_url }})
guide.
