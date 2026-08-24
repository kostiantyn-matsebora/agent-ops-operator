# Changelog

All notable changes to this project are documented here, in
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) format. Versions are
CHART versions. Image tags move independently and are named in each entry.

This file holds the **ten most recent versions**. Older entries are in
`changelog/`, linked at the [foot of this page](#older-versions).

See [../README.md](../README.md) for the product overview and [./](./) for
reference material. `CLAUDE.md` in this directory owns the rules this file
follows.

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

Adds the Home Assistant bundle. See [home-assistant.md](home-assistant.md).

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

## [5.21.0] — 2026-08-21

On 2026-08-20 a node reboot corrupted the ext4 filesystem on the shared context
volume — its claim was at the time named `agentops-home`, the former spelling.
Longhorn reported the volume **healthy** throughout, correctly: it replicates
blocks, and all three replicas agreed on the corrupt ones.

Every runtime pod mounts that volume. Five pods sat in `ContainerCreating` for
fifteen hours, held every capacity slot, and starved six more conversations. The
install was completely down and **said nothing**. The only condition present read
`DeliveryPending=False / AllDelivered`, which looks like health.

### Added

| Value | Default | What it does |
|---|---|---|
| `runtime.contextSync.paths` | `[]` | moves the live context to pod-local storage, keeping a snapshot on the volume |
| `rbac.drainAware` | `false` | releases idle runtime pods from a cordoned node so the filesystem unmounts cleanly |
| `contextProbe.enabled` | `false` | hourly mount probe, so a damaged idle volume is found in an hour rather than at next use |

- `Conversation.status` carries **`RuntimeStarted`**, whose message is the
  kubelet's own words. A bare "deadline exceeded" would have reproduced the
  original failure with a timer attached.
- New verb `POST /channel/conversations/{name}/reset-context` clears a
  conversation's context handle and keeps the conversation, its threads and its
  history. It states the loss on every bound thread. It is operator-initiated
  only — an automatic version would be indistinguishable from the silent
  degradation the continuity rules forbid.
- New metrics: `agentops_storage_outage`, `agentops_storage_outage_seconds`,
  `agentops_context_operations_total`, `agentops_context_checkpoint_bytes`.

### Changed

- **A runtime pod that never starts is now reaped** after
  `RUNTIME_START_DEADLINE` (10m), with per-conversation exponential backoff. It
  still counts against the cap until it is gone. Un-counting it would provision
  past the cap against resources the cluster has not released.
- **The storage breaker gained a second edge.** It already treated many failed
  continuations as an outage. It now also counts pods that cannot be provisioned
  for a storage reason. That is why it never fired for the incident it was
  written for — no pod started, so no run existed to report. While open it holds
  work in `Pending` with a reason naming STORAGE rather than queue, and re-tests
  with one canary.

### Upgrade

`helm upgrade` with the new image tags. No values change is required and nothing
new is on by default. Rolling back is reverting the images.

**`contextSync` needs `paths`.** Only the runtime knows where its backend keeps
context, and the chart must not guess. For the reference runtime:

```yaml
runtime:
  contextSync:
    paths: [".claude/projects/-data-workspace/**"]
```

With it set, the agent container gets ephemeral storage and **no mount of the
durable volume at all**. Only the sidecar holds it.

A run already going then survives the volume going bad underneath it. An agent
can neither read another conversation's context nor corrupt the filesystem.

**Opting in strands existing context, visibly.** Without the sidecar, context
sits at the claim root. With it, each conversation reads a per-conversation
subdirectory, which starts empty.

Every conversation holding a context handle will therefore FAIL its next run
rather than answer without memory. That is the continuity rule working, not a
defect.

Recover each one:

```sh
curl -sX POST "$MANAGER/channel/conversations/<name>/reset-context" \
  -H "Authorization: Bearer $ADAPTER_TOKEN" -H 'Content-Type: application/json' \
  -d '{"channel":"<a channel it is bound to>"}'
```

Enable `contextSync` on a quiet install, or accept that live conversations lose
their memory once and say so.

**`rbac.drainAware` costs the manager its first cluster-scoped grant** — nodes
get/list/watch, read-only. Every other permission it holds is namespaced. It
shrinks the corruption window without closing it, because the storage provider
picks where a shared volume is served independently of where runtime pods run.

## Older versions

| Archive | Covers |
|---|---|
| [CHANGELOG-5.0-5.20.md](changelog/CHANGELOG-5.0-5.20.md) | chart 5.0.0 through 5.20.0 |
| [CHANGELOG-1.0-4.0.md](changelog/CHANGELOG-1.0-4.0.md) | chart 1.0 through 4.0.0 |
