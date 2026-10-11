# Changelog archive — chart 11.0.0 through 12.0.0

Migration guides for chart versions **11.0.0** and **12.0.0**, in
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) format.

Moved here from [CHANGELOG.md](../CHANGELOG.md), which holds the ten most recent
versions.

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
