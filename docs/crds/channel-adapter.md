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
| `credentialKeys[].key` | `string` | **yes** | The Secret key (projected as env <credentialEnvPrefix><KEY>). |
| `credentialKeys[].description` | `string` |  | Description of what this key holds. |
| `credentialKeys[].required` | `boolean` |  | Required marks a key the implementation cannot work without. |
| `echoesOwnMessages` | `boolean` |  | EchoesOwnMessages says whether this transport shows a person the message they just typed on it. Defaults to `true`. |
| `externals` | `[]object` |  | Externals names the systems outside the install this implementation faces. |
| `externals[].kind` | `string` | **yes** | How the adapter meets it: `sender` pushes to the adapter, `api` is called by the adapter, `kubernetes` is the cluster's own API. One of `sender`, `api`, `kubernetes`. |
| `externals[].name` | `string` | **yes** | The system as a reader knows it: "Alertmanager", "Telegram Bot API", "Kubernetes API". |
| `port` | `integer` |  | Port the image's own HTTP surface listens on (implementations that are PUSHED to rather than polling — e.g. a channel adapter receiving updates forwarded by an ingest router). |
| `resources` | `object` |  | ResourceRequirements describes the compute resource requirements. |
| `resources.claims` | `[]object` |  | Claims lists the names of resources, defined in spec.resourceClaims, that are used by this container. |
| `resources.claims[].name` | `string` | **yes** | Name must match the name of one entry in pod.spec.resourceClaims of the Pod where this field is used. |
| `resources.claims[].request` | `string` |  | The name chosen for a request in the referenced claim. |
| `resources.limits` | `map[string]object` |  | Limits describes the maximum amount of compute resources allowed. |
| `resources.requests` | `map[string]object` |  | Requests describes the minimum amount of compute resources required. |
| `serviceAccountName` | `string` |  | The identity this adapter's workload runs as, with its token mounted. |
| `singleton` | `boolean` |  | Singleton runs the workload as replicas 1 + strategy Recreate so no rollout ever runs two instances side by side (required for pull-based transports like Telegram getUpdates). Defaults to `true`. |

<!-- /generated -->

An image and its workload knobs, nothing more — what the image owes the
manager is the
[ChannelAdapter specification]({{ '/contracts/channel-adapter/' | relative_url }}).
