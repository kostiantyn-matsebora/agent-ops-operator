---
title: Channel
permalink: /crds/channel/
description: >-
  Every spec field of the Channel kind — a chat surface that carries a
  conversation and never starts one.
---

A `Channel` is a chat surface. It carries a conversation and never starts
one — `spec.adapter` names the `ChannelAdapter` serving it, and a `Pipeline`
or `Coordinator` binds it through `channelRefs`.

## Spec, at a glance

<!-- generated: crd-example kind=Channel name=k8s-ops -->
```yaml
apiVersion: agentops.dev/v1alpha1
kind: Channel
metadata:
  name: k8s-ops
spec:
  adapter: <adapter>     # adapter names the ChannelAdapter serving this surface — a REFERENCE, …
  config: {}             # config carries whatever the channel type needs
  credentialsSecretRef:  # credentialsSecretRef names the Secret holding this surface's transport …
    name: <name>
```
<!-- /generated -->

## Every field

<!-- generated: crd-fields kind=Channel -->

| Field | Type | Required | Meaning |
|---|---|---|---|
| `adapter` | `string` | **yes** | Adapter names the ChannelAdapter serving this surface — a REFERENCE, not an attribute: the named adapter's implementation is what defines and validates Config's schema. |
| `config` | `free-form` |  | Config carries whatever the channel type needs. |
| `credentialsSecretRef` | `object` |  | CredentialsSecretRef names the Secret holding this surface's transport credentials (e.g. a bot token) — credentials are per-surface usage, never per-implementation. |

<!-- /generated -->

Three fields, the rest the adapter's — wiring a surface up end to end is the
[Talk to agents from your own chat]({{ '/guides/channel-adapter/' | relative_url }})
guide.
