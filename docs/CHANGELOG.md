# Changelog

All notable changes to this project are documented here, in
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) format. Versions are
CHART versions. Image tags move independently and are named in each entry.

This file holds the **ten most recent versions**. Older entries are in
`changelog/`, linked at the [foot of this page](#older-versions).

See [../README.md](../README.md) for the product overview and [./](./) for
reference material. `CLAUDE.md` in this directory owns the rules this file
follows.

## [13.1.0] — 2026-08-26

**A second runtime: `agentops-runtime-ollama` 0.1.0, shipped as the `ollama`
bundle.** A local-model runtime over an Ollama endpoint you already run, in
which the RUNTIME is the harness — the agent loop, tool dispatch, the
transcript and the context handle are its own, and Ollama is called only for
the next message. Building it needed no manager, CRD or work-contract change,
which is the finding: the contract is vendor-neutral.

### Added

- `chart/charts/ollama/` — OFF by default. `ollama.enabled: true` with
  `ollama.endpoint` and `ollama.model` renders one `AgentRuntime` named
  `ollama` through the parent's shared renderer, inheriting
  `global.agentops.runtimeDefaults`. A route selects it with
  `pipelines[].runtimeRef: ollama`. The bundle deploys no model server, and
  the render FAILS naming the key when either value is missing.
- The runtime implements the six built-in tools natively — `Read`, `Grep`,
  `Glob`, `Edit`, `Write`, `Bash` — so `agentops-observe` / `-shell` / `-edit`
  mean the same thing on both runtimes, connects the bound MCP servers from the
  same `mcp.json`, and keeps context as one transcript per conversation under
  `$HOME/.agentops/contexts/`, declared to `context-sync` by the bundle.
- `docs/integrations/ollama.md`, and the Ollama chip on the landing page.

**Nothing changes for an install that does not enable the bundle.** The
rendered manifest is identical.

## [13.0.1] — 2026-08-24

**Every component image is now published BY CI, from a tag.**

### Changed

Ten components moved to versions the release workflow built and pushed:
`channel-telegram` 0.24.3, `console` 0.38.1, `context-sync` 0.2.2,
`egress-proxy` 0.2.3, `gateway-telegram` 0.5.2, `housekeeping` 0.2.2,
`signal-alertmanager` 0.7.2, `signal-ha` 0.2.2, `signal-k8s-events` 0.4.2,
`signal-telegram` 0.6.2.

**No behaviour changes.** The images are built from the same source as the
versions they replace; what is new is HOW they got there. The previous ones were
pushed by hand before this repository was public, so nothing in the tree
recorded which commit produced them and the registry's immutability rule meant
a tag could never be added after the fact — only a new version could carry one.

Component tags exist for that provenance and deliberately create no GitHub
Release: the chart version is the one an adopter types.

## [13.0.0] — 2026-08-24

**The one-flag demo works on a laptop cluster, with its memory intact.**

### Changed

**`persistence.context.accessModes` and `.workspace.accessModes` ship EMPTY, and
the chart answers them.** `ReadWriteMany` for an ordinary install, exactly as
before; `ReadWriteOnce` under `global.demo.enabled`.

The demo failed on every cluster a reader is likely to try it on. `local-path`
is the only storage class rancher-desktop, k3d, kind and minikube ship, and it
REFUSES an RWX claim:

```
failed to provision volume with StorageClass "local-path":
NodePath only supports ReadWriteOnce and ReadWriteOncePod (1.22+) access modes
```

The claim sat `Pending`, no runtime pod was ever created, and the conversation
waited forever. Getting started documented the workaround —
`persistence.context.enabled=false` — which buys a working demo by taking away
the thing the demo is showing.

- **It is not about node COUNT.** Any cluster whose only provisioner is RWO-only
  hits it; a single-node cluster with an RWX provisioner never does.
- **RWO is CORRECT under demo, not merely tolerable.** Many pods share an RWO
  volume on one node and the volume's affinity is what puts them there, so the
  default cap of five concurrent conversations still holds.
- **An explicit value wins in both modes.** The default is empty rather than a
  mode because an empty list is the one thing a chart can tell apart from a
  choice somebody typed.

### Upgrading

**A DEMO install whose context claim already BOUND cannot be upgraded in place.**
A PVC's `accessModes` is immutable, so Helm's patch is refused by the API server
and the upgrade fails naming the field.

Two ways out, and the first is the one a demo deserves:

```sh
# the demo is disposable — reinstall it
helm uninstall agent-ops -n agent-ops && kubectl delete ns agent-ops

# or keep the claim exactly as it is
helm upgrade ... --set 'persistence.context.accessModes={ReadWriteMany}'
```

**No ordinary install is affected.** Outside demo mode the rendered claim is
`ReadWriteMany` before and after, so there is no patch to refuse.

## [12.0.0] — 2026-08-24

**Context synchronisation is ON by default**, and the mode no longer builds a
pod it cannot serve.

### Changed

**The reference runtime declares its context paths, so a default install runs
synchronised.** `chart/charts/claude/values.yaml` ships
`contextSync.paths: [".claude/projects/-data-workspace/**"]`.

The mechanism existed because a node reboot corrupted the shared context
filesystem on 2026-08-20, taking every conversation's context AND stopping every
runtime pod from starting. It shipped inert: the correct value sat commented out
two lines beneath an empty list, waiting for an operator to type a fact the
runtime already holds.

**It is in the VENDOR'S BUNDLE, not `global.agentops.runtimeDefaults`** — beside
that runtime's image and model credential, which moved there in 11.0.0 for the
same reason. An include list is one vendor's filesystem layout: it describes
where claude-code files transcripts and means nothing to another backend.
Running one means replacing the paths with its own, in the same section.

### Fixed

**A runtime declaring `contextSync` on an install with no context volume built a
pod that could not start.** The sidecar branch mounted the durable claim
unconditionally, so with persistence off it rendered a `PersistentVolumeClaim`
reference whose name was the empty string — refused at admission, no pod, and a
conversation with no phase.

Two things hid it. Every test shared one config that set a claim, so the
combination was never constructed. And continuity resolution got it RIGHT — it
reports no promise without a volume — so the manager correctly told the
conversation it would answer fresh, then failed to provide one.

**The fix is one conjunct**: no durable volume means no sidecar, and the pod is
exactly today's unsynchronised pod with ephemeral context. One fallback rule,
three conditions — no declaration, no sidecar image, no volume — rather than one
rule with an exception.

Rare before, because it needed someone to declare `contextSync` while running
without persistence. With the default on it would have been every conversation
of every persistence-disabled install, which is why the fix landed first.

### BREAKING — existing conversations lose their context handles

**Synchronisation relocates the durable layout** from the claim root to a
per-conversation path. Nothing copies a volume, so a conversation holding a
context handle looks for it under a path that has nothing in it.

**It FAILS that conversation's next run rather than answering without memory** —
the continuity rule working, not a defect. The recovery is the reset verb, which
clears the handle and keeps the conversation, its threads, its inputs and its
recorded runs:

```
POST /channel/conversations/{name}/reset-context
```

**No migration is provided and none is owed.** This project is pre-1.0 and
unpublished, and the decision on record is that existing conversations are not
preserved. Enable it on a quiet install, or reset the conversations that fail.

**Rolling back has the same cost in reverse.** Clearing `claude.contextSync.paths`
restores the direct mount, and handles written under the per-conversation path
are not found there.

### The cost of the default, stated rather than discovered

**`$HOME` is pod-local on every conversation now.** It is not only transcripts
that live there — caches, tool state and anything else the agent writes home are
node ephemeral storage, and they die with the pod.
`runtimeDefaults.contextSync.liveSizeLimit` bounds it at `4Gi` per conversation.
Budget node ephemeral storage for `maxActiveConversations` × that.

### Upgrade

1. `helm upgrade`. Nothing to restate — the default arrives with the chart.
2. Expect conversations holding a context handle to fail their next run; reset
   each with the verb above.
3. Turning it back off is `claude.contextSync.paths: []`, with the same
   relocation cost in reverse.

**Running without a context volume?** Nothing to do. Synchronisation is skipped
because there is nothing to snapshot to, and conversations answer fresh and say
so — which is what they already did.

## [11.0.0] — 2026-08-24

**No account exists unless something is bound to it or something authenticates
as it.** Rendering the reference install found four ServiceAccounts bound to
nothing, a preset posture the spec had already banned, and one vendor's
credential in the block every vendor reads.

### Removed

**`rbac.runtime.serviceAccounts[].rbacMode` is DELETED, with no alias.** The
render FAILS naming what to write instead.

Release 10.0.0 deleted `global.agentops.runtime.rbacMode` and wrote "there shall
be no preset posture" into the spec — then shipped the identical mechanism one
level down. A reviewer reading `rbacMode: full` sees a word, not the verbs, and
declaring the account rather than defaulting it does not make the word readable.

It was also a second cluster-write path: the runtime pod mounts its token and an
acting route binds a shell, while the runtime image ships no kubectl precisely
so cluster reach goes through the MCP server and its toolset split.

```yaml
# before
rbac:
  runtime:
    serviceAccounts:
      - name: agentops-runtime-acting
        rbacMode: full

# after — rules you wrote, and can read
rbac:
  runtime:
    serviceAccounts:
      - name: agentops-runtime-acting
        clusterRoles:
          - name: workloads
            rules:
              - apiGroups: ["apps"]
                resources: ["deployments"]
                verbs: ["get", "list", "patch"]
```

`agentops.runtimeReadRules` and `agentops.runtimeWriteRules` in
`chart/templates/_helpers.tpl` are what the modes expanded to — copy from them.
**Read the write rules before copying them:** where the helper emits them they
are gated by `runtimeDefaults.allowPodExecution`, and a hand-written copy is not.

**`SignalAdapter.spec.kubernetesAccess` and `ChannelAdapter.spec.kubernetesAccess`
are DELETED.** Replaced by `spec.serviceAccountName`, and naming an account is
what mounts its token — the two were always one decision. `POD_NAMESPACE` is now
injected unconditionally; it is a downward-API field, not a permission.

**A CRD FIELD WAS DELETED, so `kubectl apply -f chart/crds/` is a step for this
upgrade AND for a fresh install.** Helm installs CRDs from `crds/` only when
absent and never upgrades one. The API server prunes an unknown field silently.

### Changed

**The reference runtime's image and model credential moved to the `claude:`
bundle.** The render FAILS on either key left in `global.agentops.runtimeDefaults`.

```yaml
# before
global:
  agentops:
    runtimeDefaults:
      credentialsSecret:
        token: <your token>

# after
claude:
  credentialsSecret:
    token: <your token>
```

They are not silently ignored, they are worse: left in the defaults they still
merge into EVERY runtime, so a non-Claude backend inherits
`CLAUDE_CODE_OAUTH_TOKEN`. What remains in `runtimeDefaults` is vendor-neutral,
and the parent `values.yaml` now carries a documented `claude:` section so
`helm show values` shows it.

**A bundle renders a route's ServiceAccount only where it also grants that route
something.** `agentops-ha-ops`, `agentops-ha-control` and the `kubernetes`
bundle's route accounts held no RBAC at all — indistinguishable from the floor
every unnamed route already inherits, while adding names to every audit. Those
routes now name nothing and inherit the floor.

**The MCP servers for `home-assistant` and `prometheus` render no ServiceAccount.**
Both mount no token, so the identity was never presented to the API server.
`agentops-mcp-k8s` keeps its account: it mounts its token and carries the grant.

**Adapter ServiceAccounts are the CHART's now, not the manager's.** The adapter
reconciler was the only one in the project that created a ServiceAccount, and it
was forbidden from binding RBAC to what it created. The chart that grants an
adapter renders the account beside the grant, and the CR names it.

**ORPHANED, NOT DELETED:** accounts the manager created carried an ownerRef on
the adapter CR. After this upgrade nothing creates or owns them. Nothing is bound
to them either, so nothing breaks — remove them when convenient:

```sh
kubectl -n <ns> delete sa agentops-adapter-telegram agentops-signal-home-assistant \
  agentops-signal-telegram agentops-mcp-ha agentops-mcp-prometheus
```

An adapter naming no account now runs as the release floor with its token
mounted — an authenticated identity denied every verb, rather than an anonymous
one a forgotten grant looks identical to.

### Upgrade

1. `kubectl apply -f chart/crds/` — a CRD field was deleted.
2. Move `global.agentops.runtimeDefaults.credentialsSecret.token` to
   `claude.credentialsSecret.token`. Same for `.image` if you pinned one.
3. Replace any `rbacMode` with explicit `clusterRoles` — or drop the account and
   the `serviceAccountName` naming it, if the route's cluster reach is its MCP
   server's (it is, with the shipped runtime, which has no CLI).
4. `helm upgrade`. Every retired key fails the render naming its replacement, so
   nothing is silently ignored.
5. Optionally delete the orphaned accounts listed above.

## [channel-telegram 0.24.2] — 2026-08-24

### Fixed

**A signal card larger than about 4KB never arrived**, and the delivery retried
in a loop. The agents' own answers were unaffected — only event cards failed.

Telegram answered:

```
can't parse entities: Can't find end tag corresponding to start tag "blockquote"
```

**Cause.** A signal payload over six lines is folded into
`<blockquote expandable>`, which spans many lines. The splitter chooses a chunk
boundary at a newline, on an assumption stated in its own comment — that every
tag it emits opens and closes on the same line.

That was false for exactly this tag. The first chunk carried an opening tag with
no end and the second a stray end tag, and the Bot API rejects the **whole
message** rather than the tag.

The adapter already solved this one path over: a split fold in an agent's answer
has each piece re-wrapped in its own quote. A signal is not agent output, so it
never reached that code.

**Fix.** The splitter closes a quote it cuts and reopens the same tag on the
remainder, and reserves room for the end tag up front — adding it after choosing
a cut would push the chunk past the 4096 limit, trading one rejected message for
another.

**Still true after this:** a message the Bot API will never accept retries
forever. The latch that disables expandable quotes recognises a refusal about an
*unsupported* tag, and this one was *unbalanced*. This removes the trigger, not
the poison-pill behaviour.

### Upgrade

Nothing to do. The chart pins the new tag.

## [10.0.0] — 2026-08-24

**Every subchart is renamed, the release-wide permission mode is DELETED, and
the runtime values block splits in two.** This is the widest values break this
chart has shipped: every operator has set at least one renamed key.

**Nothing is silently ignored.** Every retired key FAILS the render naming its
replacement, and the guard needs no cluster — so a GitOps render refuses too.
Helm never reports an unread values key, and the quiet outcome is the expensive
one: a bundle that simply does not render is indistinguishable from an operator
who meant to leave it off.

### The five things that will break, in the order they bite

1. **The subchart keys.** `k8s-bundle` → `kubernetes`, `ha-bundle` →
   `home-assistant`, `prometheus-bundle` → `prometheus`, `telegram-bundle` →
   `telegram`.
2. **`global.agentops.runtime.rbacMode` is gone**, with no alias. An install
   that had `full` now grants NOTHING until it declares an account.
3. **`runtime:` splits** into `global.agentops.runtimeDefaults` and `runtimes:`.
4. **`runtimeIdleTtlMinutes` moves** into the defaults block.
5. **Egress mediation is ON**, which a `restricted` Pod Security namespace
   refuses — at POD ADMISSION, not at render.

### Changed

**BREAKING — every subchart is named for the SYSTEM it integrates**, and the
`-bundle` suffix is dropped.

| Was | Now |
|---|---|
| `k8s-bundle:` | `kubernetes:` |
| `ha-bundle:` | `home-assistant:` |
| `prometheus-bundle:` | `prometheus:` |
| `telegram-bundle:` | `telegram:` |

`k8s` and `ha` alone are not descriptive, and both collide in READING with the
`k8s-ops`, `k8s-observe`, `ha-ops` and `ha-control` PIPELINE names an install
declares. The same string on two kinds of object is what the naming rules exist
to prevent.

Published IMAGE names do not change. Only values keys and chart directories do.

**BREAKING — the runtime values block splits in two**, and the rule separating
them is now stateable in two lines:

| Block | Holds |
|---|---|
| `global.agentops.runtimeDefaults` | what EVERY runtime inherits — a COMPLETE, working configuration |
| `runtimes:` | the runtimes that EXIST, each stating only what DIFFERS |

`runtime:` named one thing, `global.agentops.runtime:` another and
`rbac.runtime:` a third, and no page stated which was which.

**The defaults are SUFFICIENT.** The model credential is the only value with no
defensible default, and therefore the only thing an install must supply.

**`resources` is written out** — 100m/256Mi requests, 1/1536Mi limits. The
numbers already existed, compiled into the operator where no operator could read
or tune them. Behaviour is unchanged on every install.

**BREAKING — `runtimeIdleTtlMinutes` moves** to
`global.agentops.runtimeDefaults.idleTtlMinutes`, for the same reason: a
bundle-shipped runtime cannot read a parent-scope value, so it rendered an EMPTY
field and the CRD's structural default of 10 silently replaced the release's
setting.

**BREAKING — the reference runtime becomes the `claude` bundle**, ON by default.
An install using another vendor turns it off and stops carrying it.

**BREAKING — egress mediation defaults to ON.** The wall that constrains an
agent that does not cooperate should not be something an operator discovers.

**IT COSTS A PRIVILEGED INIT CONTAINER** (`NET_ADMIN`), which a namespace under
`restricted` Pod Security admission REFUSES — at POD ADMISSION, when a
conversation starts, far from the setting responsible. The chart cannot see your
namespace's Pod Security level, so the notes say this rather than guessing.

Decline it release-wide, or on one runtime:

```yaml
global:
  agentops:
    runtimeDefaults:
      egressMediation:
        enabled: false
```

**BREAKING — the Kubernetes bundle's four consequences become ONE STATED
SETTING**, `kubernetes.allowMutations`, which is the bundle's own:

1. the MCP server drops `--read-only`, so mutating tools are REGISTERED
2. that server's ServiceAccount gets the acting grant instead of the reads
3. the `k8s-admin` mutating toolset renders
4. the ACTING route ships instead of the observing one

They moved together before too, driven by `rbacMode` — a release-wide value
whose name mentioned none of the four. Each stays individually overridable, and
none derives from another now.

### Removed

**BREAKING — `global.agentops.runtime.rbacMode` is DELETED**, with no alias and
no migration path that preserves its meaning.

**THE DEFAULT IS NOW NO PERMISSIONS, FULL STOP.** No setting widens it.

It rendered an extra, named ServiceAccount carrying a preset posture, and that
account granted nothing until a `Pipeline` named it. So the name described a
mode the runtime was in — which is what it USED to mean, and the reading that
caused the incident it was reverted for.

Its one load-bearing behaviour was demo mode resolving empty to `readonly`. That
does not survive scrutiny, and it was MEASURED before removal: an agent reaches
the cluster through the MCP SERVER, which carries its own account and its own
grant, and the runtime image ships no `kubectl`. On a clean demo install the
runtime pod's account holds no ClusterRoleBinding and is denied `list
namespaces`, while the answer still carries real cluster data.

**BREAKING — `rbac.runtime.clusterRoles`, `.bindClusterRoles` and `.namespaced`
are removed.** They attached to the account the mode rendered and have no other
target. They survive PER ACCOUNT, on a `rbac.runtime.serviceAccounts` entry.

**BREAKING — `serviceAccountName` names the DEFAULT account and is a REFERENCE
this chart does not create.** Naming is not creating, the posture adapters
already have.

Two accounts can then exist, and the second is the useful half:

| Account | Created by | Is |
|---|---|---|
| `agentops-runtime` | ALWAYS, the chart | bound to nothing |
| whatever `serviceAccountName` names | **you** | the default a Pipeline inherits |

So an install that points the default at its own account keeps
`agentops-runtime` available to NAME on one Pipeline and take that route back to
nothing.

### Added

**A bundle may ship a runtime**, declaring it in its own values and rendering
its own CR, exactly as bundles already ship pipelines.

**A new guard replaces a deleted invariant.** "The parent always renders
`default`" cannot hold once the runtime ships in a bundle an operator may turn
off. In its place: **the render FAILS when no runtime answers to `default` and a
route still resolves to it**, naming both the missing runtime and the routes.

It reads no cluster, so it protects a GitOps install exactly as it protects an
interactive one. Routes naming their own `runtimeRef` need no default, and the
check knows that.

### Upgrade

**0. APPLY THE CRDs.** No CRD field changed in this release, but the rule has
not: Helm installs a CRD from `crds/` only when absent and never upgrades one.

```sh
kubectl apply -f chart/crds/
```

**1. Rename every subchart key.** Nothing else in those blocks changes.

```yaml
# before                    # after
k8s-bundle:                 kubernetes:
ha-bundle:                  home-assistant:
prometheus-bundle:          prometheus:
telegram-bundle:            telegram:
```

**2. Rewrite the runtime block.**

```yaml
# before
runtime:
  image: ghcr.io/.../agentops-runtime-claude:0.8.0
  contextSync:
    paths: [".claude/projects/-data-workspace/**"]
  credentialsSecret:
    token: <ref>
runtimeIdleTtlMinutes: 1

# after
global:
  agentops:
    runtimeDefaults:
      image: ghcr.io/.../agentops-runtime-claude:0.8.0
      idleTtlMinutes: 1
      contextSync:
        paths: [".claude/projects/-data-workspace/**"]
      credentialsSecret:
        token: <ref>
```

`runtimes:` stays EMPTY unless you declare a second vendor — the `claude` bundle
ships the one named `default`.

**3. Replace `rbacMode` with a DECLARED account.** This is the step that changes
what your agents can do, so read it before syncing.

```yaml
# before
global:
  agentops:
    runtime:
      rbacMode: full

# after
rbac:
  runtime:
    serviceAccounts:
      - name: agentops-runtime-acting
        rbacMode: full          # the same vocabulary, per account

pipelines:
  - name: k8s-ops
    serviceAccountName: agentops-runtime-acting   # NAME it on the routes
```

**IF YOU SKIP THE `pipelines` HALF, THOSE ROUTES SILENTLY LOSE THEIR CLUSTER
POWER.** They keep working — an agent reaches the cluster through the MCP
server — but anything using the account's own credentials stops.

**IF YOU WERE ALREADY NAMING `agentops-runtime-acting` on your Pipelines**, keep
those lines exactly as they are and add only the `rbac.runtime.serviceAccounts`
entry above. The account name is unchanged.

**4. Decide about egress mediation.** It is ON now. If this namespace runs under
`restricted` Pod Security admission, turn it off before syncing — otherwise the
first conversation after the upgrade fails at pod admission.

**5. Move `rbac.runtime.{clusterRoles,bindClusterRoles,namespaced}` onto the
account** you declared in step 3.

**6. Set `kubernetes.allowMutations: true`** if you had `rbacMode: full` and
relied on the acting route, the write-capable MCP server or the `k8s-admin`
toolset. Those four followed the mode and follow this now.

**Then sync.** Any key you missed fails the render naming its replacement.

## [9.0.0] — 2026-08-24

**Persistence is WIRING. It moves off `AgentRuntime` and onto `Pipeline`**, and
the volume it names is the CONTEXT volume everywhere — object, field, values
block, claim and MOUNT PATH.

A runtime is an ENGINE: an image and its pod-level defaults.

WHERE a route's conversations keep their state is a property of the ROUTE. It is
decided beside the tools it grants, the channels it delivers to, the runtime it
selects and the identity it executes under — all four already `Pipeline`
fields.

**Two Pipelines sharing one runtime can now keep their conversations on
different volumes without cloning that runtime** — which is exactly what
expressing a second trust level used to require, and was fixed the same way.

**READ THIS BEFORE UPGRADING A GITOPS INSTALL.** The chart carries a guard that
refuses the upgrade where it can see the old claim.

That guard reads the CLUSTER, and Argo CD, Flux, `helm template` and CI all
render without one. For those installs this entry is the only warning that
arrives.

### The five things that will break, in the order they bite

0. **APPLY THE CRDs, WHATEVER YOU ARE DOING** — upgrade, reinstall, or a full
   wipe. See below: skipping it is the one failure here that reports success.
1. **An `AgentRuntime` still declaring the retired volume field contributes NO
   volume.** It is DELETED, not aliased.
2. **`runtime.contextPvcRef`, `runtime.homePvcRef` and
   `runtime.workspacePvcRef` FAIL the render.** They are gone, not renamed.
3. **The default claim is `agentops-context`.** Nothing copies a volume.
4. **The mount path is `/data/context`.** `/data/workspace` did NOT move.

### A FRESH INSTALL NEEDS THE CRDs TOO, AND THAT IS THE SURPRISING ONE

**Helm installs a CRD from `crds/` only when it is ABSENT, and never upgrades
one.** CRDs are CLUSTER-scoped, so they survive everything an install normally
tears down:

| What you did | CRDs after it |
|---|---|
| `helm upgrade` | untouched |
| `helm uninstall` | untouched |
| `kubectl delete ns agent-ops` | untouched |
| deleted the namespace AND reinstalled from scratch | **still the OLD ones** |

**So a clean wipe-and-redeploy lands on stale CRDs, and the API server then
PRUNES every field this release added** — `Pipeline.spec.persistence` and the
conversation's claim snapshot — without a warning anywhere.

Every conversation then resolves to EPHEMERAL and answers normally. The install
looks healthy and quietly keeps nothing.

**It was hit exactly this way** on a redeploy that had deleted the whole
namespace first, which is precisely the case that feels like it cannot need a
migration step.

```sh
kubectl apply -f https://raw.githubusercontent.com/kostiantyn-matsebora/agent-ops-operator/master/chart/crds/
```

```powershell
kubectl apply -f https://raw.githubusercontent.com/kostiantyn-matsebora/agent-ops-operator/master/chart/crds/
```

**Check it took**, because the symptom of missing it is silence:

```sh
kubectl get crd pipelines.agentops.dev -o jsonpath='{.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.persistence.type}'
```

```powershell
kubectl get crd pipelines.agentops.dev -o jsonpath='{.spec.versions[0].schema.openAPIV3Schema.properties.spec.properties.persistence.type}'
```

It prints `object`. An empty answer means the old CRD is still in place.

### Changed

- **BREAKING: `AgentRuntime.spec.home` and `spec.context` are DELETED, and so is
  `spec.workspace`.** No alias survives, and that is deliberate rather than
  harsh.

  A one-release dual-read is honest only where a field was renamed IN PLACE —
  which is what `sessionId` was, and reading this as the same case is the trap.
  Here the CONCEPT moved to a different CR: there is no replacement on
  `AgentRuntime` for an alias to resolve to. An alias would point at a field
  that is not on that object at all.

  **A runtime CR still carrying the retired field therefore contributes no
  volume after this upgrade.** Being told that plainly is recoverable. A quiet
  resolution to nothing is not.

  **Move the declaration to the Pipelines using that runtime:**

  ```yaml
  pipelines:
    - name: k8s-ops
      profile: k8s-engineer
      contextClaim: the-claim-the-runtime-used-to-name
  ```

  Or leave it out entirely and take the release-wide claim, which is what nearly
  every install wants.

  **`spec.contextStorage` STAYS on the runtime**, and it is the one storage
  question only a runtime can answer: whether its BACKEND writes context to a
  disk at all. A runtime keeping context at a vendor API needs no volume and
  must not be given one. The split is BACKEND SHAPE against PLACEMENT.

  This release changes three CRDs, and Helm does not upgrade CRDs:

  ```sh
  kubectl apply -f https://raw.githubusercontent.com/kostiantyn-matsebora/agent-ops-operator/master/chart/crds/
  ```

  ```powershell
  kubectl apply -f https://raw.githubusercontent.com/kostiantyn-matsebora/agent-ops-operator/master/chart/crds/
  ```

- **BREAKING: `runtime.contextPvcRef`, `runtime.homePvcRef` and
  `runtime.workspacePvcRef` are GONE**, and supplying any of them FAILS the
  render naming both places the declaration moved to. The rendered
  `AgentRuntime` declares no volume at all.

  Helm never reports an unread values key, so the alternative was silence — and
  the quiet case is the expensive one: an operator who deliberately pointed the
  runtime at a claim the chart did not create would keep every signal of success
  while the release-wide claim was used instead. This guard needs no cluster, so
  it fires under `helm template`, in CI and under a GitOps controller.

- **BREAKING: the default context claim is `agentops-context`**, renamed with
  the volume. **Nothing copies a volume**, so upgrading as-is would provision a
  second, EMPTY claim and every conversation in the install would answer without
  its accumulated context while every signal reported success.

  **The chart FAILS the render where it can see that outcome** — see *Upgrade*
  below for the two ways out, and note again that this guard reads the cluster
  and a GitOps renderer has none.

- **BREAKING: the MOUNT PATH moves to `/data/context`, and `HOME` with it.**

  **Nothing inside the volume moves, and this was measured rather than
  reasoned.** A claim's contents appear AT the mount path, so the same volume
  mounted elsewhere shows the same bytes. And the stored transcript directory
  is named for the WORKING directory (`-data-workspace`), not for `$HOME`. A
  live volume was mounted read-only at a third path to confirm it.

  **`/data/workspace` does NOT move.** THAT is the load-bearing path — the
  transcript directory is named for it, so relocating it would strand every
  stored context. An earlier draft of this change had the two the wrong way
  round.

- **BREAKING: the `persistence` block moved under `persistence.context`.** The
  workspace block is unchanged in shape and in place.

  | Was | Now |
  |---|---|
  | `persistence.enabled` | `persistence.context.enabled` |
  | `persistence.name` (`agentops-home`) | `persistence.context.name` (`agentops-context`) |
  | `persistence.size` | `persistence.context.size` |
  | `persistence.storageClassName` | `persistence.context.storageClassName` |
  | `persistence.accessModes` | `persistence.context.accessModes` |
  | `persistence.volumeName` | `persistence.context.volumeName` |
  | `persistence.existingClaim` | `persistence.context.existingClaim` |

  **Supplying any retired key FAILS the render**, naming where it moved, and
  this guard needs no cluster either.

- **BREAKING: `HOME_PVC` is GONE on the manager.** The bootstrap default is
  `CONTEXT_PVC`, and it is not dual-read, for the reason the CRD field is not:
  the concept moved. The chart supplies it, and the chart and the manager ship
  together in this release. Manager image **0.54.0**.

### Added

- **`Pipeline.spec.persistence`** — `context` and `workspace`, independently.
  Each binding names EITHER a claim that already exists OR a
  `PersistentVolume`, and the API server refuses both at once.

  | You set | Who renders the claim | The conversation gets |
  |---|---|---|
  | `contextClaim` | nobody — it exists | that claim |
  | `contextVolume` | **the manager** | the claim it created |
  | neither | — | the release-wide claim, then ephemeral |

  **A pod can mount only a claim, never a `PersistentVolume`** — so naming a
  volume requires that something render the claim on it. That is THE ONE PLACE
  in this system where naming a resource creates it, and it is stated rather
  than smuggled: the manager gains `persistentvolumeclaims` **get/list/watch/
  create**, and deliberately **no `delete`, `update` or `patch`**.

  **A claim the manager creates carries NO ownerRef on the Pipeline.** Deleting
  a Pipeline must never delete the accumulated context of the conversations it
  started — storage is the one thing here whose loss cannot be repaired by
  reconciling again. That is guarded twice: the absent ownerRef, and the absent
  verb.

- **The resolved claim is SNAPSHOTTED into the `Conversation`**
  (`spec.contextClaimName` / `spec.workspaceClaimName`), exactly as the
  execution identity is. Editing a Pipeline's persistence moves only
  conversations created afterwards.

  **This is the sharpest case of that rule on the object.** Without it, an edit
  changes which volume an INFLIGHT conversation's next pod mounts — work that
  has already WRITTEN to the old one, coming back to a different disk and
  reporting success.

- **Either volume can bind to a PersistentVolume the chart did not create.**
  Three forms, all three on BOTH volumes, at BOTH levels:

  | Form | You supply | The chart |
  |---|---|---|
  | existing claim | `existingClaim` | renders no claim, references yours |
  | volume by name | `volumeName` | renders a claim bound to it |
  | volume by label | `selector` | renders a claim carrying that selector |

  `selector` is new. `existingClaim` and `volumeName` already existed — but
  `volumeName` alone never actually bound anything, which is the fix below.

- **`storageClassName: "-"` disables dynamic provisioning**, following the
  convention prometheus-community, Bitnami and most charts already use:

  | Value | Renders |
  |---|---|
  | undefined or empty | no field — the cluster's default provisioner |
  | `-` | `storageClassName: ""` — no class, bind to a pre-created volume |
  | a name | that class |

  **Purely additive.** Empty keeps the meaning it has always had here, so no
  existing install changes behaviour.

### Fixed

- **A claim naming a pre-created volume is now actually bound to it.** Binding
  to a static `PersistentVolume` requires an EXPLICIT empty storage class. The
  template emitted `storageClassName` only when non-empty and the shipped
  default was `""`, so the field was omitted, the admission plugin filled it in
  from the cluster's default StorageClass, and the claim was dynamically
  provisioned against a volume you had already made. There was no spelling of
  "no storage class" at all. `-` is that spelling.

  **The chart still renders no `PersistentVolume`.** A pre-created volume is by
  definition not the release's to create.

### Upgrade

1. **Apply the CRDs** (command above). Helm does not upgrade them — and it does
   not replace them on a reinstall either, so this step is NOT optional just
   because you wiped the namespace first.
2. **Move any `AgentRuntime` volume declaration onto the Pipelines using that
   runtime**, as `contextClaim` / `workspaceClaim`. A runtime still declaring
   the retired field contributes nothing after this upgrade.
3. **Delete `runtime.contextPvcRef` / `runtime.homePvcRef` /
   `runtime.workspacePvcRef`** from your values. The render fails naming them.
4. **Rewrite any flat `persistence.*` values** under `persistence.context`, per
   the table above. The render fails naming them too.
5. **Decide what happens to your existing claim.** Check for it first:

   ```sh
   kubectl -n agent-ops get pvc agentops-home
   ```

   ```powershell
   kubectl -n agent-ops get pvc agentops-home
   ```

   **Either keep it under its old name** — one line, moves no data, and the
   chart then renders no claim of its own:

   ```yaml
   persistence:
     context:
       existingClaim: agentops-home
   ```

   **or REBIND the volume under the new name**, which also moves no data and is
   what actually finishes the migration:

   ```sh
   PV=$(kubectl -n agent-ops get pvc agentops-home -o jsonpath='{.spec.volumeName}')
   kubectl get pv "$PV" -o jsonpath='{.spec.storageClassName}'   # note this
   kubectl patch pv "$PV" -p '{"spec":{"persistentVolumeReclaimPolicy":"Retain"}}'
   kubectl -n agent-ops delete pvc agentops-home
   kubectl patch pv "$PV" --type=json -p '[{"op":"remove","path":"/spec/claimRef"}]'
   ```

   ```yaml
   persistence:
     context:
       volumeName: <the PV>
       storageClassName: <THE PV'S OWN CLASS, from the command above>
   ```

   **THE STORAGE CLASS LINE IS THE ONE THAT BITES, and `-` is the WRONG answer
   here.** A PV that was DYNAMICALLY PROVISIONED — which is what an existing
   agent-ops install has — keeps its `storageClassName` forever, and a claim
   requesting a different one is refused with `VolumeMismatch: storageClassName
   does not match`. The claim then sits `Pending`, looking exactly like a
   missing provisioner. `-` is for a STATICALLY created PV that has no class at
   all.

   **And a claim's spec is immutable once created**, so a first attempt that got
   the class wrong cannot be corrected by re-running `helm upgrade` — delete the
   wrong claim first.

6. **`helm upgrade`.**

**Rollback is a chart downgrade plus restoring the runtime's volume field**, and
the volume is untouched throughout, which is what makes it recoverable.

## [8.0.0] — 2026-08-23

**The chart could not be installed on a cluster that did not already have its
CRDs**, and it had been that way for the project's whole life. Every install
until now landed where a previous one had left the CRDs behind, so nothing ever
surfaced it.

**BREAKING: `crds.enabled` and `crds.keep` are gone.** Setting either now FAILS
the render naming the replacement, rather than being silently ignored.

| Was | Now |
|---|---|
| `crds.enabled: false` | `helm install --skip-crds` |
| `crds.keep: true` | inherent — Helm never deletes CRDs it installed from `crds/` |

### Fixed

- **The CRDs moved from `templates/` to the chart's `crds/` directory.** Helm
  applies that directory out-of-band, invalidates discovery and waits for the
  CRDs to establish BEFORE it builds the rest of the manifest.

  This chart ships eleven CRDs beside eight instances OF them — Pipelines,
  Channels, profiles, toolsets. Helm resolves every kind in a manifest before
  applying any of it, so as templates those instances could not map and a clean
  install aborted with `ensure CRDs are installed first`.

  Helm's own guidance names two methods and no third: this directory, or two
  separate charts. There is no annotation that orders resources within one
  release.

- **Cluster-scoped RBAC now carries the release namespace.** Every `ClusterRole`
  and `ClusterRoleBinding` the chart renders is suffixed with it, so two installs
  in one cluster no longer collide. Previously a second release failed with
  `ClusterRole "agentops-signal-k8s-events-events" … cannot be imported`, which
  made a side-by-side demo or a staging namespace impossible.

  ServiceAccount subjects are untouched — those are namespaced already.

- **The console no longer serves the wrong auth mode at startup.** It read its
  browser token only after the manager's channel listing arrived, and retried
  that listing on the steady 60-second cadence — so a fresh install prompted for
  a token for a full minute, even where a proxy authenticates instead.

  The credentials are projected into the pod before the process starts. The
  console now reads them from its own environment, and retries an unresolved
  listing every second rather than every minute. Console image **0.38.0**.

### Changed

- **Helm no longer upgrades the CRDs either**, which is the documented cost of
  the `crds/` directory. When a release changes a CRD field its entry here says
  so and gives the command:

  ```sh
  kubectl apply -f https://raw.githubusercontent.com/kostiantyn-matsebora/agent-ops-operator/master/chart/crds/
  ```

  This release changes no CRD field, so there is nothing to apply.

### Upgrade

**Nothing to do**, unless you set `crds.enabled` or `crds.keep` yourself — remove
them, or the render fails naming the replacement.

Your existing CRDs are already in the cluster and Helm adopts them where the
annotations match. An install onto a cluster with no agentops CRDs now works
with no flags and no pre-step, which is what this release is for.

## [7.0.0] — 2026-08-23

**Every image moves registry.** All twelve first-party images the chart renders
repoint from Docker Hub `kmatsebora/*` to
**`ghcr.io/kostiantyn-matsebora/agentops-*`**. No CRD field changes.

Nine of them also move a patch, because nine components now build from one
shared Dockerfile and their entrypoint became `/app`:

| Component | Tag | | Component | Tag |
|---|---|---|---|---|
| `manager` | 0.53.0 | | `channel-telegram` | 0.24.1 |
| `console` | 0.37.0 | | `gateway-telegram` | 0.5.1 |
| `runtime-claude` | 0.8.0 | | `signal-telegram` | 0.6.1 |
| `context-sync` | 0.2.1 | | `signal-alertmanager` | 0.7.1 |
| `egress-proxy` | 0.2.2 | | `signal-k8s-events` | 0.4.1 |
| `housekeeping` | 0.2.1 | | `signal-ha` | 0.2.1 |

**Docker Hub keeps the old tags**, unchanged and still pullable. A tag never
means two different things: where content changed, the number moved.

The chart itself is now published as an **OCI artifact**, so installing no
longer needs a checkout:

```sh
helm install agent-ops \
  oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator \
  --version 7.0.0 --namespace agent-ops --create-namespace
```

**No credential anywhere.** The GHCR packages are public, so pulls stay
anonymous. The chart gains no `imagePullSecrets` value, and no ServiceAccount
changes — which matters because adapter ServiceAccounts are created by the
manager's reconciler, not by the chart, so a chart-rendered pull secret could
never have reached six of the thirteen images.

### Added

- **CI on every pull request** (`.github/workflows/ci.yml`): the twelve Go
  modules build, vet and test — the operator's against a real API server — the
  console UI builds and runs its own suite, the chart lints and renders across
  its value permutations under `kubeconform`, and all thirteen images build.
- **Tag-driven publishing.** A tag of the form `<component>-v<semver>`
  publishes exactly that component. `chart-v<semver>` publishes the chart.
- **Every image is multi-arch** — `linux/amd64` and `linux/arm64` as one
  manifest list, `runtime-claude` included. The release asserts what it pushed
  against a declaration, as equality, so an image that quietly loses an
  architecture fails the release rather than a reschedule weeks later.
- **A LICENSE.** Apache 2.0. Public packages publish the built binaries, so it
  stopped being a footnote.

### Changed

- **BREAKING for pinned images.** Every default image repository gains the
  `ghcr.io/kostiantyn-matsebora/` prefix — six in `chart/values.yaml` (manager,
  console, housekeeping, `runtime.image`, `contextSync`, `egressMediation`) and
  six across the bundles.
- **A published tag can no longer be overwritten.** The release refuses a tag
  already in the registry, so the recovery from a partial failure is a new
  patch version. This was a note in `CLAUDE.md` and is now a gate.
- **Nine images now share one Dockerfile**, and their entrypoint is **`/app`**
  rather than `/signal-cron`, `/channel-telegram` and so on. The images affected
  are the five signal adapters, `channel-telegram`, `gateway-telegram`,
  `context-sync` and `housekeeping`. Nothing the chart renders names an
  entrypoint path, so this is invisible unless YOU set `command:` on one of
  those containers yourself.
- **`runtime-claude` is multi-arch.** It was published amd64-only because the
  hand-run build command said so, not because its upstream is — building it for
  `linux/arm64` and running `claude --version` on `aarch64` settled that. The
  runtime `nodeSelector` the chart still ships is therefore compensating for
  nothing, and can be relaxed at your discretion.

### Upgrade

**Existing Docker Hub tags stay published and are untouched.** An install that
does nothing keeps running.

Upgrading pulls from GHCR. If your cluster cannot reach `ghcr.io`, or you mirror
Docker Hub, stay on the old registry by naming it:

```sh
helm upgrade agent-ops … \
  --set image.repository=kmatsebora/agentops-manager \
  --set runtime.image=kmatsebora/agentops-runtime-claude:0.8.0
```

The same override applies to `console.image.repository`,
`housekeeping.image.repository`, `runtime.contextSync.image`,
`runtime.egressMediation.image`, and each bundle's
`<bundle>.<component>.image.repository`.

**If you set any image repository yourself, that value wins and nothing
changes** — repoint it when you are ready.

**If you override `command:` on an adapter, the sidecar or the housekeeping
job**, change the path to `/app`. Nothing the chart ships does.

## [6.0.0] — 2026-08-23

**Every image moves.** The repository was regrouped by component type and every
component rebuilt and republished, so the version line is uniform rather than
partial:

| Component | Tag | | Component | Tag |
|---|---|---|---|---|
| `manager` | 0.44.0 | | `signal-cron` | 0.3.0 |
| `console` | 0.26.1 | | `signal-alertmanager` | 0.7.0 |
| `runtime-claude` | 0.8.0 | | `signal-k8s-events` | 0.4.0 |
| `context-sync` | 0.2.0 | | `signal-ha` | 0.2.0 |
| `egress-proxy` | 0.2.1 | | `signal-telegram` | 0.6.0 |
| `housekeeping` | 0.2.0 | | `channel-telegram` | 0.20.0 |
| | | | `gateway-telegram` | 0.5.0 |

No CRD field is removed and **the adapter contract version stays `2`**.

**One thing to do on upgrade**, and only for Telegram: `gateway-telegram` is
`telegram-router` renamed, workload included — see the entry below. Everything
else is a tag bump.

### Added

- **`Pipeline.spec.runtimeRef` and `Pipeline.spec.serviceAccountName`.** The
  Pipeline now selects WHAT EXECUTES its conversations and UNDER WHOSE IDENTITY,
  beside the tools and MCP servers it already granted.

  - **Capabilities and execution identity are the same decision** — one says
    which tools may be called, the other with whose credentials. Split across
    two objects, no single object stated an agent's power.
  - **Both are OPTIONAL and an install setting neither is unchanged.**
    `runtimeRef` falls back to the `AgentRuntime` named `default`, and the
    identity to that runtime's own `serviceAccountName`.
  - **One runtime image can now serve two trust levels.** An observing route and
    an acting route differ in their account, not in their image, so the second
    no longer needs a cloned `AgentRuntime`.
  - **The Conversation SNAPSHOTS both at creation.** Editing a Pipeline never
    moves a conversation already running onto a different identity — that would
    be a privilege change applied to work in progress. The runtime name is
    frozen resolved. The account is frozen only where the Pipeline named one, so
    correcting a runtime's own account still heals existing conversations.
  - **Naming an account does not create one.** No reconciler makes a
    ServiceAccount and `Ready` does not check for one — the pod fails at
    admission naming it. A dangling `runtimeRef` does report `Ready=False`.

- **`rbac.runtime.serviceAccounts`** — additional runtime identities the parent
  renders, each with its own `rbacMode` and targeted grants.

- **EVERY BUNDLE RENDERS THE ACCOUNTS ITS OWN ROUTES NEED**, scoped to what
  those routes do, and names each on its own Pipeline. The bundle is the only
  scope that knows: `k8s-bundle` renders one per route, `ha-bundle` renders two
  with no Kubernetes RBAC at all because neither route touches that API, and
  `telegram-bundle` renders none because it ships no Pipeline.

  **"No subchart renders a runtime ServiceAccount" is REVERSED.** That rule was
  protecting against a bundle rendering the SUBSTRATE — a runtime, a credential,
  an account sized for everything. An account sized DOWN to one route is the
  opposite. The substrate stays the parent's, exclusively.

- **Structured agent output.** An agent's answer now reaches chat as named
  blocks with a fold, so a reader gets the conclusion first and the long tail
  only on request.

  - **Two reserved block tags** — `<title>` and `<details>`, THE FOLD. Every
    other tag is a section the agent names for its own job, rendered above the
    fold in the order written.
  - **Each channel adapter parses the grammar** and renders it to what its
    transport has. The manager passes an agent's text through untouched, exactly
    as it already passes markdown through.
  - **Nothing shortens an agent's output.** What sits above the fold is what the
    agent put there.
  - **`AgentProfile.spec.outputFormat`** — REQUIRED, `blocks` or `none`, with no
    default. It gates the prompt only: output is parsed either way.
  - **Telegram** folds `<details>` into an expandable blockquote and splits at
    block boundaries, so the first message always carries the conclusion.
  - **The console** renders the fold as a real disclosure control.
  - **A `signal` is never parsed.** Its structured fields are the message and the
    adapter renders a card from them — the source stated plainly, labels as a
    table, the payload behind its own control.
  - **History renders like live output.** The tags are in `status.runs[].result`,
    so a conversation reopened after a restart parses the same characters a live
    message carried.

- **`GET /channel/vocabulary`** — what may be typed on a chat surface: the
  manager commands and every Ready Pipeline, each with a `position` (`general`
  or `thread`). A channel adapter holds no Kubernetes access, so this is the
  only way it can know what is addressable.
- **`X-Agentops-Vocabulary-Revision`** on every `GET /channel/ops` response, the
  `200` and the `204` alike. A changed revision means refetch. The manager
  cannot dial an adapter, so the news rides a connection the adapter already
  holds.
- **`choices[]` and `inReplyTo` on an outbound message.** Offered actions, and
  the transport's own handle for the message being answered. Both optional, both
  structured. A transport without controls renders the same list as text.
- **Telegram registers a command menu** per served chat, covering the manager
  commands and your Ready Pipelines. Telegram then shows its own control in the
  composer and completes what you type.
- **Tapping a Pipeline on the ambiguity refusal sends the message you already
  typed**, rather than making you write it again.
- **The console's reply box completes commands** — `/exit` and `/close`, with
  the difference stated. It never offers a Pipeline there.
- `agentops.dev/message`, an optional label on a chat signal carrying the
  transport's handle for the arriving message.
- **Seven adopter guides**, in learning order rather than by risk. Each one
  states what its own mistake costs, because risk is not monotonic along
  that order — a capability binding is pure YAML and can grant more than an
  adapter's code ever could.

  | Guide | Teaches |
  |---|---|
  | [Put an agent to work](guides/pipeline.md) | the wiring, built from what a demo install already has. It creates nothing but the `Pipeline` |
  | [Add your own agent](guides/agent-profile.md) | an `AgentProfile` of your own |
  | [Run your agent from a repository](guides/agent-from-a-repository.md) | the checkout, its deploy key and the agent definition |
  | [Give your agent tools](guides/toolsets.md) | `MCPToolset` and `MCPConfig`, bound from the Pipeline |
  | [React to signals](guides/signal-adapter.md) | implementing a `SignalAdapter` |
  | [Talk to agents from your own chat](guides/channel-adapter.md) | implementing a `ChannelAdapter` |
  | [Run agents on your own backend](guides/agent-runtime.md) | implementing the work contract as an `AgentRuntime` |

  - **The Pipeline guide is FIRST and creates nothing new.** A guide that opens
    by declaring an identity teaches an inert object whose purpose is a Pipeline
    the reader has not met.
  - **Every template and worked example is GENERATED**, never written — the
    templates from the CRDs the chart ships, the examples from each bundle's own
    values. An invented example is a second set of values to keep true.

- **[`docs/cr-reference.md`](cr-reference.md)** — every field of every kind,
  generated from the CRDs. A reference file beside `concepts.md`, not a site
  page, so the guides can carry the minimal resource and link the rest.
- **A drift check in CI.** `python3 .github/scripts/docs-generate.py`
  regenerates the templates, the examples and the reference; CI reruns it with
  `--check` and FAILS on any difference, naming the file and the command.
  Committed generated output without that check is correct the day it is
  written and silently wrong after the next field rename.

### Changed

- **BREAKING: A Pipeline that names no `serviceAccountName` now has NO CLUSTER
  POWER.** It runs as a floor account the chart renders and binds to nothing.

  A route used to inherit whatever the release granted, so it held cluster
  power by not typing a field. Three of four routes in the reference install
  held pod-delete and node-patch that way, and two of them reached no Kubernetes
  API at all.

  **Acting power is now something a route OPTS INTO by name.** `rbacMode`
  renders `agentops-runtime-acting` (or `-readonly`) and a Pipeline names it.

- **The grant is CLUSTER-WIDE, and `clusterroles` is no longer readable.**

  Namespaced Roles were built and reverted before release. RBAC cannot express
  "everywhere except", so bounding an agent meant an allow-list: one binding per
  namespace per account — 224 objects on a 28-namespace cluster — and every new
  namespace invisible to the agent until someone edited values and redeployed.

  **What makes cluster-wide safe is OMISSION, not scope.** `agentops.dev` is
  never granted in any rule, so Conversations and Pipelines are unreadable
  everywhere. Neither is `secrets`. `clusterroles` is dropped too — that listing
  maps every identity in the install and which one is worth attacking.

  **What it costs:** under `full` an agent can restart or delete pods in the
  operator's own namespace, the manager and adapters included.

- **BREAKING: `global.agentops.runtime.allowPodExecution` is OFF, and it is what
  makes "agents cannot read your Secrets" TRUE.**

  No role carries a verb on `secrets`. That was never enough on its own: the
  KUBELET resolves a Secret when it builds a pod, so an agent that can create a
  pod mounting one — or exec into a pod that already has one — reads the value
  having never asked the API server. `secrets: get` is never evaluated.

  Verified on a live cluster against the shipped role: pod created, pod log
  read, secret value returned, all seven `secrets` verbs denied throughout.

  **It gates every write that produces or enters a pod**, not just
  `pods: create` — creating a Job or Deployment writes a pod spec, and patching
  one edits it. With it off an agent still scales, restarts, evicts, cordons,
  deletes workloads and edits ConfigMaps, Services and Ingresses. What it loses
  is the ability to run new code.

- **BREAKING: `rbacMode: full` no longer binds `cluster-admin`, and `readonly`
  no longer binds the built-in `view`.** Every grant is now a role this chart
  writes out, so an operator can read it without resolving an aggregated or
  built-in role. **No runtime role carries any verb on `secrets`, in any mode.**

  An agent has a shell. `--allowedTools` configures a COOPERATING agent, while a
  ServiceAccount binding is what an uncooperative one actually has — so
  `cluster-admin` meant every credential in the cluster was readable by a model,
  and no allowlist changed that. The manager itself holds no `secrets` verbs.

  **BOTH WALLS MOVED.** `k8s-bundle`'s MCP server bound `cluster-admin` under
  `full` and the built-in `view` under `readonly` — and an agent reaches the
  cluster THROUGH it, so fixing one wall and not the other would have left the
  hole one indirection along. It now carries the same split from the same
  values, and cannot read this release's own namespace either.

  **The role grants** every read `readonly` grants, plus the workload verbs an
  agent fixes things with: delete or patch a pod, create a `pods/eviction`,
  update or patch a Deployment / StatefulSet / DaemonSet / ReplicaSet, scale
  through `*/scale`, create, delete or patch a Job, patch a CronJob, patch a
  Node. RBAC objects are readable and never writable, and there is no `escalate`
  or `bind`.

  **It will be wrong at first, and it fails CLOSED** — an agent is refused an
  action and says so, rather than quietly holding power nobody reviewed.

- **BREAKING: `AgentProfile.spec.runtimeRef` is DEPRECATED**, moved to
  `Pipeline.spec.runtimeRef`, and read for ONE release only.

  An `AgentRuntime` carries the ServiceAccount an agent runs as, so a profile
  choosing one chose the agent's power in the cluster — which made profile-edit
  rights into service-account-choice rights. A profile is prompts, a repo ref
  and limits, while a Pipeline already grants tools and MCP servers.

  **Nothing changes for an install that does not act.** The profile ref is read
  below the Pipeline's own field, so a profile applied before the upgrade keeps
  dispatching where it named, and setting both is harmless with the Pipeline
  winning.

- **BREAKING: `AgentProfile.spec.outputFormat` is REQUIRED.** Every profile must
  declare `blocks` or `none`. There is no default, because both candidates are
  wrong — `none` leaves output unformatted unless the prompt says otherwise, and
  `blocks` shapes it by something the author never asked for.

  **`kubectl apply` on a profile omitting it FAILS**, naming the valid values.
  Chart-managed profiles are re-applied with the field by the same upgrade.
  Hand-written ones need a one-line edit.

- **BREAKING (prompt only): the five report templates in the shared format
  specification are REMOVED.** `format.md` is rewritten in the block grammar,
  and its numbered templates become a default set of SECTIONS — a starting
  point, not a mandatory form.

  A profile that relied on the built-in investigation or action templates keeps
  a shared shape only by declaring `outputFormat: blocks`, which injects the
  new default sections instead.

  **Nothing breaks silently.** An agent that formats however it likes produces
  one block, which renders exactly as agent output does today.

- **The outbound contract version does NOT move.** The block grammar adds no
  field and changes no meaning: a body that was markdown is now markdown plus a
  grammar, read by the component that already read the markdown.

  **An adapter with no parser for it renders the tags literally.** That is
  prevented by a profile declaring `outputFormat: none` — the compatibility
  boundary is the profile, not the wire.

- **`/agents` is now `/pipelines`.** The old name still works and always will,
  but it is never printed, offered or registered. It listed Pipelines, and
  "agent" already names a definition inside a profile's repository.
- A Pipeline named `pipelines` joins the set unreachable by command.
- A hyphenated Pipeline is completed on Telegram under an underscored spelling
  (`/k8s_observe`). **The CR is not renamed** and the manager never sees the
  other form — the adapter translates it back. Both forms work when typed.
- **The console applies stream events instead of reloading.** Its `delta` event
  now carries the changed object, projected into the shapes its own snapshot
  endpoints serve, so a listing, a kind detail and an open conversation update
  in place. A message appears from the message event itself.
- **Sending a message from the console asks for nothing WHILE THE STREAM IS
  UP.** The echo, the acknowledgement and the answer all arrive as events. The
  composer used to re-read the whole conversation on every send — the heaviest
  read on the page, for what was already on its way. With the stream DOWN it
  still reads once, because the confirmation that clears `sending…` is itself a
  stream event, and without it a sent message sits unconfirmed until the page is
  reloaded.
- **A console view that has painted never goes back to a spinner.** A change
  counter used to sit in every query key, so each event asked for a cache entry
  that had never been filled — which is what made the page blank for a second
  every time anything moved. Sending a message did it three times.
- **Refetching in the console keeps four reasons**: first load, a resync, an
  explicit action, and a value that decays with time. Overview and Topology keep
  their timers, because rates and ages are wrong when time passes rather than
  when something changes. Aggregates the browser cannot recompute — install
  counts, the traffic graph, cross-object findings, resolved capabilities — are
  re-read on a stable key, so the page stays on screen while the read lands.
- **The browser cache is bounded.** Data for a view that is off screen is
  released after five minutes, and returning to a view after a minute loads
  fresh. Nothing is persisted — no `localStorage`, no IndexedDB — so closing the
  tab still leaves nothing behind.

### Fixed

- **A tool advertised with a parameter that has NO TYPE is repaired before the
  agent sees it.** Home Assistant publishes `GetLiveContext`'s `domain` filter as
  `anyOf: [{}, {type: array…}]` — the first branch is an empty schema, so the
  parameter constrains nothing and a model writes `{"domain": sensor}`, which is
  not valid JSON and never runs. The egress-proxy already rewrites tool listings
  on the way back, and now drops union branches that say nothing, leaving the
  typed branch the server itself published. **It invents no types**: a union of
  typed branches passes through untouched, and a property whose only branch is
  empty is left exactly as published. Requires `runtime.egressMediation`.
- **A tool call the model could not FORM no longer spins.** Arguments that are
  not valid JSON are discarded by claude-code before anything runs — no MCP
  server sees them, no allowlist refuses them — so a run made of them looks busy
  and then answers from whatever the session already held. `runtime-claude` now
  counts them, and ends the run as **failed** when the same tool is called with
  the same unparsable arguments five times in a row, naming the tool and
  quoting what was written. `RUNTIME_UNPARSED_REPEAT_LIMIT` tunes it, `0`
  disables the breaker and keeps the counting.
- **A run that recovers from one says so on the answer.** Recovering usually
  means abandoning the tool rather than fixing the call, and the model then
  answers from what the session already held without mentioning it. The runtime
  appends one line naming how many calls never ran and which tool — the agent's
  answer is still the answer.
- **Both Home Assistant profiles are told to quote `domain`.** Home Assistant
  advertises `GetLiveContext`'s domain filter with an `anyOf` whose first branch
  is an empty schema, so the parameter has no declared type and a model writes
  `{"domain": sensor}`. Measured on one install, 59 of 110 calls to that tool
  never executed. The prompt line is a workaround for a schema the chart does
  not own.

### Changed

- **The repository is grouped by component type** — `platform/` `runtimes/`
  `signals/` `channels/` `gateways/`, one container per directory, with the
  operator now at `platform/manager/`. A component's published name is derived
  from its PATH: a plural group lends its singular as a prefix
  (`signals/cron` → `agentops-signal-cron`), a singular one lends nothing
  (`platform/console` → `agentops-console`). Twelve of thirteen image names are
  unchanged.

  Every Go module path now follows its directory, so `api/v1alpha1` is imported
  as `…/agent-ops-operator/platform/manager/api/v1alpha1`. No CRD, contract or
  runtime behaviour changed.

- **BREAKING — `telegram-router` is now `gateway-telegram`.** The image is
  `agentops-gateway-telegram` and the Deployment is `agentops-gateway-telegram`.
  The old image stays published, as `signal-vmalertmanager` did.

  ### Upgrade

  Helm creates the new Deployment before deleting the old one, so for a few
  seconds **two consumers poll one bot token** — 409s and a couple of stolen
  updates, with the same image on both sides. To avoid the overlap entirely,
  scale `agentops-telegram-router` to zero before upgrading, or uninstall the
  telegram bundle and reinstall it.

  Nothing else moves: the `router:` values key in `telegram-bundle` keeps its
  name, and both adapters, the Channel and the SignalSource are untouched.

### Removed

- **The `/<pipeline>:<agent>` addressed form.** A Pipeline names one profile and
  a profile names one agent, so the agent is decided by the wiring. Letting the
  sender pick a different one reached past it.

  Text after the Pipeline name is now simply the task, colons included.

### Deprecated

- `Conversation.spec.inputs[].agent`. Nothing writes it. Dispatch reads it for
  one release so an input queued before the upgrade still reaches the agent it
  was parsed with. The field is removed in a later release.
- `AgentProfile.spec.runtimeRef`. Moved to `Pipeline.spec.runtimeRef` and read
  for ONE release, below the Pipeline's own field. Removed in the next major.

### Upgrade

**Every route loses its cluster reach until you say otherwise.** That is the
point of the change, and it fails CLOSED — an agent is refused and says so.

**1. Name an account on every route that needs cluster power.** A Pipeline that
names none now runs as an account bound to nothing.

```yaml
global:
  agentops:
    runtime:
      rbacMode: full                # renders agentops-runtime-acting

pipelines:
  - name: <the route that acts>
    profile: <its profile>
    serviceAccountName: agentops-runtime-acting
  - name: <a route that only observes>
    profile: <its profile>
    # name nothing: no cluster power at all
```

**2. Decide `allowPodExecution`, and read why before you set it.**

It is off, and it is what makes "agents cannot read your Secrets" true rather
than merely written down. With it off your agents cannot create a pod, edit a
workload's pod template, or exec into a container.

**Turn it on only if you accept an agent reading every Secret in the cluster** —
the kubelet resolves a Secret when it builds a pod, so pod execution and Secret
access are the same capability.

```yaml
global:
  agentops:
    runtime:
      allowPodExecution: true    # grants pods_run and pods_exec, and Secret reach
```

**3. If you ran `rbacMode: full`, check what the enumerated roles omit.**

```sh
helm template <release> agentops/agent-ops-operator -f your-values.yaml \
  | awk '/^kind: (Cluster)?Role$/,/^---/' | grep -A400 'agentops-runtime-acting'
```

```powershell
helm template <release> agentops/agent-ops-operator -f your-values.yaml `
  | Select-String -Pattern 'agentops-runtime-acting' -Context 0,400
```

Need something they omit? **Add your own `ClusterRole`** rather than widening
the shipped one, and attach it to the route that needs it:

```yaml
rbac:
  runtime:
    serviceAccounts:
      - name: agentops-runtime-special
        rbacMode: full
        clusterRoles:
          - name: extra
            rules:
              - apiGroups: ["example.com"]
                resources: ["widgets"]
                verbs: ["get", "list", "patch"]

pipelines:
  - name: <the route that needs it>
    serviceAccountName: agentops-runtime-special
```

**4. If any `AgentProfile` sets `runtimeRef`, move it to every Pipeline that
routes to that profile.**

```sh
kubectl -n agent-ops get agentprofiles \
  -o jsonpath='{range .items[?(@.spec.runtimeRef)]}{.metadata.name}{"\t"}{.spec.runtimeRef.name}{"\n"}{end}'
```

```powershell
kubectl -n agent-ops get agentprofiles `
  -o jsonpath='{range .items[?(@.spec.runtimeRef)]}{.metadata.name}{\"`t\"}{.spec.runtimeRef.name}{\"`n\"}{end}'
```

Empty output means there is nothing to do. Otherwise set `runtimeRef` on each
routing Pipeline — the profile field keeps working for this release, and the
Pipeline wins wherever both are set, so routes can move one at a time.

## [5.25.0] — 2026-08-22

Images: manager `0.38.1`, console `0.16.0`.

### Added

- `Conversation.status.runs[].inputs[]` — what each run was asked. Text, arrival
  time, origin surface and sender, beside what it answered. Text is inlined to
  2000 characters and marked `truncated` beyond that.
- `ChannelAdapter.spec.echoesOwnMessages`, default `true`. Declares whether the
  transport shows a person the message they just typed. A **viewer** — one that
  renders only what it is sent — sets it `false`. The console does, in the chart.

### Changed

- **A person's message now reaches every bound channel except the surface it was
  typed on.** It used to be withheld from all of them, so a second surface never
  saw what somebody asked.
- The console shows the message that **started** a conversation, and keeps it
  across a reload or a restart. It used to begin at the agent's answer.
- `spec.inputs[]` is still a queue and is still pruned once processed. Pruning is
  no longer the only copy of what a person said.

Nothing is posted back to the surface that displayed it. Nothing is delivered
retroactively.

### Upgrade

1. Apply the CRDs **before** the manager image. Both new fields are optional, but
   a manager writing a field the CRD does not know loses it silently.
2. `helm upgrade` with the new image tags.
3. Nothing is backfilled. Runs recorded before this carry no inputs.

**If you wrote your own channel adapter**, check one rule: it must never
re-ingest its own outbound posts as inbound. One adapter may now serve several
surfaces of one conversation, so a message can be delivered toward the transport
it entered through.

Rolling back is reverting the images. Old records stay readable.

## [5.24.0] — 2026-08-22

### Changed

**BREAKING for pinned images.** `signal-vmalertmanager/` is now
**`signals/alertmanager/`**, and its image is
**`kmatsebora/agentops-signal-alertmanager`** at the same tag (`0.6.0`,
identical behaviour).

The adapter reads the standard Alertmanager webhook payload, which vanilla
Alertmanager and VictoriaMetrics both send. The vendor name described one
sender, not the component.

Unchanged, so no immutable-field upgrade failure:

| Thing | Value |
|---|---|
| `SignalAdapter` CR name | `alertmanager` |
| `SignalSource.spec.adapter` | unchanged |
| Deployment selector label | `agentops.dev/signal-adapter` |

These deliberately keep VictoriaMetrics names, because each names a
VictoriaMetrics API object rather than one of ours:

- `register.go` writes a **`VMAlertmanagerConfig`**. Vanilla Alertmanager's
  config is a file with no object to write, so NOTES.txt prints a receiver stanza
  for it instead.
- `metrics.vmServiceScrape` renders a **`VMServiceScrape`**. The rules component
  renders a **`VMRule`**.

### Security

`global.agentops.networkPolicy.enabled` now covers the prometheus-bundle metrics
MCP server, the third and last unprotected one. It authenticates nobody, so any
pod in the cluster could query the whole metrics backend through it. It is now
restricted to runtime pods.

The webhook adapter is restricted only once you name the sender:

```yaml
prometheus-bundle:
  alertmanager:
    webhookFrom:
      - namespaceSelector:
          matchLabels:
            kubernetes.io/metadata.name: monitoring
```

**Empty leaves it reachable on purpose.** A policy that selects the adapter and
names nobody denies the alert lane silently, and that is discovered during an
incident. Under-restricting is the recoverable mistake.

The metrics MCP server moves to an exec probe over loopback, for the reason the
Kubernetes one did. The kubelet probes from the node, no policy peer can name a
node, and this server is reached on the port it serves on.

### Upgrade

Only if you set `prometheus-bundle.alertmanager.image.repository` yourself. Point
it at `kmatsebora/agentops-signal-alertmanager`. The old image stays published
for installs pinned to an older chart.

## [5.23.0] — 2026-08-22

Two things were true and neither was written down. Both are now closable, and
**both are off by default**. Upgrading changes nothing until you ask.

Decided in [adr/0001-bound-component-reach.md](adr/0001-bound-component-reach.md).

### Added

- `global.agentops.networkPolicy.enabled` renders one NetworkPolicy per
  component, allowing only the callers your wiring implies.
- `runtime.egressMediation.enabled` puts a proxy in the runtime pod that the
  agent's traffic cannot route around, and enforces the bound toolsets there.
- `Conversation.status` gains an `EgressMediated` condition.

### Security

**Nothing restricted who may reach this release's components.**

- The MCP servers accept any caller with no credential. Under `rbacMode: full`
  the Kubernetes one runs as cluster-admin, so reaching it *is* cluster-admin.
- The manager's work contract took no credential either. Any pod could take a
  queued work unit or post a forged agent answer.

**A route's toolsets bound only a cooperating agent.** `--allowedTools` is
applied by the CLI beside the agent.

An MCP server has never heard of an `MCPToolset`. An agent with a shell reached a
bound server directly and called anything it registered. `agentops-shell` is
bound on ordinary routes.

Egress mediation costs two things, so read them before enabling it:

- A **privileged init container**, refused by a namespace under `restricted` Pod
  Security admission.
- A container per active conversation.

stdio servers and `https` MCP endpoints stay unenforceable. They are reported on
`EgressMediated` rather than passed off as covered.

### Upgrade

Nothing, unless you enable one of the two flags. If you enable network policy,
name these four or a workload breaks quietly:

| Value | Names |
|---|---|
| `networkPolicy.metricsFrom` | a collector outside the namespace |
| `networkPolicy.consoleFrom` | your ingress controller |
| `networkPolicy.probesFrom` | your node network, if your CNI does not exempt host traffic |
| `prometheus-bundle.alertmanager.webhookFrom` | your Alertmanager sender |

**Read the note it prints.** A NetworkPolicy on a cluster whose CNI does not
enforce policy applies cleanly, appears in `kubectl get`, and blocks nothing. The
chart cannot detect that, so NOTES.txt tells you how to check.

The manager's probe port serves only health, so it is opened unconditionally. The
Kubernetes MCP server now probes itself over loopback, which no CNI can block.

## [console 0.15.9] — 2026-08-22

### Fixed

A conversation started from the composer showed a transcript beginning at the
**agent's answer**, with the question that caused it missing. Typing into an
already-open conversation was fine. Only the opening message vanished.

**Cause.** The manager posts an input to bound threads only when the person has
not already seen it. An alert gets a signal card. A message somebody typed gets
nothing, because posting it back would echo on the surface it was typed on.

That rule is right for a transport and wrong for a **viewer**. A Telegram user's
own message is already in their thread, put there by Telegram.

A console user's is not — the console renders what it was sent. The input is then
pruned once processed, so nothing could recover it.

**Fix, console-side only.** The console watches conversations and records what
people typed into its own transcript buffer, keyed on the input id. The set is
read off the manager's own rule rather than guessed.

An input with **no** origin is skipped. It cannot be told from an alert, and
inventing the wrong bubble is worse than a missing one.

Three things the first cut got wrong:

- **It read as typed.** An addressed task (`/ha-control turn the AC on`) reaches
  the conversation as the rest, because the manager consumed the address. The
  console posted the whole thing, being the only component that still has it.
- **It carries the starter's identity.** The input records provenance, not
  authorship. Without this the opening message read `local` while the reply below
  it read your address.
- **A reply is not duplicated.** The input a typed message becomes is the durable
  identity of that bubble, not a second one. It is adopted, keeping its id.

The UI also stopped printing `local` as a speaker's name. `local` means "typed on
this console", which is a fact about where a message entered, not a person.

### Upgrade

Bump the console image tag. The chart default moves with it. Nothing else
changes.

Alerts keep their manager-posted card. A console restart still loses what was
never CR state.

## [5.22.0] — 2026-08-21

Adds the Home Assistant bundle. See [the Home Assistant integration](integrations/home-assistant.md).

### Added

`chart/charts/ha-bundle/` ships a **privilege split** rather than one agent:

| Agent | Profile | Reached by | Job |
|---|---|---|---|
| The house's user | `ha-user` | an ordinary chat message | **use** the house — services, lights, automations |
| The administrator | `ha-operator` | `/ha-ops <task>`, by name | **fix** it — integrations, configuration, repairs |

The split is **use versus fix**, not read versus act. Home Assistant has no
read-only role, so neither credential merely looks.

What separates the lanes is the REST path. Assist intents reach no
configuration, so repairing needs a shell, and only the ops route binds one.

The acting route claims the log source and **no chat source**, so escalating is a
deliberate act. Claiming and addressing are independent mechanisms, so listing a
chat source there would grant it nothing while making every unaddressed message
on that surface ambiguous.

The **operator credential gates the fixing half**, and the ingest lane needs it
too. Home Assistant's `subscribe_events` is admin-only, so a control token
connects, passes auth, and is then refused the subscription.

There is **no MCP server workload**. Home Assistant serves its own endpoint
through the built-in MCP Server integration.

**A second MCP server for the ops lane**, off by default (`adminMcp.enabled`).
The built-in server exposes Assist intents only. It cannot read a log, reload an
integration or disable an entity, which live in registries served over the
WebSocket API.

It is bound to `ha-ops` alone, so `ha-control` reaches a server with no such
tools. Two walls, not one allowlist.

Two ways to have one, both off by default:

1. Let the chart deploy [ha-mcp](https://github.com/homeassistant-ai/ha-mcp)
   in-cluster (`adminMcpServer.enabled`).
2. Point `adminMcp.url` at a server you run, including a HACS integration inside
   Home Assistant.

Enabling the config with neither fails the render. **Add-ons are not an option on
Home Assistant Core**, which is the usual shape in Kubernetes.

Of the 78 tools that server registers, **52 ship**. The 26 withheld restart Home
Assistant, manage backups, delete registry objects or install software.

The toolset is enumerated and the image tag pinned. A server that renames a tool
changes what the allowlist means with nothing failing.

**The ops role names the REST path explicitly.** Without that the agent reads its
own tool list, finds device controls and no way to reach a log, and reports the
task impossible.

**New module `signals/ha/`** — a dependency-free signal adapter reading the
instance's WebSocket API over a hand-written RFC 6455 client.

- Watches `system_log_event`.
- Same `rules` / `route` vocabulary as the cluster Events adapter, minus the
  time axis.
- `kubernetesAccess: false`, because its data source is the house.
- Image `kmatsebora/agentops-signal-ha:0.1.0`.

### Changed

A subchart may render a `Pipeline` when — and only when — **all** of these hold:

1. Rendering is behind an explicit wiring flag.
2. Every reference to an object the bundle does not render is a values-supplied
   NAME, omitted when unset.
3. Each `Pipeline` renders only with its own profile.
4. The flag **defaults off**, forced on by nothing but a values path whose
   declared purpose is a turnkey install.

This does **not** make bundle-shipped wiring the norm. A bundle whose sources and
channels come from elsewhere still cannot meet condition 2. `telegram-bundle`
continues to ship none, and is the counter-example: a chat surface is answered by
an agent from somewhere else.

`ha-bundle` is the third bundle to qualify, after `k8s-bundle` and
`prometheus-bundle`.

### Upgrade

Nothing. `ha-bundle.enabled` defaults `false` and demo mode never turns it on.

To enable it, create the Secrets first. They are referenced by name and never
created by the chart. Each carries one token under **two** keys, because the
adapter sends the raw token and the MCP path sends a complete header value:

```sh
kubectl -n <ns> create secret generic ha-admin \
  --from-literal=token="$TOKEN" --from-literal=authorization="Bearer $TOKEN"
```

Then set the endpoint, the credentials and — deliberately — the routes.

## Older versions

| Archive | Covers |
|---|---|
| [CHANGELOG-5.0-5.21.md](changelog/CHANGELOG-5.0-5.21.md) | chart 5.0.0 through 5.21.0 |
| [CHANGELOG-1.0-4.0.md](changelog/CHANGELOG-1.0-4.0.md) | chart 1.0 through 4.0.0 |
