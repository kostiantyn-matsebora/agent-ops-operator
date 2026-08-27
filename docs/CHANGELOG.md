---
title: Changelog
permalink: /changelog/
description: >-
  Every chart version, newest first — what changed, and the upgrade steps a
  breaking one needs.
---

All notable changes to this project are documented here, in
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) format. Versions are
CHART versions. Image tags move independently and are named in each entry.

This file holds the **ten most recent versions**. Older entries are in
`changelog/`, linked at the [foot of this page](#older-versions).

See [the repository](https://github.com/kostiantyn-matsebora/agent-ops-operator)
for the source and the reference material beside this file.

## [13.2.0] — 2026-08-27

**A burst that healed before its dwell closed no longer opens a
conversation.** Both dwell adapters re-check a matched event at the end of its
`for` window. For a kind with no health predicate — every kind but Pod in the
cluster-events lane, every integration without a config-entry state in the
Home Assistant lane — the re-check asked *did it recur*, and a controller that
retried every few seconds for half a minute and then healed answered yes. On
the reference install that was most of the backlog: twelve conversations
about Longhorn snapshot purges that had healed two minutes before the
catch-all fired, each concluding "self-resolved, no action needed".

### Changed

- `signal-k8s-events` 0.4.4, `signal-ha` 0.2.4: the second verification rung
  asks whether the event was **still recurring as the window closed** — its
  last third, floored at thirty seconds, derived from the window actually
  waited so escalation keeps the proportion. A burst that went quiet before
  that is dropped as churn; one still arriving is reported once, and its
  evidence names how long before the close the last event arrived.
- The Home Assistant re-check applies the same rule to the log's own count: a
  count that rose early in the window and stopped rising is a blip that
  healed, not an integration still failing.

**Trade-off, named.** A controller backing off to a retry period longer than
the closing window — ninety seconds under a three-minute rule — is dropped at
that deadline, and its next retry opens a fresh window. The report is delayed
by one window, not lost. Anyone preferring the previous behaviour restates the
rule with a shorter `for`; there is no switch.

No configuration, CRD or RBAC change. `NodeNotReady` and the other `for: "0"`
reasons are unaffected — they are never re-checked; a scheduled reboot is the
time axis's job (`route.muteTimeIntervals`, see the Kubernetes page).

## [13.1.1] — 2026-08-27

**Every image rebuilt on the current toolchain.** The weekly scan of the
published images reported fixable findings in every Go binary: 22 Go
standard-library CVEs, CVE-2025-68121 among them at CRITICAL.

The manager carried eight more from `golang.org/x/net`, `x/oauth2` and
`x/text`, and `runtime-claude` five inside npm's bundled dependencies. The tree
had already moved past all of them (Go 1.25, the manager's `x/*` bumped). The
published images had not.

### Changed

Twelve components moved to versions built from that tree:
`channel-telegram` 0.24.4, `console` 0.38.2, `context-sync` 0.2.3,
`egress-proxy` 0.2.4, `gateway-telegram` 0.5.3, `housekeeping` 0.2.3,
`manager` 0.57.2, `runtime-claude` 0.8.3, `signal-alertmanager` 0.7.3,
`signal-ha` 0.2.3, `signal-k8s-events` 0.4.3, `signal-telegram` 0.6.3.

**No behaviour changes.** `runtime-ollama` 0.1.0 was already built on Go 1.25
and scanned clean, so it is unchanged.

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
  the render FAILS naming the key when the endpoint is missing. The model is
  optional while the server has exactly one pulled.
- The runtime implements the six built-in tools natively — `Read`, `Grep`,
  `Glob`, `Edit`, `Write`, `Bash` — so `agentops-observe` / `-shell` / `-edit`
  mean the same thing on both runtimes, connects the bound MCP servers from the
  same `mcp.json`, and keeps context as one transcript per conversation under
  `$HOME/.agentops/contexts/`, declared to `context-sync` by the bundle.
- `docs/runtimes/ollama.md`, and the Ollama chip on the landing page.

- **Every runtime has its own name, and `default` is a copy.** The claude
  bundle's CR is now named `claude`, the ollama one `ollama`, and the parent
  renders one more `AgentRuntime` named `default` — a copy of the runtime
  flagged `default: true` (`claude.default`, `ollama.default`,
  `runtimes[].default`), or of the first configured when none is. So a fresh
  install is unchanged (`default` is claude's copy), turning claude off and
  ollama on needs no rename, and `runtimeRef: claude` now works. Two flags fail
  the render. A `Pipeline` naming `runtimeRef: default` keeps resolving.

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

## Older versions

| Archive | Covers |
|---|---|
| [CHANGELOG-8.0.md]({{ '/changelog/CHANGELOG-8.0.md' | relative_url }}) | chart 8.0.0 |
| [CHANGELOG-5.22-7.0.md]({{ '/changelog/CHANGELOG-5.22-7.0.md' | relative_url }}) | chart 5.22.0 through 7.0.0 |
| [CHANGELOG-5.0-5.21.md]({{ '/changelog/CHANGELOG-5.0-5.21.md' | relative_url }}) | chart 5.0.0 through 5.21.0 |
| [CHANGELOG-1.0-4.0.md]({{ '/changelog/CHANGELOG-1.0-4.0.md' | relative_url }}) | chart 1.0 through 4.0.0 |
