All notable changes to this project are documented here, in
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) format. Versions are
CHART versions. Image tags move independently and are named in each entry.

This file holds the **ten most recent versions**. Older entries are in
`changelog/`, linked at the [foot of this page](#older-versions).

See [the repository](https://github.com/kostiantyn-matsebora/agent-ops-operator)
for the source and the reference material beside this file.

## [13.4.0] — 2026-09-06

**Every first-party image is rebuilt and re-pinned.** Beside the two entries
below, each image carries what landed on it since its last tag:

- `manager` 0.57.3 — status patches on ensure-topic use optimistic locking
  and retry on conflict. Send op ids carry a per-process epoch, so a restart
  cannot reuse one. `x/crypto` and `x/net` bumped.
- `channel-telegram` 0.25.0 — the outbound base URL is configuration
  (`TELEGRAM_API_BASE`, or the bot Secret's `apiBase` key), completed ops
  are remembered in a bounded set, logs are sanitised.
- `gateway-telegram` 0.6.0 — `TELEGRAM_API_BASE`, and the poll loop split
  into named steps.
- `signal-telegram` 0.6.4 — logs are sanitised.
- `console` 0.38.3 — the session cookie is `Secure` behind TLS or
  `X-Forwarded-Proto: https`. Logs are sanitised. The image builds with
  `npm ci --ignore-scripts`.
- `egress-proxy` 0.2.5 — refuses a connection whose original destination is
  the proxy itself.
- `runtime-claude` 0.9.0 — `@anthropic-ai/claude-code` pinned to 2.1.252
  (was `@latest`) and installed with `--ignore-scripts` plus its own
  installer. `apt-get upgrade`, so a cached layer cannot keep a patched CVE
  out. The prompt file is confined to the workspace. Logs are sanitised.
- `runtime-copilot` 0.1.1 and `runtime-ollama` 0.1.1 — the same image
  hygiene: pinned npm, `--ignore-scripts`, `apt-get upgrade`, confined
  prompt file, sanitised logs.
- `signal-alertmanager` 0.7.4, `housekeeping` 0.2.4, `context-sync` 0.2.4 —
  rebuilt on the current base image, no behaviour change.
- `signal-ha` 0.4.0 and `signal-k8s-events` 0.4.5 — below.

### Added

- **`signal-k8s-events` gained a fourth suppression axis: node STATE.** While
  a node is cordoned or maintenance-tainted, every event on its objects (and
  on the Node itself) is suppressed for exactly as long as the drain lasts —
  a rolling reboot manager (kured is the canonical case) now opens ZERO
  conversations, whatever it does to the pods scheduled there. Configured
  under a source's `spec.config.route`: `drainingNodes` (`suppress`, the
  default, or `report` to opt a source out), `drainingNodeMatchers` to
  narrow it, `drainingNodeBound` (default `1h`) — past which a forgotten
  cordon is reported once (`kind: Node`, reason `NodeDrainExceeded`) and
  suppression on that node is released. Needs the events `ClusterRole`'s new
  `nodes` `list`/`watch` grant (cluster-wide installs only — see below).
- **`signal-ha` 0.4.0: the Home Assistant lane watches the four health
  surfaces beside the log.** Home Assistant reports most of what is wrong
  with a house somewhere other than its log. An integration that cannot
  reach its device sits in a config entry state and logs one line at
  startup. A problem it has diagnosed is a repair. A device fault is a
  `problem` or `connectivity` binary sensor. A package behind is an
  `update.*` entity. Each is now a record kind on the same source, through
  the same rules, dwell and self-exclusion, labelled `surface=`
  (`log`, `config-entry`, `repair`, `sensor`, `update`) so a rule can select
  it. One signal per failed entry (with its reason), per repair, per faulting
  sensor. ONE digest listing every pending update, re-posted when the set
  grows. Each surface is switchable and tunable under
  `logsAdapter.source.surfaces` — config entries, repairs and sensors on by
  default, the update digest off — and a surface that is off is never read.
  The shipped rules open with one per surface, ahead of the log rules. A
  values file that replaces `rules` replaces those too.

### Changed

- **The `kubernetes` bundle's events `ClusterRole` gains `nodes`
  `list`/`watch`** (cluster-wide RBAC only; a namespaced install has no
  equivalent, since nodes are cluster-scoped — drain awareness is simply off
  there, and every source's Ready condition notes it once).
- **The shipped tier-3 node-condition rule now matches `kind="Node"`.**
  `NodeHasDiskPressure` / `NodeHasMemoryPressure` are unaffected — the
  kubelet already reports those once, against the Node object. `NodeNotReady`
  is NOT: the node lifecycle controller stamps it on every pod scheduled on
  the affected node, so before this change one node blip fired once per
  workload at `for: 0`, with no chance to notice the node recovering. A
  pod-level `NodeNotReady` now falls to the catch-all rule instead, which
  already dwells and re-checks the pod's own health before emitting — so a
  reboot that recovers within that dwell (the ordinary case) is no longer
  reported at all, and one that does not is reported once it recurs.
  **If your `spec.config.rules` overrides the shipped tier 3** (rather than
  inheriting the chart default), add the `kind="Node"` matcher yourself to
  get the same benefit — an override is untouched by this change otherwise,
  and keeps firing on every pod-level `NodeNotReady` exactly as before.
- **The SonarCloud quality gate is a required check.** The scan step in CI
  waits on the gate and fails the component's job on `ERROR`, reported
  through `ci-green`. Contributor-facing only; nothing in the chart or the
  operator changes. See `CONTRIBUTING.md`, *Code analysis* and *Pull
  requests* (the `autofix` label).

### Fixed

- **`signal-ha` 0.3.0: the Home Assistant log lane observes records on a
  default Home Assistant install.** The adapter learned of live records only
  from `system_log_event`, which Home Assistant fires solely under
  `system_log: fire_event: true` — off by default, and named nowhere in this
  project. On such an install the adapter saw the log once per reconnect,
  judged every record "quiet" because nothing ever recurred from its point
  of view, and posted nothing while its source reported `Ready=True`. It now
  POLLS `system_log/list` every fifteen seconds and feeds what is newer than
  its cursor through the same rule path. The event stays as a lower-latency
  path where the instance fires it, and an occurrence is considered once
  whichever path brings it. `backfill: false` keeps its meaning by moving
  the cursor past the listing on connect. No configuration changes. The
  bundle's pinned tag moves to 0.3.0.

## [13.3.0] — 2026-08-28

**A third runtime: `agentops-runtime-copilot` 0.1.0, shipped as the `copilot`
bundle.** GitHub Copilot driven through its SDK, in process. The first vendor
to own its own tool vocabulary, its own agent-definition format and its own
session store — and every one of those is translated inside the runtime, so a
Pipeline binds the same toolsets it binds for claude. No manager, CRD or
toolset change was needed.

### Added

- `chart/charts/copilot/` — OFF by default. `copilot.enabled: true` with a
  GitHub token (`copilot.credentialsSecret.token`, or a Secret you manage)
  renders one `AgentRuntime` named `copilot` through the parent's shared
  renderer, inheriting `global.agentops.runtimeDefaults`. A route selects it
  with `pipelines[].runtimeRef: copilot`. `copilot.model` and
  `copilot.maxAiCredits` are optional.
- The runtime translates the composed allowlist into Copilot's two layers:
  `availableTools` (`Read` → `view`, `Bash` → `bash`, `mcp__s__t` →
  `mcp:s-t`) and a permission callback that decides every call. An unmapped
  pattern is withheld and logged. `mcp__<server>__*` is refused rather than
  widened to every server. `Bash(kubectl:*)` is enforced per invocation. An
  empty allowlist stays empty — Copilot's "no `tools:` means everything" never
  applies — and a denial is fed back to the model, never a hang.
- It reads `.github/agents/<agent>.agent.md`, Copilot's location, with the same
  frontmatter shapes and `merge` / `overwrite` composition as the reference
  runtime. Bound MCP servers reach Copilot from the same `mcp.json`, with
  secret placeholders resolved in the pod.
- The context id is minted by the runtime, so `runtimeContextId` is a handle
  it chose. State lives under `$HOME/.copilot/session-state/<id>/`, declared to
  `context-sync` by the bundle. A resume whose state is gone re-checks, then
  FAILS with `continuity: unavailable` and a readable reason.
- `docs/runtimes/copilot.md`, and the Copilot chip on the landing page.
- CI runs `node --test` for the Node runtimes (`runtimes/claude`,
  `runtimes/copilot`) when their directories change. Neither suite ran in CI
  before.

**Nothing changes for an install that does not enable the bundle.** The
rendered manifest is identical.

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

## Older versions

| Archive | Covers |
|---|---|
| [CHANGELOG-9.0-10.0.md](changelog/CHANGELOG-9.0-10.0.md) | chart 9.0.0 through 10.0.0 |
| [CHANGELOG-8.0.md](changelog/CHANGELOG-8.0.md) | chart 8.0.0 |
| [CHANGELOG-5.22-7.0.md](changelog/CHANGELOG-5.22-7.0.md) | chart 5.22.0 through 7.0.0 |
| [CHANGELOG-5.0-5.21.md](changelog/CHANGELOG-5.0-5.21.md) | chart 5.0.0 through 5.21.0 |
| [CHANGELOG-1.0-4.0.md](changelog/CHANGELOG-1.0-4.0.md) | chart 1.0 through 4.0.0 |
