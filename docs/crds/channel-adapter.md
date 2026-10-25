---
title: ChannelAdapter
permalink: /crds/channel-adapter/
description: >-
  Every spec field of the ChannelAdapter kind — the out-of-process workload
  that plugs a channel implementation into the operator.
---

A `ChannelAdapter` plugs a channel implementation into the operator — an
out-of-process workload consuming `/channel/*`, the ops long-poll plus
inbound push.

Its name is the routing key: every `Channel` whose `spec.adapter` matches is
served by it.

## Spec, at a glance

<!-- generated: crd-example kind=ChannelAdapter name=telegram -->
```yaml
apiVersion: agentops.dev/v1alpha1
kind: ChannelAdapter
metadata:
  name: telegram
spec:
  image: <image>                            # image implementing the adapter contract
  configSchema: {}                          # a JSON Schema (draft 2020-12) describing spec.config on the …
  credentialKeys:                           # credentialKeys documents the Secret keys the implementation expects in …
  - key: <key>
  echoesOwnMessages: true                   # echoesOwnMessages says whether this transport shows a person the …
  externals:                                # externals names the systems outside the install this implementation …
  - kind: sender   # sender | api | kubernetes
    name: <name>
  port: 8080                                # port the image's own HTTP surface listens on (implementations that are …
  resources:                                # resourceRequirements describes the compute resource requirements
    claims:
    - name: <name>
    limits:
      cpu: "100m"
    requests:
      cpu: "100m"
  serviceAccountName: <serviceAccountName>  # the identity this adapter's workload runs as, with its token mounted
  singleton: true                           # singleton runs the workload as replicas 1 + strategy Recreate so no …
```
<!-- /generated -->

## Every field

<!-- generated: crd-fields kind=ChannelAdapter -->

| Field | Type | Required | Meaning |
|---|---|---|---|
| `image` | `string` | **yes** | Image implementing the adapter contract. |
| `configSchema` | `free-form` |  | A JSON Schema (draft 2020-12) describing spec.config on the Channels/SignalSources this adapter serves. |
| `credentialKeys` | `[]object` |  | CredentialKeys documents the Secret keys the implementation expects in a served CR's credentialsSecretRef. |
| `echoesOwnMessages` | `boolean` |  | EchoesOwnMessages says whether this transport shows a person the message they just typed on it. |
| `externals` | `[]object` |  | Externals names the systems outside the install this implementation faces. |
| `port` | `integer` |  | Port the image's own HTTP surface listens on (implementations that are PUSHED to rather than polling — e.g. a channel adapter receiving updates forwarded by an ingest router). |
| `resources` | `object` |  | ResourceRequirements describes the compute resource requirements. |
| `serviceAccountName` | `string` |  | The identity this adapter's workload runs as, with its token mounted. |
| `singleton` | `boolean` |  | Singleton runs the workload as replicas 1 + strategy Recreate so no rollout ever runs two instances side by side (required for pull-based transports like Telegram getUpdates). |

<!-- /generated -->

An image and its workload knobs, nothing more — what the image owes the
manager is the
[ChannelAdapter specification]({{ '/contracts/channel-adapter/' | relative_url }}).
