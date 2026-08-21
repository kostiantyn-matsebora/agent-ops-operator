---
title: Installation
permalink: /installation/
description: >-
  Install agent-ops for real — the decisions to make first, the values that
  matter, how to enable a bundle, and the one route without which nothing
  answers.

next:
  eyebrow: Next
  title: Every CRD in full
  body: >-
    The kinds you just declared, field by field, and exactly how a route's tool
    access resolves.
  url: https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/concepts.md
---

This is the real install. For a first look in fifteen minutes, read
[Getting started]({{ '/getting-started/' | relative_url }}) instead — it
installs a read-only demo and says so.

## Decide first

Three choices are cheap now and expensive later.

| Decision | Default | Change it and |
|---|---|---|
| **Storage** — do conversations remember? | a `ReadWriteMany` claim | **every run starts fresh.** The operator says so up front rather than failing a follow-up |
| **The agent's power** — what may it do? | `none` | see below. `full` is cluster-admin |
| **CRD ownership** — what survives uninstall? | installed and kept | uninstall takes every Conversation with it |

**Storage.** `ReadWriteMany` matters because more than one agent runs at a time.
With `ReadWriteOnce` every agent pod lands on one node.

**The agent's power.** `global.agentops.runtime.rbacMode` is the agent's reach.
It ships empty, which resolves to `none`.

| Mode | Bound to | When |
|---|---|---|
| `none` | nothing | **the default** |
| `readonly` | `view`, plus node and metrics reads | the default under demo mode |
| `full` | **cluster-admin** | never a default. Never inferred |

`full` gives an LLM-driven process unrestricted control of your cluster. Prefer
`readonly` plus targeted grants in `rbac.runtime`.

**CRD ownership.** The keep annotation protects nothing retroactively. Decide it
before the first install, not after.

## Install

1. **Create the namespace and the model credential.**

   ```sh
   kubectl create namespace agent-ops

   kubectl -n agent-ops create secret generic agentops-claude \
     --from-literal=oauthToken=$(claude setup-token)   # or an Anthropic API key
   ```

   ```powershell
   kubectl create namespace agent-ops

   kubectl -n agent-ops create secret generic agentops-claude `
     --from-literal=oauthToken=$(claude setup-token)   # or an Anthropic API key
   ```

2. **Install the chart.**

   ```sh
   helm install agent-ops ./chart -n agent-ops
   ```

   ```powershell
   helm install agent-ops ./chart -n agent-ops
   ```

3. **Verify.**

   ```sh
   kubectl -n agent-ops rollout status deploy/agentops-manager
   kubectl -n agent-ops get agentruntime default
   ```

   ```powershell
   kubectl -n agent-ops rollout status deploy/agentops-manager
   kubectl -n agent-ops get agentruntime default
   ```

That brings up the manager, `AgentRuntime/default` and the console — and **no
routes**, so nothing answers yet. The next two sections fix that.

## Enable a bundle

A bundle contributes domain — sources, profiles, tooling, channels. The
substrate they run on comes from this chart.

| Bundle | Set | Its values |
|---|---|---|
| Kubernetes events | `k8s-bundle.enabled` | [k8s-bundle](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/k8s-bundle.md) |
| Prometheus alerts | `prometheus-bundle.enabled` | [prometheus-bundle](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/prometheus-bundle.md) |
| Telegram | `telegram-bundle.enabled` | [telegram-bundle](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/telegram-bundle.md) |
| Home Assistant | `ha-bundle.enabled` | [ha-bundle](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/ha-bundle.md) |

All four are off by default. Each bundle's own page owns its values — this page
does not repeat them.

## Configure

The values below are the ones you decide. For the exhaustive list:

```sh
helm show values ./chart
```

```powershell
helm show values ./chart
```

### Capacity

| Key | Default | Consequence |
|---|---|---|
| `maxActiveConversations` | `5` | how many agents hold a pod at once. Over-cap work waits in `Pending` with no pod and no thread |
| `maxQueuedConversations` | `50` | the backlog bound. Past it, new signals are declined and the sender is told |
| `runtimeIdleTtlMinutes` | `1` | how long a finished agent keeps its pod. Raise it for expensive startup |

### Storage

| Key | Default | Consequence |
|---|---|---|
| `persistence.enabled` | `true` | off means conversations never keep context |
| `persistence.accessModes` | `[ReadWriteMany]` | `ReadWriteOnce` pins every agent pod to one node |
| `persistence.size` | `5Gi` | session files for every conversation |
| `persistence.storageClassName` | `""` | empty uses the cluster default |

**Surviving a damaged volume.** A shared volume's filesystem can be corrupted by
a node reboot, and the storage layer will still call it healthy — it replicates
blocks and cannot see a filesystem.

Three settings limit what that costs. All are off by default and independent.

| Key | Default | What it does |
|---|---|---|
| `runtime.contextSync.paths` | `[]` | moves the live context to pod-local storage, leaving a snapshot on the volume |
| `rbac.drainAware` | `false` | releases idle agent pods from a cordoned node, so the filesystem unmounts before the reboot |
| `contextProbe.enabled` | `false` | hourly mount probe, so a damaged idle volume is found in an hour rather than at next use |

`contextSync` is the one that matters most. With it set, the agent container
gets ephemeral storage and **no mount of the durable volume at all** — so a run
already going survives the volume failing underneath it.

It needs `paths`, because only the runtime knows where its backend keeps
context. For the reference runtime:

```yaml
runtime:
  contextSync:
    paths: [".claude/projects/-data-workspace/**"]
```

`rbac.drainAware` costs the manager its only cluster-scoped permission — reading
nodes. It shrinks the window in which a reboot can corrupt the volume. It does
not close it.

### The agent's power

| Key | What it does |
|---|---|
| `global.agentops.runtime.rbacMode` | the mode above. Empty by default, resolving to `none` |
| `rbac.runtime.bindClusterRoles` | existing ClusterRoles to bind, additive to the mode. Default `[]` |
| `rbac.runtime.clusterRoles` | rules to create and bind. Default `[]` |
| `global.agentops.runtime.serviceAccountName` | the one identity every agent runs as — its RBAC **is** the agent's power. Default `agentops-runtime` |

### The runtime

```yaml
runtime:
  # the agent backend — swap it to change vendor
  image: kmatsebora/agentops-runtime-claude:0.6.0
  credentialsSecret:
    # read by the kubelet, never by the operator
    name: agentops-claude
  # set it when the runtime image is not multi-arch
  nodeSelector: {}

image:
  # the manager itself; its tag moves per release
  repository: kmatsebora/agentops-manager
```

A wrong credential fails late and quietly. The pod is created, then sits in
`CreateContainerConfigError` while conversations queue behind it.

### Access

| Key | Default | Consequence |
|---|---|---|
| `adapterAuth.secretName` | `agentops-adapter-token` | the token every adapter authenticates with. Change it and every adapter 401s until restarted |
| `console.enabled` | `true` | the browser console, on by default |

The console's own values — its token, ingress, TLS and forward-auth — are in
[console.md](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/console.md),
which owns the trust boundary.

### Housekeeping

| Key | Default | Consequence |
|---|---|---|
| `retention.autoclose.enabled` | `false` | close idle finished conversations. Reversible — a closed conversation reopens |
| `retention.autoclose.idleAge` | `168h` | idle time, never lifetime |
| `retention.autodelete.enabled` | `false` | **irreversible.** `status.runs[].result` is the only durable copy of an answer |
| `retention.autodelete.closedAge` | `720h` | how long you want to be able to read it, not how long until it is tidy |
| `housekeeping.enabled` | `false` | the CronJob that reclaims disk for conversations that no longer exist |

### Lifecycle

| Key | Default | Consequence |
|---|---|---|
| `crds.enabled` | `true` | install and upgrade the CRDs with the release |
| `crds.keep` | `true` | uninstall deletes neither the CRDs nor your Conversations |

## Wire one route

A source no Ready `Pipeline` claims **drops every signal it admits**. A fresh
install has exactly that, so nothing answers until you declare a route.

The smallest real one names a source, a profile and the tools that profile may
use:

```yaml
apiVersion: agentops.dev/v1alpha1
kind: Pipeline
metadata:
  name: cluster-triage
  namespace: agent-ops
spec:
  profileRef:
    name: k8s-engineer
  signalSourceRefs:
    - name: cluster-events
  toolsets:
    refs:
      - name: agentops-observe
      - name: k8s-observability
  mcpConfigs:
    refs:
      - name: k8s-api
```

Capability comes from this object and nothing else — the profile carries no
tools. The fields are in
[concepts](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/concepts.md).

## Upgrade and uninstall

```sh
helm upgrade agent-ops ./chart -n agent-ops
```

```powershell
helm upgrade agent-ops ./chart -n agent-ops
```

Read [CHANGELOG.md](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/CHANGELOG.md)
first. It is the only place migration steps live, newest first, keyed by chart
version.

An uninstall removes the workloads. With `crds.keep: true` it leaves the CRDs,
every Conversation and the session claim — so reinstalling finds your data
where it was.
