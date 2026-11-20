---
title: "Put an agent to work"
permalink: /guides/pipeline/
description: >-
  What a Pipeline is, why it is the only object in agent-ops that carries any
  wiring, and how to build a working route out of what you already have.

next:
  eyebrow: Next
  title: "Add your own agent"
  body: >-
    Declare an agent of your own — its role, its bounds, and what executes it —
    then route to it with a Pipeline.
  url: /agent-ops-operator/guides/agent-profile/
---

A `Pipeline` is **the wiring, and the only object in agent-ops that carries
any**. It names what starts a conversation, which agent answers, where the
answer goes, and what that agent may touch.

{: .ao-callout}
> **To learn what an agent can do, read its Pipeline.** No permission lives on
> the profile, and none is inherited.

![A SignalSource feeds a Pipeline, and the Pipeline names an AgentProfile to answer, Channels to answer on, and the toolsets and MCP servers it may use.]({{ '/assets/img/guides/pipeline-light.svg' | relative_url }}){: .ao-diagram}

You create nothing new here. Every object a Pipeline names is already in your
install. If a bundle shipped sources and tooling but no route, this is the
missing piece. For the other cases, see [Add your own agent]({{ '/guides/agent-profile/' | relative_url }})
and [Give your agent tools]({{ '/guides/toolsets/' | relative_url }}).

## What it names

| Field | Means | If omitted |
|---|---|---|
| `profileRef` | who answers | **required** |
| `signalSourceRefs` | the sources it listens on | claims nothing, reachable by name only |
| `channelRefs` | where the answer goes (all of them, mirrored) | the answer stays on the conversation |
| `toolsets`, `mcpConfigs` | what it may touch, composed by `mode` | no tools |
| `runtimeRef` | what executes it | the runtime named `default` |
| `serviceAccountName` | whose credentials it runs with | an account bound to **nothing** |
| `persistence` | where its conversations keep state | the release-wide volumes |

**Ready validates every reference.** Until it is `True` the Pipeline claims
nothing, and its message names what it could not find.

## Write one

<!-- generated: template kind=Pipeline name=my-route fields=signalSourceRefs,channelRefs,toolsets,toolsets.mode,mcpConfigs,runtimeRef,serviceAccountName,persistence.context.claimName comments=off -->
```yaml
apiVersion: agentops.dev/v1alpha1
kind: Pipeline
metadata:
  name: my-route
spec:
  signalSourceRefs:
  - name: <name>
  channelRefs:
  - name: <name>
  toolsets:
    refs:
    - name: <name>
    mode: merge
  mcpConfigs:
    refs:
    - name: <name>
  runtimeRef:
    name: <name>
  serviceAccountName: <serviceAccountName>
  persistence:
    context:
      claimName: <claimName>
```
<!-- /generated -->

Name it for its job, not its channel: `k8s-observe`, not `telegram-route`.
Tool lists concatenate in order with dedup, and a later server key wins a
collision.

```sh
kubectl -n agent-ops apply -f my-route.yaml
kubectl -n agent-ops get pipeline my-route \
  -o jsonpath='{.status.conditions[?(@.type=="Ready")]}'
```

```powershell
kubectl -n agent-ops apply -f my-route.yaml
kubectl -n agent-ops get pipeline my-route `
  -o jsonpath='{.status.conditions[?(@.type==\"Ready\")]}'
```

`Ready=True` means every ref resolved, and a claimed source flips to `WIRED=True`.

## Power: tools and identity

`toolsets` say which tools may be called. `serviceAccountName` says with whose
credentials. Both live here, so one object states what an agent can do.

{: .ao-callout}
> **Silence means no power.** A route naming no account runs as one bound to
> nothing. Cluster power is an account you declare in `rbac.runtime.serviceAccounts`
> and then name here.

```yaml
spec:
  toolsets:
    refs: [{name: agentops-observe}, {name: k8s-admin}]
  serviceAccountName: agentops-runtime-acting
```

- The account lives in the conversations namespace, beside the pods that run as it.
- A name nothing backs fails when the pod is created, saying which account.
- To pull one route back to nothing, name `agentops-runtime`, the floor the chart always renders.

## Storage per route

A route can keep its conversations on volumes of its own. Each volume takes
**either** a claim or a `PersistentVolume`, never both:

```yaml
spec:
  persistence:
    context:
      claimName: ha-ops-context
```

| Field | Means |
|---|---|
| `claimName` | an existing `PersistentVolumeClaim`, in the conversations namespace |
| `volumeName` | a `PersistentVolume`, and the manager renders the claim `agentops-<route>-<volume>` |

`workspace` takes the same two fields. The claim outlives the route, so deleting
a Pipeline never deletes the context it accumulated.

Re-wiring never moves a conversation already running. Changes apply to
conversations started afterwards.

## Reach it

A Pipeline is reached by a signal posted to a source it claims, or by a
`/<pipeline>` command on any wired chat surface:

```
/my-route what is running in kube-system?
```

Addressing needs no claim, and `/pipelines` lists the Ready routes. There is no
HTTP form that names a Pipeline.

## Sharing a source

Wiring is many-to-many with no exclusivity. Several Pipelines can claim one
source, and each opens its own conversation. On a chat surface, an
unaddressed message behaves by how many Ready Pipelines serve it:

| Ready Pipelines | Unaddressed message |
|---|---|
| one | routes to it |
| several | answered with the list of agents, so the person names one |
| none | dropped, and the source reports `Wired=False` |

## What comes next

1. **[Add your own agent]({{ '/guides/agent-profile/' | relative_url }})** — when none of the installed agents fits.
2. **[Give your agent tools]({{ '/guides/toolsets/' | relative_url }})** — declare toolsets and MCP servers to bind here.
3. **[Every Pipeline field]({{ '/crds/pipeline/' | relative_url }})** — the full shape of each ref.
