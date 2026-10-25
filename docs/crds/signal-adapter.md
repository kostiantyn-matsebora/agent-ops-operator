---
title: SignalAdapter
permalink: /crds/signal-adapter/
description: >-
  Every spec field of the SignalAdapter kind — the inbound-only workload
  that plugs a signal implementation into the operator.
---

A `SignalAdapter` plugs a signal implementation into the operator — inbound
only, pushing normalized signals to `/signal/inbound`, with no ops queue.

Its name is the routing key every `SignalSource.spec.adapter` references.

## Spec, at a glance

<!-- generated: crd-example kind=SignalAdapter name=cron -->
```yaml
apiVersion: agentops.dev/v1alpha1
kind: SignalAdapter
metadata:
  name: cron
spec:
  configSchema: {}                          # a JSON Schema (draft 2020-12) describing spec.config on the …
  credentialKeys:                           # credentialKeys documents the Secret keys the implementation expects in …
  - key: <key>
  externals:                                # externals names the systems outside the install this implementation …
  - kind: sender   # sender | api | kubernetes
    name: <name>
  image: <image>                            # image implementing the signal adapter contract
  port: 8080                                # port the image's own HTTP surface listens on (webhook-receiving …
  resources:                                # resourceRequirements describes the compute resource requirements
    claims:
    - name: <name>
    limits: <limits>
    requests: <requests>
  servedBy:                                 # servedBy declares this adapter EXTERNALLY SERVED: another adapter's …
    kind: ChannelAdapter
    name: <name>
  serviceAccountName: <serviceAccountName>  # the identity this adapter's workload runs as, and its token IS mounted …
  singleton: true                           # singleton runs the workload as replicas 1 + strategy Recreate so no …
```
<!-- /generated -->

## Every field

<!-- generated: crd-fields kind=SignalAdapter -->

| Field | Type | Required | Meaning |
|---|---|---|---|
| `configSchema` | `free-form` |  | A JSON Schema (draft 2020-12) describing spec.config on the Channels/SignalSources this adapter serves. |
| `credentialKeys` | `[]object` |  | CredentialKeys documents the Secret keys the implementation expects in a served CR's credentialsSecretRef. |
| `externals` | `[]object` |  | Externals names the systems outside the install this implementation faces. |
| `image` | `string` |  | Image implementing the signal adapter contract. |
| `port` | `integer` |  | Port the image's own HTTP surface listens on (webhook-receiving implementations). |
| `resources` | `object` |  | ResourceRequirements describes the compute resource requirements. |
| `servedBy` | `object` |  | ServedBy declares this adapter EXTERNALLY SERVED: another adapter's pod already holds the process, and this CR exists only to give it a signal identity. |
| `serviceAccountName` | `string` |  | The identity this adapter's workload runs as, and its token IS mounted — naming an account whose token is never mounted would grant nothing, since the pod would never present that identity. |
| `singleton` | `boolean` |  | Singleton runs the workload as replicas 1 + strategy Recreate so no rollout ever runs two instances side by side (pollers and schedulers must not double-fire). |

<!-- /generated -->

An image and its workload knobs, nothing more — what the image owes the
manager is the
[SignalAdapter specification]({{ '/contracts/signal-adapter/' | relative_url }}).
