---
title: AgentRuntime
permalink: /crds/agent-runtime/
description: >-
  Every spec field of the AgentRuntime kind — the image and pod defaults
  that execute an agent's conversations.
---

An `AgentRuntime` is what executes an agent — the runtime image and the pod
defaults its conversations run with. It carries no volume: persistence is
wiring, declared on the `Pipeline`.

`Pipeline`, `Coordinator` and `AgentCapability` select one through
`runtimeRef`, and the CR named `default` answers when none is named.

## Spec, at a glance

<!-- generated: crd-example kind=AgentRuntime name=default -->
```yaml
apiVersion: agentops.dev/v1alpha1
kind: AgentRuntime
metadata:
  name: default
spec:
  image: <image>                            # image implementing the work contract
  args:                                     # command / Args override the image's entrypoint
  - <arg>
  command:                                  # command/Args override the image entrypoint
  - <command>
  contextStorage: volume                    # contextStorage declares whether this runtime's backend keeps a …
  contextSync:                              # contextSync moves the LIVE context off the durable volume and keeps a …
    paths:
    - <path>
  egressMediation:                          # egressMediation interposes a proxy in the runtime pod that the agent's …
    excludePorts:
    - 8080
    port: 15001
    resources:
      claims:
      - name: <name>
      limits: <limits>
      requests: <requests>
  env:                                      # extra environment for every runtime pod of this runtime
  - name: <name>
  idleTtlMinutes: 10                        # idleTTLMinutes before an idle runtime pod exits (respawned on demand)
  nodeSelector: <nodeSelector>              # nodeSelector placing runtime pods, applied with Tolerations and …
  resources:                                # resources default for runtime pods (AgentProfile.resources overrides)
    claims:
    - name: <name>
    limits: <limits>
    requests: <requests>
  serviceAccountName: <serviceAccountName>  # this runtime's security identity: its RBAC defines exactly what agents …
```
<!-- /generated -->

## Every field

<!-- generated: crd-fields kind=AgentRuntime -->

| Field | Type | Required | Meaning |
|---|---|---|---|
| `image` | `string` | **yes** | Image implementing the work contract. |
| `args` | `[]string` |  | Command / Args override the image's entrypoint. |
| `command` | `[]string` |  | Command/Args override the image entrypoint. |
| `contextStorage` | `string` |  | ContextStorage declares whether this runtime's backend keeps a conversation's context on a disk at all, so the manager can tell whether continuity is possible here BEFORE promising it. |
| `contextSync` | `object` |  | ContextSync moves the LIVE context off the durable volume and keeps a snapshot on it instead. |
| `egressMediation` | `object` |  | EgressMediation interposes a proxy in the runtime pod that the agent's traffic cannot route around, so the tool access its wiring granted is enforced somewhere the agent does not control. |
| `env` | `[]object` |  | Extra environment for every runtime pod of this runtime. |
| `idleTtlMinutes` | `integer` |  | IdleTTLMinutes before an idle runtime pod exits (respawned on demand). |
| `nodeSelector` | `map[string]string` |  | NodeSelector placing runtime pods, applied with Tolerations and Affinity below. |
| `resources` | `object` |  | Resources default for runtime pods (AgentProfile.resources overrides). |
| `serviceAccountName` | `string` |  | This runtime's security identity: its RBAC defines exactly what agents executing on this runtime may do in the cluster. |

<!-- /generated -->

One example, one table — building an image of your own against the work
contract is the
[AgentRuntime specification]({{ '/contracts/runtime/' | relative_url }}).
