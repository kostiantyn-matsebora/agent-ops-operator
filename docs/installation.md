---
title: Installation
permalink: /installation/
description: >-
  Install agent-ops for real — the decisions to make first, the values that
  matter, how to enable a bundle, and the one route without which nothing
  answers.

next:
  eyebrow: Next
  title: "Put an agent to work"
  body: >-
    The only object that carries any wiring — what starts a conversation, which
    agent answers, and what it may touch. Built from what you already installed.
  url: /agent-ops-operator/guides/pipeline/
---

This is the real install. For a first look in fifteen minutes, read
[Getting started]({{ '/getting-started/' | relative_url }}) instead — it
installs a read-only demo and says so.

## Decide first

Three choices are cheap now and expensive later.

| Decision | Default | Change it and |
|---|---|---|
| **Storage** — do conversations remember? | a `ReadWriteMany` claim | **every run starts fresh.** The operator says so up front rather than failing a follow-up |
| **The agent's power** — what may it do? | `none`, and a route that names no account gets nothing | see below. Three settings, not one |
| **CRD ownership** — what survives uninstall? | installed and kept | uninstall takes every Conversation with it |

**Storage.** `ReadWriteMany` matters because more than one agent runs at a time.
With `ReadWriteOnce` every agent pod lands on one node.

**The agent's power.** `global.agentops.runtime.rbacMode` is the agent's reach.
It ships empty, which resolves to `none`.

| Mode | Renders | When |
|---|---|---|
| `none` | nothing | **the default** |
| `readonly` | `agentops-runtime-readonly` | the default under demo mode |
| `full` | `agentops-runtime-acting` | never a default. Never inferred |

**A mode renders a NAMED account, and a Pipeline opts into it.** Nothing is
bound to the account a Pipeline naming none runs as — see
[silence means no power](#silence-means-no-power).

**`full` used to be `cluster-admin`, and `readonly` used to bind the built-in
`view`.** Both are gone — see [CHANGELOG](CHANGELOG.md) for the version and the
upgrade step.

**One more decision comes with it:** whether an agent may run a pod
([`allowPodExecution`](#allowpodexecution--read-this-before-turning-it-on)).
That is what makes "agents cannot read your Secrets" true rather than merely
written down.

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

2. **Install the chart** from the registry. There is no repo to add and no
   checkout to clone.

   ```sh
   helm install agent-ops \
     oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator \
     --version 7.0.0 -n agent-ops
   ```

   ```powershell
   helm install agent-ops `
     oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator `
     --version 7.0.0 -n agent-ops
   ```

   **No registry credential.** The chart and every image it renders are public
   packages on GHCR, so the pull is anonymous and the install needs no
   `imagePullSecrets`.

   Working from a checkout instead? `helm install agent-ops ./chart -n
   agent-ops` still does the same thing.

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
helm show values oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator
```

```powershell
helm show values oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator
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
| `global.agentops.runtime.serviceAccountName` | the **floor** identity — what a Pipeline naming none runs as. It holds no RBAC. Default `agentops-runtime` |
| `global.agentops.runtime.allowPodExecution` | may it create or enter a pod. Default `false` |
| `rbac.runtime.serviceAccounts` | **additional** identities a Pipeline may name. Default `[]` |

#### Silence means no power

**A Pipeline that names no `serviceAccountName` runs as the floor account, and
nothing is ever bound to it.** That agent can do nothing in the cluster. It
reaches only what its toolsets and MCP servers give it.

**Acting power is something a route opts into, by name:**

```yaml
global:
  agentops:
    runtime:
      rbacMode: full            # renders agentops-runtime-acting

pipelines:
  - name: k8s-observe
    profile: k8s-engineer
    toolsets: [agentops-observe, k8s-observability]
    # names no account, so it gets the floor and no cluster power

  - name: k8s-operate
    profile: k8s-engineer
    toolsets: [agentops-observe, k8s-observability, k8s-admin]
    serviceAccountName: agentops-runtime-acting
```

**This is a deliberate break.** A Pipeline used to inherit whatever the release
granted, so a route held cluster power by not typing a field.

- **The parent renders the floor and whatever `rbacMode` produces.**
- **Each bundle renders the accounts its own routes need**, scoped to what those
  routes do. `ha-bundle` renders two with no Kubernetes RBAC at all, because
  neither of its routes touches the Kubernetes API.
- **`rbac.runtime.serviceAccounts`** is where you declare your own.
- **A name nothing backs fails at pod admission**, saying which account.

#### The grant is cluster-wide

**Every rule applies in every namespace, including this release's own.** There
is no namespace list, and a namespace created tomorrow is covered immediately.

What keeps that from exposing anything is **omission, not scope**:

| Never granted | So |
|---|---|
| `agentops.dev` | Conversations, Pipelines and profiles are unreadable everywhere |
| `secrets` | no credential is readable through RBAC |
| `clusterroles` | an agent cannot map which identity is worth attacking |

No component in this release logs message content, so the pod logs an agent can
read carry no conversation text. What it gains in the operator's namespace is
pod names and specs, Services, events and the compiled MCP ConfigMaps.

{: .ao-callout}
> **Under `full` an agent can also restart or delete those pods** — the manager
> and the adapters included. It can disrupt its own supervisor.

**Namespaced Roles were tried and reverted.** RBAC cannot express "everywhere
except", so bounding an agent meant an allow-list: one binding per namespace per
account, 224 objects on a 28-namespace cluster, and every new namespace
invisible to the agent until someone edited values and redeployed.

#### `allowPodExecution` — read this before turning it on

{: .ao-callout}
> **No `secrets` verb is not the same as cannot read Secrets.** The **kubelet**
> resolves a Secret when it builds a pod. An agent that can create a pod
> mounting one, or exec into a pod that already has one, reads the value having
> never asked the API server for a Secret.

Verified against the shipped role on a live cluster: pod created, pod log read,
secret value returned, with all seven `secrets` verbs denied throughout.

**So the flag is off by default**, and it gates every write that produces or
enters a pod — `pods: create`, `pods/exec`, and create/update/patch on
Deployments, StatefulSets, DaemonSets, ReplicaSets, Jobs and CronJobs. It is
the boundary that keeps Secrets genuinely out of reach.

**Gating only `pods: create` would close nothing.** `kubectl create job`
produces a pod just as well, and patching a Deployment edits a pod template.

**With it off an agent is still an operator.** It reads what it is scoped to,
scales, restarts, evicts, cordons, deletes workloads, and creates or edits
ConfigMaps, Services, Ingresses, NetworkPolicies, PDBs, HPAs and PVCs. What it
cannot do is run new code.

**Turn it on only if you accept the agent reading every Secret in the
cluster.** The grants are cluster-wide, so that is the scope.

#### What the roles grant

Every grant is a role this chart writes out — no `cluster-admin`, and no
built-in `view`, `edit` or `admin`. Read them in
`chart/templates/_helpers.tpl`: `runtimeReadRules` and `runtimeWriteRules`. They are lists, deliberately, so you can see every verb
without resolving an aggregated role.

| Grant | Covers |
|---|---|
| **Reads** | pods and their logs, services, endpoints, ConfigMaps, PVCs, events, workloads, ingresses, PDBs, HPAs, nodes, namespaces, storage classes, CRDs, node metrics, `nodes/proxy` for kubelet stats, and `roles`/`rolebindings` so an agent can explain a `Forbidden` |
| **Writes** | delete or evict a pod, scale, cordon a node, delete a workload, and create or edit ConfigMaps, Services, Ingresses, NetworkPolicies, PDBs, HPAs, PVCs |
| **Gated writes** | create or patch a pod, `pods/exec`, and create or patch a workload — only with `allowPodExecution` |

What they will never grant:

- **Any verb on `secrets`.** Not `get`, not `list`, not `watch`.
- **A `*` in `resources` or `apiGroups`.** A wildcard reaches Secrets without
  naming them.
- **`escalate` or `bind` on RBAC.** A role that can widen itself makes every
  line above advisory.
- **Any write on RBAC or CRDs.** Reading a Role is how an agent explains a
  `Forbidden`. Writing one is how it grants itself one.
- **Cluster-scoped RBAC reads.** `clusterroles` is a map of every identity in
  the install and which one is worth attacking.

**Verify it rather than trusting this page:**

```sh
kubectl auth can-i get secrets --as=system:serviceaccount:agent-ops:agentops-runtime
```

```powershell
kubectl auth can-i get secrets --as=system:serviceaccount:agent-ops:agentops-runtime
```

It must answer `no`. For a subresource use `--subresource=`, never the slash
form — `kubectl auth can-i` misparses `pods/eviction`.

#### Adding a grant the acting role omits

**Do not widen the shipped role.** Add your own, and attach it to the route that
needs it.

1. **Write your own `ClusterRole`** with the rules you need.
2. **Render an account for it** under `rbac.runtime.serviceAccounts`, with your
   role in `bindClusterRoles`.
3. **Name that account on the Pipeline** that needs it.

The grant is then deliberate, separately reviewable, and attached to one route
instead of every agent in the install.

**The role will be wrong at first, and it fails closed.** An agent is refused an
action and says so. Add verbs on evidence, one at a time.

### The runtime

```yaml
runtime:
  # the agent backend — swap it to change vendor
  image: ghcr.io/kostiantyn-matsebora/agentops-runtime-claude:0.8.0
  credentialsSecret:
    # read by the kubelet, never by the operator
    name: agentops-claude
  # set it when the runtime image is not multi-arch
  nodeSelector: {}

image:
  # the manager itself; its tag moves per release
  repository: ghcr.io/kostiantyn-matsebora/agentops-manager
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

### Who may reach what

Nothing restricts which pods may reach this release's components. Several of
them authenticate nobody.

- The **MCP servers** accept any caller. Under `rbacMode: full` the Kubernetes
  one holds the acting role above — an agent reaches the cluster through it, so
  it is the same wall.
- The **manager's work contract** takes no credential.
- The **console's API** answers below its authenticating proxy.

Two switches, both off by default, and they close different things.

| Key | Default | What it bounds |
|---|---|---|
| `global.agentops.networkPolicy.enabled` | `false` | **who may connect** — one policy per component, allowing only the callers your wiring implies |
| `runtime.egressMediation.enabled` | `false` | **what a connected agent may do** — the bound toolsets, enforced outside the agent |

**Network policy applies successfully on a cluster that does not enforce it, and
protects nothing.** There is no error. This chart cannot detect the difference,
so the install output tells you how to check rather than calling your components
protected.

Two things to name when you enable it, or they break quietly:

```yaml
global:
  agentops:
    networkPolicy:
      enabled: true
      # a collector outside this namespace, or metrics go silent
      metricsFrom:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: monitoring
      # your ingress controller, or the console becomes unreachable
      consoleFrom:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: ingress-nginx
```

### Enforcing the toolset

A route's toolsets reach the agent as `--allowedTools`, applied by the CLI in
the runtime pod. That configures a **cooperating** agent.

An agent that can run commands reaches a bound MCP server directly and calls
whatever that server registers. `agentops-shell` is bound on ordinary routes, so
this is the common case, not an exotic one.

`runtime.egressMediation.enabled` puts a proxy in the runtime pod that the
agent's traffic cannot route around, and enforces the bound toolsets there.

Know two things before enabling it.

1. It adds a **privileged init container** that installs the redirect and exits
   before the agent starts. A namespace under `restricted` Pod Security
   admission refuses it.
2. It adds a container per active conversation.

Two things it does not cover, by design:

- **stdio MCP servers**, which are child processes of the agent container.
- **`https` MCP endpoints**, which would need TLS interception inside the pod
  running the model's output.

Neither is passed off as enforced. The conversation's `EgressMediated` condition
names what is not covered.

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
tools.

**So does execution.** Add `runtimeRef` to choose what runs it, and
`serviceAccountName` to choose the identity it runs as. Both are optional, and
omitting them resolves the `default` runtime and that runtime's own account.

The fields are in
[concepts](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/concepts.md).

## Upgrade and uninstall

```sh
helm upgrade agent-ops \
  oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator \
  --version <version> -n agent-ops
```

```powershell
helm upgrade agent-ops `
  oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator `
  --version <version> -n agent-ops
```

Read [CHANGELOG.md](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/CHANGELOG.md)
first. It is the only place migration steps live, newest first, keyed by chart
version.

An uninstall removes the workloads. With `crds.keep: true` it leaves the CRDs,
every Conversation and the session claim — so reinstalling finds your data
where it was.
