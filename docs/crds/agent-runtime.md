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
      limits:
        cpu: "100m"
      requests:
        cpu: "100m"
  env:                                      # extra environment for every runtime pod of this runtime
  - name: <name>
  idleTtlMinutes: 10                        # idleTTLMinutes before an idle runtime pod exits (respawned on demand)
  nodeSelector:                             # nodeSelector placing runtime pods
    <nodeSelector>: <value>
  resources:                                # resources default for runtime pods (AgentProfile.resources overrides)
    claims:
    - name: <name>
    limits:
      cpu: "100m"
    requests:
      cpu: "100m"
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
| `contextStorage` | `string` |  | ContextStorage declares whether this runtime's backend keeps a conversation's context on a disk at all, so the manager can tell whether continuity is possible here BEFORE promising it. One of `volume`, `external`, `none`. Defaults to `volume`. |
| `contextSync` | `object` |  | ContextSync moves the LIVE context off the durable volume and keeps a snapshot on it instead. |
| `contextSync.paths` | `[]string` | **yes** | INCLUDE globs, relative to the runtime's HOME, naming what is worth persisting. |
| `contextSync.exclude` | `[]string` |  | Exclude drops churn from INSIDE the included paths — lock files, temp files, anything rewritten constantly without being context. |
| `contextSync.interval` | `string` |  | How often the context is checkpointed while a pod is alive, as a Go duration ("2m"). "0" disables the timer and leaves only work-boundary checkpoints, which is the right setting for a low-churn backend. Defaults to `2m`. |
| `contextSync.retain` | `integer` |  | How many previous copies to keep. Defaults to `3`. |
| `egressMediation` | `object` |  | EgressMediation interposes a proxy in the runtime pod that the agent's traffic cannot route around, so the tool access its wiring granted is enforced somewhere the agent does not control. |
| `egressMediation.excludePorts` | `[]integer` |  | Destination ports left unredirected. |
| `egressMediation.port` | `integer` |  | Port the proxy listens on inside the pod, and the port the agent's traffic is redirected to. Defaults to `15001`. |
| `egressMediation.resources` | `object` |  | Resources for the proxy container. |
| `egressMediation.resources.claims` | `[]object` |  | Claims lists the names of resources, defined in spec.resourceClaims, that are used by this container. |
| `egressMediation.resources.claims[].name` | `string` | **yes** | Name must match the name of one entry in pod.spec.resourceClaims of the Pod where this field is used. |
| `egressMediation.resources.claims[].request` | `string` |  | The name chosen for a request in the referenced claim. |
| `egressMediation.resources.limits` | `map[string]object` |  | Limits describes the maximum amount of compute resources allowed. |
| `egressMediation.resources.requests` | `map[string]object` |  | Requests describes the minimum amount of compute resources required. |
| `env` | `[]object` |  | Extra environment for every runtime pod of this runtime. |
| `env[].name` | `string` | **yes** | Name of the environment variable. |
| `env[].value` | `string` |  | Variable references $(VAR_NAME) are expanded using the previously defined environment variables in the container and any service environment variables. |
| `env[].valueFrom` | `object` |  | Source for the environment variable's value. The standard Kubernetes `EnvVarSource` -- set exactly one of `configMapKeyRef`, `secretKeyRef`, `fieldRef` or `resourceFieldRef`, each naming its source object and key. |
| `idleTtlMinutes` | `integer` |  | IdleTTLMinutes before an idle runtime pod exits (respawned on demand). Defaults to `10`. |
| `nodeSelector` | `map[string]string` |  | NodeSelector placing runtime pods. |
| `resources` | `object` |  | Resources default for runtime pods (AgentProfile.resources overrides). |
| `resources.claims` | `[]object` |  | Claims lists the names of resources, defined in spec.resourceClaims, that are used by this container. |
| `resources.claims[].name` | `string` | **yes** | Name must match the name of one entry in pod.spec.resourceClaims of the Pod where this field is used. |
| `resources.claims[].request` | `string` |  | The name chosen for a request in the referenced claim. |
| `resources.limits` | `map[string]object` |  | Limits describes the maximum amount of compute resources allowed. |
| `resources.requests` | `map[string]object` |  | Requests describes the minimum amount of compute resources required. |
| `serviceAccountName` | `string` |  | This runtime's security identity: its RBAC defines exactly what agents executing on this runtime may do in the cluster. |

<!-- /generated -->

One example, one table — building an image of your own against the work
contract is the
[AgentRuntime specification]({{ '/contracts/runtime/' | relative_url }}).
