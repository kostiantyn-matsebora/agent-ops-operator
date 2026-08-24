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
> **To learn what an agent can do, read its Pipeline.** There is nowhere else to
> look. No permission lives on the profile, and none is inherited.

![A SignalSource feeds a Pipeline, and the Pipeline names an AgentProfile to answer, Channels to answer on, and the toolsets and MCP servers it may use.]({{ '/assets/img/guides/pipeline-light.svg' | relative_url }}){: .ao-diagram}

## Before you start

Creating a Pipeline is appropriate when:

- You want a **second route** over what is already installed — a different agent
  on an existing source, or an existing agent on a new surface.
- You enabled a **bundle** that ships sources, profiles and tooling but no
  wiring, and nothing answers yet.
- You want an agent that is **reachable by name** and claims no source at all.

It is **not** what you want when:

- The agent you need **does not exist yet**. Create it first —
  [Add your own agent]({{ '/guides/agent-profile/' | relative_url }}).
- The tool or server you need does not exist yet —
  [Give your agent tools]({{ '/guides/toolsets/' | relative_url }}).

**You create nothing new here.** Every object a Pipeline names is already in
your install, which is what makes this the first thing to learn.

Review
[Pipeline](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/concepts.md#pipeline)
first.

## The overall shape

A Pipeline is six sets of references and one composition rule:

1. **`profileRef`** — who answers. The only **required** field.
2. **`signalSourceRefs`** — the sources it listens on. Omit it and the Pipeline
   claims nothing, and is reachable only by name.
3. **`channelRefs`** — where the answer goes. Every conversation is mirrored on
   all of them.
4. **`toolsets` and `mcpConfigs`** — what it may touch, plus the `mode` that
   composes them against the agent's own definition.
5. **`runtimeRef` and `serviceAccountName`** — what executes it, and under whose
   identity. Both optional.
6. **`persistence`** — where its conversations keep their state. Optional.

Nothing else carries wiring. A `SignalSource` has no profile and no channel. A
`Channel` has no default profile. That concentration is the point.

**Ready validates every reference.** Until the condition is `True` the Pipeline
claims nothing, and its message names the object it could not find.

## What runs it, and as whom

**A Pipeline also decides what executes its conversations.** Two optional
fields, and omitting both is the ordinary case.

| Field | Selects | Omit it and |
|---|---|---|
| `runtimeRef` | the `AgentRuntime` | the one named `default` runs it |
| `serviceAccountName` | the identity that runtime executes under | **it can do nothing in the cluster** |

**This is the same decision as `toolsets`.** One says which tools may be called.
The other says with whose credentials. Both are here so that ONE object states
what an agent may do.

{: .ao-callout}
> **Silence means no power.** A route that names no account runs as an identity
> bound to nothing. It reaches only what its toolsets and MCP servers give it.
> Cluster power is something you opt into, by name.

```yaml
spec:
  profileRef:
    name: k8s-engineer
  toolsets:
    refs:
      - name: agentops-observe
      - name: k8s-admin
  serviceAccountName: agentops-runtime-acting
```

**Two routes can share one runtime image and differ only in their account** —
that is what the field is for. The difference is the credentials, not the
engine.

**Naming an account does not create one.** The chart renders the ones
`rbacMode` produces, each bundle renders the accounts its own routes need, and
`rbac.runtime.serviceAccounts` is where you declare your own. A name nothing
backs fails when the pod is created, saying which account.

**Re-wiring never moves a conversation already running.** The conversation
records what it was created with, so changing this field affects only
conversations started afterwards. Correcting the `AgentRuntime` itself still
reaches the ones running.

## Where its conversations keep their state

**A route can declare its own storage**, beside the tools it grants and the
account it runs as. Omit it and the route takes the release-wide volumes the
chart configures, which is what nearly every route wants.

**An `AgentRuntime` declares no volume at all.** A runtime is an ENGINE, and
where a route persists is the route's decision — the same argument that put
`serviceAccountName` here.

```yaml
spec:
  profileRef:
    name: ha-engineer
  persistence:
    context:
      claimName: ha-ops-context
```

Each volume takes EITHER a claim or a `PersistentVolume`, never both:

| Field | Means |
|---|---|
| `claimName` | a `PersistentVolumeClaim` that already exists. Nothing is created |
| `volumeName` | a `PersistentVolume`. **The manager renders the claim on it**, as `agentops-<route>-<volume>` |

**A pod can mount only a claim, never a `PersistentVolume`** — which is why
naming a volume is the one place in agent-ops where naming a resource creates
it.

**The API server refuses both fields at once.** They decide who creates the
claim, so both is two answers rather than a preference.

{: .ao-callout}
> **The claim outlives the route.** It carries no ownerRef on the Pipeline and
> the manager holds no `delete` verb on claims, so deleting this route never
> deletes the accumulated context of the conversations it started. Removing the
> claim is yours to do deliberately.

**`workspace` takes the same two fields**, for the repository checkout.

**Two routes on ONE runtime can persist to different volumes.** Before this,
that needed a second `AgentRuntime` identical but for one field — the same
failure a second trust level used to have.

**Re-wiring never moves a conversation already running here either**, and the
reason is sharper: that conversation has already WRITTEN to the volume it was
created against.

## See what you have to wire

```sh
kubectl -n agent-ops get agentprofiles,signalsources,channels,mcptoolsets,mcpconfigs
```

```powershell
kubectl -n agent-ops get agentprofiles,signalsources,channels,mcptoolsets,mcpconfigs
```

A demo install answers with a `k8s-engineer` profile, a `console` source and
channel, a `cluster-events` source, and the three built-in toolsets —
`agentops-observe` (`Read` `Grep` `Glob`), `agentops-shell` (`Bash`) and
`agentops-edit` (`Edit` `Write`).

That is enough to build a route with no new objects at all.

## Write the Pipeline

<!-- generated: template kind=Pipeline name=my-route fields=signalSourceRefs,channelRefs,toolsets,toolsets.mode,mcpConfigs,runtimeRef,serviceAccountName,persistence.context.claimName comments=off -->
```yaml
apiVersion: agentops.dev/v1alpha1
kind: Pipeline
metadata:
  name: my-route
spec:
  profileRef:
    name: <name>
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

**Name it for its JOB**, not for the channel it answers on. `ha-ops` and
`k8s-observe` say what they are for. `telegram-route` says nothing.

**Refs apply in order.** Tool lists concatenate with dedup, the first occurrence
keeping its position. Server keys overlay, and a later ref wins a collision.

## Apply it, and check it is Ready

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

`Ready=True` means every ref resolved. A source you claimed also flips to
`WIRED=True`, which you can see with `kubectl get signalsources`.

## Reach it

A Pipeline is reached two ways, and no others:

1. **A signal posted to a source it claims.**
2. **A `/<pipeline>` command** on any wired chat surface.

```
/my-route what is running in kube-system?
```

**Addressing needs no claim.** A Pipeline listing no chat source is still
reachable by name, and the reply lands in the thread it was asked from.

There is no HTTP form that names a Pipeline. A caller selecting its own wiring
is the shape this object exists to prevent.

`/pipelines` lists the Ready ones, so an addressable route stays discoverable
whether or not it claims anything.

## Share a source, or do not

**Wiring is many-to-many in every direction**, and there is no exclusivity
anywhere:

| Relationship | Allowed |
|---|---|
| One Pipeline claims many sources | yes |
| Many Pipelines claim one source | yes — each opens its **own** conversation, with its own profile and capabilities |
| One channel carries many Pipelines' conversations | yes |

Two agents watching one thing is an ordinary configuration you choose, not a
hazard. There is no conflict condition and no tiebreak.

The only consequence of several claimants is on a chat surface:

| Ready Pipelines serving a chat source | An **unaddressed** message |
|---|---|
| one | routes to it |
| several | is answered with the list of agents, so the person names one |
| none | is **dropped**, and the source reports `Wired=False` |

Listing a chat source on a Pipeline that is only ever addressed grants it
nothing, while making every unaddressed message on that surface ambiguous.

## Compare against a working route

This is what the chart's `k8s-bundle` renders:

<!-- generated: example preset=tier1 kind=Pipeline name=k8s-observe -->
```yaml
# Source: agent-ops-operator/charts/k8s-bundle/templates/pipelines.yaml
apiVersion: agentops.dev/v1alpha1
kind: Pipeline
metadata:
  name: k8s-observe
  namespace: agent-ops
  labels:
    app.kubernetes.io/name: agentops-k8s-bundle
spec:
  # Display only: how this route is recognised in a chat command menu or the
  # console's typeahead. Nothing routes on it.
  icon: "aops:observe"
  profileRef:
    name: k8s-engineer
  # UNDER WHOSE IDENTITY, overriding the runtime's own. The same decision as the
  # toolsets below — which tools, and with whose credentials — which is why both
  # are on this one object.
  #
  # The account is the PARENT's to render (`rbac.runtime.serviceAccounts`) or
  # yours to grant. This bundle renders no ServiceAccount.
  serviceAccountName: agentops-k8s-observe
  # Wiring is pipeline-only: without this claim the source reports Wired=False
  # and DROPS every event it admits.
  signalSourceRefs:
    - name: cluster-events
    - name: console
  # A class of object this bundle does not render, so each is named from values
  # and the key is absent when nobody named one. With no channel the
  # conversation dispatches immediately and its answer lands in
  # status.runs[].result.
  channelRefs:
    - name: console
  # Declared, not inherited: profiles carry no capabilities and nothing supplies
  # a default, so this stanza IS the agent's allowlist.
  toolsets:
    refs:
      - name: agentops-observe
      - name: k8s-observability
  mcpConfigs:
    refs:
      - name: k8s-api
```
<!-- /generated -->

Two sources claimed, one channel answered on, the observing toolsets and the
Kubernetes MCP server granted. Read it and you know exactly what that agent can
do.

## What comes next

1. **[Add your own agent]({{ '/guides/agent-profile/' | relative_url }})**
   — when none of the installed agents is the one you want.
2. **[Give your agent tools]({{ '/guides/toolsets/' | relative_url }})**
   — declare new toolsets and MCP servers to bind here.
3. **[Every Pipeline field](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/cr-reference.md#pipeline)**
   — the icon, and the full shape of each ref stanza.
