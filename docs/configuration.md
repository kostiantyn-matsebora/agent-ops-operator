# Configuration reference

Every value the chart takes, in full. **[Installation](https://kostiantyn-matsebora.github.io/agent-ops-operator/installation/)
is the page to start from** — it covers installing, wiring a route and the
handful of values most installs set. This page is what you read when you need
the rest.

For the exhaustive value list the chart itself carries:

```sh
helm show values oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator
```

## Storage

### Pointing a volume at storage the chart did not create

**Both volumes accept all three forms:**

| Form | Key | The chart |
|---|---|---|
| a claim you already made | `existingClaim` | renders no claim, references yours |
| a `PersistentVolume` by name | `volumeName` | renders a claim bound to it |
| a `PersistentVolume` by label | `selector` | renders a claim carrying that selector |

**Naming a volume is not enough on its own.** A claim binds to a pre-created
volume only when it declines a storage class, and an absent `storageClassName`
is filled in by the cluster's default — which provisions a second volume and
leaves yours untouched. `-` is how a claim declines a class:

| `storageClassName` | Renders |
|---|---|
| undefined or empty | no field — the cluster's default provisioner |
| `-` | `storageClassName: ""` — no class, bind to a pre-created volume |
| a name | that class |

```yaml
persistence:
  context:
    volumeName: agentops-context-pv
    storageClassName: "-"
```

{: .ao-callout}
> **`-` is for a volume you made by hand, not for a retained one**
>
> A dynamically provisioned volume keeps its `storageClassName` forever, so a
> claim asking for `""` against one is refused with `VolumeMismatch:
> storageClassName does not match` and sits `Pending`, which looks exactly like
> a missing provisioner.
>
> Name the class the volume already carries:
>
> ```sh
> kubectl get pv <name> -o jsonpath='{.spec.storageClassName}'
> ```
>
> A claim's spec is immutable, so a wrong first attempt needs the claim deleted
> rather than another `helm upgrade`.

**The chart never renders a `PersistentVolume`.** Create it yourself — a
node-affine `local` PV is the usual answer on a cluster with no dynamic
provisioner.

### Per-route storage

**Persistence is wiring**, so a route declares its own beside the tools it
grants and the account it runs as. Leave it out and the route takes the
release-wide volumes above, which is what nearly every install wants.

```yaml
pipelines:
  - name: k8s-ops
    profile: k8s-engineer
    contextClaim: k8s-ops-context      # a claim that already exists
  - name: ha-ops
    profile: ha-engineer
    contextVolume: pv-ha-context       # a PersistentVolume the MANAGER
                                       # renders a claim on
```

| Key | Who renders the claim |
|---|---|
| `contextClaim` / `workspaceClaim` | nobody — it already exists |
| `contextVolume` / `workspaceVolume` | **the manager**, as `agentops-<route>-<volume>` |
| `contextSize` / `workspaceSize` | shapes that rendered claim |

**Naming both a claim and a volume is refused by the API server.** They decide
who creates the claim, so both is two answers rather than a preference.

**Two routes on one `AgentRuntime` can keep their state on different volumes.**

{: .ao-callout}
> **A claim the manager creates OUTLIVES the route.** It carries no ownerRef and
> the manager holds no `delete` verb on claims, so deleting a Pipeline never
> deletes the accumulated context of the conversations it started. Removing it
> is yours to do deliberately:
>
> ```sh
> kubectl -n agent-ops delete pvc agentops-<route>-context
> ```

### Surviving a damaged volume

A shared volume's filesystem can be corrupted by a node reboot, and the storage
layer will still call it healthy — it replicates blocks and cannot see a
filesystem.

Three settings limit what that costs, and they are independent.

| Key | Default | What it does |
|---|---|---|
| `claude.contextSync.paths` | the reference runtime's paths — **on** | moves the live context to pod-local storage, leaving a snapshot on the volume |
| `rbac.drainAware` | `false` | releases idle agent pods from a cordoned node, so the filesystem unmounts before the reboot |
| `contextProbe.enabled` | `false` | hourly mount probe, so a damaged idle volume is found in an hour rather than at next use |

`contextSync` is the one that matters most, and it is the one that ships on. The
agent container gets ephemeral storage and **no mount of the durable volume at
all** — so a run already going survives the volume failing underneath it.

It needs `paths`, because only the runtime knows where its backend keeps
context. The bundle that ships the reference runtime states them, beside that
runtime's image and credential:

```yaml
claude:
  contextSync:
    paths: [".claude/projects/-data-workspace/**"]
```

**Not in `global.agentops.runtimeDefaults`.** Those are what every runtime
inherits, and an include list is one vendor's filesystem layout — running
another backend means replacing the paths with its own, in the same section that
carries its image and credential. Clearing them gives that runtime the volume
mounted directly, exactly as before this existed.

**The cost, paid on every conversation.** `$HOME` is pod-local in this
mode, so it is not only transcripts that live there — caches, tool state and
anything else the agent writes home are node ephemeral storage now, and they die
with the pod:

| Key | Default | Consequence |
|---|---|---|
| `global.agentops.runtimeDefaults.contextSync.liveSizeLimit` | `4Gi` | per conversation, on the node running it. Empty is unbounded, which is Kubernetes' own default |

Budget node ephemeral storage for `maxActiveConversations` × this, and raise it
before a run that checks out something large evicts itself.

`rbac.drainAware` costs the manager its only cluster-scoped permission — reading
nodes. It shrinks the window in which a reboot can corrupt the volume. It does
not close it.

### Silence means no power

**A Pipeline that names no `serviceAccountName` runs as the floor account, and
nothing is ever bound to it.** That agent can do nothing in the cluster. It
reaches only what its toolsets and MCP servers give it.

**Acting power is something a route opts into, by name:**

```yaml
rbac:
  runtime:
    serviceAccounts:
      - name: agentops-runtime-acting
        clusterRoles:           # rules you wrote, and a reviewer can read
          - name: workloads
            rules:
              - apiGroups: ["apps"]
                resources: ["deployments"]
                verbs: ["get", "list", "patch"]

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

- **The parent always renders the floor**, bound to nothing, whatever else you
  configure. That is what keeps it NAMEABLE to take one route back to nothing on
  an install whose inherited default carries rights.
- **Each bundle renders the accounts its own routes need**, scoped to what those
  routes do. `home-assistant` renders two with no Kubernetes RBAC at all, because
  neither of its routes touches the Kubernetes API.
- **`rbac.runtime.serviceAccounts`** is where you declare your own.
- **A name nothing backs fails at pod admission**, saying which account.

### The grant is cluster-wide

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

### `allowPodExecution` — read this before turning it on

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

### What the roles grant

Every grant is a role this chart writes out — no `cluster-admin`, and no
built-in `view`, `edit` or `admin`. Read them in
`chart/templates/_helpers.tpl`: `runtimeReadRules` and `runtimeWriteRules`. They are plain lists, so every verb is readable without resolving an
aggregated role.

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

### Adding a grant the acting role omits

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

**TWO BLOCKS, AND THE RULE SEPARATING THEM:**

| Block | Holds |
|---|---|
| `global.agentops.runtimeDefaults` | what EVERY runtime inherits |
| `runtimes:` | the runtimes that EXIST, each stating only what DIFFERS |

```yaml
global:
  agentops:
    runtimeDefaults:
      # the agent backend — swap it to change vendor
      image: ghcr.io/kostiantyn-matsebora/agentops-runtime-claude:0.8.3
      credentialsSecret:
        # read by the kubelet, never by the operator
        name: agentops-claude
        # supply `token` and the chart CREATES the Secret
        token: ""
      # set it when the runtime image is not multi-arch
      nodeSelector: {}

runtimes: []      # the `claude` bundle ships `claude`; `default` is its copy

image:
  # the manager itself; its tag moves per release
  repository: ghcr.io/kostiantyn-matsebora/agentops-manager
```

**The defaults are SUFFICIENT.** The model credential is the only value with no
defensible default, and therefore the only thing you must supply.

**Another vendor is a bundle, or an entry stating only its difference.** The
chart ships [Ollama](https://kostiantyn-matsebora.github.io/agent-ops-operator/runtimes/ollama/) and
[GitHub Copilot](https://kostiantyn-matsebora.github.io/agent-ops-operator/runtimes/copilot/) as bundles:

```yaml
ollama:
  enabled: true
  endpoint: http://ollama.ollama.svc:11434   # a server you already run
  model: qwen2.5:14b                         # optional while the server has one model

copilot:
  enabled: true
  credentialsSecret:
    token: placeholder-token                  # a GitHub token with Copilot access

pipelines:
  - name: house-ops
    runtimeRef: ollama            # the route selects it
  - name: k8s-observe
    runtimeRef: copilot
```

A vendor with no bundle is a `runtimes:` entry with its own image, and
whatever differs from the defaults.

**`default` is what a route naming no `runtimeRef` resolves to, and the chart
renders it as a copy of one runtime you declared.** Every runtime keeps its own
name. Which one is copied is `default: true` on that runtime — on a bundle or a
`runtimes:` entry — or, with none flagged, the first configured. Every runtime
is optional: the `claude` bundle is the first shipped and on by default, so it
is the default on a fresh install, and turning it off with another on moves the
default there with no rename. **No runtime at all FAILS the render** when a
route still needs one, naming the routes — rather than leaving conversations in
`Pending` forever with the reason in the manager's log. So do two flags.

**Why the defaults live under `global.`** — a forcing, not tidiness. A subchart
reads no parent scope but that one, and a bundle-shipped runtime has nowhere
else to inherit from. `allowPodExecution` is read from there by a parent helper
the Kubernetes bundle CALLS, where only `.Values.global` resolves.

A wrong credential fails late and quietly. The pod is created, then sits in
`CreateContainerConfigError` while conversations queue behind it.

### Who may reach what

Nothing restricts which pods may reach this release's components. Several of
them authenticate nobody — the threat, and this control's honest limit, are on
[Security](https://kostiantyn-matsebora.github.io/agent-ops-operator/security/#network-segmentation).

- The **MCP servers** accept any caller. Under `kubernetes.allowMutations: true`
  the Kubernetes one holds an acting role — an agent reaches the cluster through
  it, so it is the same wall.
- The **manager's work contract** takes no credential.
- The **console's API** answers below its authenticating proxy.

Two switches, and they close different things. One is on by default.

| Key | Default | What it bounds |
|---|---|---|
| `global.agentops.networkPolicy.enabled` | `false` | **who may connect** — one policy per component, allowing only the callers your wiring implies |
| `global.agentops.runtimeDefaults.egressMediation.enabled` | **`true`** | **what a connected agent may do** — the bound toolsets, enforced outside the agent |

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
the runtime pod. That configures a **cooperating** agent. What that leaves open
is on
[Security](https://kostiantyn-matsebora.github.io/agent-ops-operator/security/#egress-control).

An agent that can run commands reaches a bound MCP server directly and calls
whatever that server registers. `agentops-shell` is bound on ordinary routes, so
this is the common case, not an exotic one.

`global.agentops.runtimeDefaults.egressMediation.enabled` puts a proxy in the
runtime pod that the agent's traffic cannot route around, and enforces the bound
toolsets there.

**IT IS ON BY DEFAULT.** The wall that constrains an agent that does not
cooperate should not be something you have to discover.

Know two things about the cost.

1. It adds a **privileged init container** (`NET_ADMIN`) that installs the
   redirect and exits before the agent starts. **A namespace under `restricted`
   Pod Security admission REFUSES it** — at POD ADMISSION, when a conversation
   starts, not at render. Turn it off before installing there:

   ```yaml
   global:
     agentops:
       runtimeDefaults:
         egressMediation:
           enabled: false
   ```

2. It adds a container per active conversation.

**A single runtime can decline it** instead, with `egressMediation.enabled:
false` on its `runtimes:` entry or on the `claude:` bundle — a vendor reaching
no MCP server has nothing to mediate.

Two things it does not cover, by design:

- **stdio MCP servers**, which are child processes of the agent container.
- **`https` MCP endpoints**, which would need TLS interception inside the pod
  running the model's output.

Neither is passed off as enforced. The conversation's `EgressMediated` condition
names what is not covered.

### Capacity

| Key | Default | Consequence |
|---|---|---|
| `maxActiveConversations` | `5` | how many agents hold a pod at once. Over-cap work waits in `Pending` with no pod and no thread |
| `maxQueuedConversations` | `50` | the backlog bound. Past it, new signals are declined and the sender is told |
| `global.agentops.runtimeDefaults.idleTtlMinutes` | `1` | how long a finished agent keeps its pod. Raise it for expensive startup |

### The agent's power

What this bounds, and what it does not, is on
[Security](https://kostiantyn-matsebora.github.io/agent-ops-operator/security/#cluster-authorization).

| Key | What it does |
|---|---|
| `rbac.runtime.serviceAccounts` | the identities this install DECLARES, each with its own posture, for a Pipeline to NAME. Default `[]` — the only source of runtime permissions |
| `global.agentops.runtimeDefaults.serviceAccountName` | the identity every route INHERITS. A REFERENCE this chart does not create. Default `agentops-runtime`, the floor |
| `global.agentops.runtimeDefaults.allowPodExecution` | may it create or enter a pod. Default `false` |

Each `serviceAccounts` entry takes:

| Field | Default | Is |
|---|---|---|
| `name` | — | what a Pipeline's `serviceAccountName` refers to |
| `create` | `true` | `false` references an account you own |
| `clusterRoles` | `[]` | rules to create and bind |
| `bindClusterRoles` | `[]` | existing ClusterRoles to bind |
| `namespaced` | `[]` | Roles in other namespaces |

**An entry stating none of the three is created holding nothing** — a named
identity for an audit, with no grant.

Start from `agentops.runtimeReadRules` and `runtimeWriteRules` in
`chart/templates/_helpers.tpl` and copy what you want. Read what you copy: where
the helper emits the write rules they are gated by `allowPodExecution`, and a
hand-written copy is not.

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

**The CRDs are not values.** They live in the chart's `crds/` directory, which
Helm applies before anything else — that is what lets one release install the
CRDs *and* the Pipelines and Channels that are instances of them.

| Was | Now |
|---|---|
| `crds.enabled: false` | `helm install --skip-crds` |
| `crds.keep: true` | inherent — Helm never deletes CRDs it installed from `crds/` |

**Helm never upgrades them either.** When a release changes a CRD field, its
entry in [CHANGELOG.md](https://github.com/kostiantyn-matsebora/agent-ops-operator/blob/master/docs/CHANGELOG.md) says so and gives you the `kubectl apply`
line. Nothing else in the chart needs that treatment.
