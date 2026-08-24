## Context

See `proposal.md` — Why. Four facts shape the approach.

**The word is already right where the concept is described.**
`runtimeContextId`, `AgentRuntime.spec.contextStorage`, `platform/context-sync/`,
`contextProbe`, `CONTEXT_LIVE_DIR` — all correct. Only the OBJECT is misnamed, so
this is a rename inward toward vocabulary the repo already committed to, not a
new word being introduced.

**THE MOUNT PATH IS NOT LOAD-BEARING, AND AN EARLIER DRAFT OF THIS DESIGN SAID
IT WAS.** That draft kept `/data/home` on the grounds that `runtime.js` resolves
`${HOME}/.claude/projects` and moving it would break resume. The claim was
checked against the live volume and does not hold:

```
/<conversation>/current -> gen-4
/<conversation>/gen-4/.claude/projects/-data-workspace/<session>.jsonl
```

- **A claim's contents appear AT the mount path.** Mounting the same volume at
  `/data/context` shows the same bytes — nothing inside relocates.
- **The transcript directory is named for the WORKING DIRECTORY** —
  `-data-workspace` — not for `$HOME`.
- **`contextSync.paths` are RELATIVE to `$HOME`**, so they follow it.

**`/data/workspace` IS the load-bearing path**, it is what `invariants.md`
means, and this change does not touch it. The history is kept because the wrong
version is persuasive: both paths sit under `/data`, both are mounted from a
claim, and only one of them is baked into stored state.

**The claim NAME is live data, which is what makes renaming it a guarded
operation rather than an impossible one.** `helm.sh/resource-policy: keep` is
already on the rendered claim, so the old object survives leaving the manifest —
but nothing points the runtime back at it, so an unremarked rename yields a
second empty volume and an install that answers without context while every
signal reports success. The repo has met this exact shape before, in
`agentops.generatedSecretGuard`, and answered it by failing the render.

The storage half: `persistence.volumeName` renders on both claims today, but
`storageClassName` is emitted only when non-empty and the shipped default is
`""`. A claim naming a pre-created volume therefore also gets the cluster's
default StorageClass injected by admission, and is dynamically provisioned
against a volume the operator already made.

## Goals / Non-Goals

**Goals**

- One word for the volume, the field, the values block, the MOUNT PATH and the
  prose. No alias survives the change.
- The volume declared where the rest of a route's wiring already is.
- An upgrade that needs no values edit and moves no data.
- Binding to a pre-created volume expressible without a tri-state the operator
  has to reason about.
- Both volumes, every consumer.

**Non-Goals**

- **Moving `/data/workspace`.** THIS is the path that must not move — the
  transcript directory is named for it, so relocating it strands every stored
  context. `/data/home` was mistaken for it in an earlier draft.
- **Moving data.** The migration names the existing claim; nothing is copied.
- **Rendering a `PersistentVolume`.** Out of scope by the proposal's reading of
  "pre-created".
- **Renaming the `runtime-workspace-persistence` capability directory**, which
  covers both volumes and is misnamed for the same reason. Deferred: a spec path
  rename churns archive history for a filename, and it is not what the operator
  meets.
- **Touching the ordinary English word.** `state-durability`'s "every piece of
  live state has one declared home" is not this volume, and neither is Home
  Assistant anywhere in `signals/ha/`, `ha-bundle` or `docs/ha-bundle.md`. A
  blanket rename is how `AgentRuntimes` got into an RBAC `resources:` list.

## Decisions

### The claim is renamed, and the render is guarded

`persistence.context.name` defaults to `agentops-context`. The object is
renamed with the vocabulary, because a `kubectl get pvc` that still reads
`agentops-home` teaches the word this change exists to retire.

Nothing copies a volume, so the rename is GUARDED rather than trusted, following
`agentops.generatedSecretGuard` in `chart/templates/_helpers.tpl` — the same
technique against the same class of problem:

- **On upgrade, if a claim under the retired name exists in the namespace and
  neither `persistence.context.name` nor `existingClaim` was set explicitly, the
  render FAILS**, printing the one line that fixes it. An operator who has to
  read an error is in a strictly better position than one whose agents quietly
  forgot everything.
- **REBINDING THE PV UNDER THE NEW CLAIM NAME IS THE MIGRATION**, not the
  alternative. An earlier draft made `persistence.context.existingClaim:
  agentops-home` the recommended path — one values line, no data moved, and it
  works. It was rejected on the ground the rename exists for: a `kubectl get
  pvc` reading `agentops-home` forever teaches the retired word to everyone who
  runs that command, and the install never actually finishes migrating.
  - **It still moves no data.** The PV is retained, its claimRef cleared, and a
    claim under the new name binds to it by `volumeName` — which is the
    `volumeName` support this same change adds, and the reason to ship them
    together.
  - **`existingClaim` remains SUPPORTED** for an operator who genuinely wants to
    keep a claim they manage. It is simply not what the upgrade note leads with.

**The limitation, named:** the guard reads the cluster, so it is silent under
`helm template`, CI and a GitOps controller — exactly the caveat
`.claude/rules/gotchas.md` already records for `lookup`. `generatedSecretGuard`
accepted that trade for the same reason: a guard that catches the interactive
majority plus a `docs/CHANGELOG.md` entry beats no guard. **Argo CD users are
therefore covered by the CHANGELOG alone**, and that entry has to be written as
if it is the only warning, because for them it is.

### `storageClassName: "-"` — the convention, not a new one

Research settled this rather than judgement. prometheus-community's chart
documents it verbatim in its own values:

```
## If defined, storageClassName: <storageClass>
## If set to "-", storageClassName: "", which disables dynamic provisioning
## If undefined (the default) or set to null, no storageClassName spec is set,
##   choosing the default provisioner.
```

Bitnami's `common.storage.class` helper implements exactly the same rule
(`eq "-" $storageClass` → `storageClassName: ""`), and the two together are what
most charts an operator has already configured behave like.

- **It is ADDITIVE, which is what makes it the right answer here.** `""` is
  falsy in that helper, so empty keeps meaning "default provisioner" — which is
  what our template already does with `chart/values.yaml`'s shipped
  `storageClassName: ""`. No install changes behaviour.
- **`volumeName` and `selector` are the same pair those charts ship** beside it,
  for a volume by name and a volume by label. `volumeName` already exists here;
  `selector` is the gap.
- **Alternative rejected — a `staticVolume: {enabled, name, selector}` block.**
  It was the earlier draft and it is more foolproof: the broken combination
  becomes inexpressible. But it invents a fourth persistence shape for an
  operator who already knows three charts' worth of this one, and "the value
  every other chart uses" beats "the value that cannot be got wrong" when the
  reader is arriving from those charts.
- **Alternative rejected — flip `""` to mean "no class".** Breaks every install
  that copied the shipped values, and contradicts the convention besides.
- **Our value is spelled `storageClassName` where those charts spell it
  `storageClass`.** Keeping our spelling: it already exists, it matches the
  field it renders, and one rename per change is enough.

### PERSISTENCE MOVES TO THE PIPELINE

| Kind | Keeps | Why |
|---|---|---|
| `AgentRuntime` | `spec.contextStorage` (`volume` / `external` / `none`) | only the runtime knows whether its BACKEND writes context to a disk at all. A runtime storing context at a vendor API needs no volume and must not be given one |
| `Pipeline` | `spec.persistence.context` / `.workspace` | WHERE that disk is, is the route's decision |

**The split is BACKEND-SHAPE against PLACEMENT**, and it is the same cut that
already separates `AgentRuntime` from `Pipeline` everywhere else.

**THE PIPELINE IS NOT IN A FALLBACK CHAIN WITH THE RUNTIME**, because the
runtime no longer carries the field. The chain is:

```
pipeline.spec.persistence.<vol>  ->  the chart's release-wide claim  ->  ephemeral
```

and the CONVERSATION snapshots what that resolved to, exactly as it snapshots
`serviceAccountName`. Without the snapshot, editing a Pipeline changes which
volume an INFLIGHT conversation's next pod mounts — a storage change applied to
work already in progress, which is the sharper version of the privilege change
the identity snapshot exists to prevent.

### NO DUAL-READ. THE RETIRED SPELLINGS ARE DELETED

An earlier draft kept `spec.home` and `HOME_PVC` honoured for one release,
following the `sessionId` precedent.

**That precedent does not apply, and reading it as though it did is the trap.**
`sessionId` was a field RENAMED IN PLACE — same object, same meaning, so an
alias pointed at something real. `spec.home` has no replacement on
`AgentRuntime` at all: the concept moved to another CR. An alias would resolve
to a field that is not there, and the only honest behaviour is to fail.

- **A runtime still declaring `spec.home` therefore gets no volume**, and the
  upgrade note has to say so. That is louder than a silent alias, which is the
  point.
- **`HOME_PVC` goes with it.** The bootstrap default is chart-supplied, and the
  chart and the manager ship together in this release.

### A BINDING NAMES A CLAIM OR A VOLUME, AND THAT DECIDES WHO CREATES WHAT

`claimName` names a PVC that exists. `volumeName` names a `PersistentVolume`,
and **a pod cannot mount one** — only a claim on it. So supporting a PV at all
means something renders the claim, and which thing depends on who declared it:

| Declared on | Renders the claim |
|---|---|
| a Pipeline | the MANAGER |
| the chart | the CHART |

- **THIS IS THE ONE PLACE `NAMING IS NOT CREATING` DOES NOT HOLD**, and it is
  stated rather than smuggled. The manager gains `persistentvolumeclaims`
  create/get. It already creates Pods and ConfigMaps, so the category is not
  new — what is new is that the object OUTLIVES the conversation.
- **The created claim carries NO ownerRef on the Pipeline.** Deleting a Pipeline
  must never delete the accumulated context of every conversation it started.
  Storage is the one thing in this system whose loss is not recoverable by
  re-reconciling.
- **The pod's internal volume name changes too.** It is invisible outside the
  pod spec and pods are per-conversation, so there is nothing to migrate.

### Every consumer, or the rename is a bug

The claim name is resolved in five places and the change is not done until all
five follow both volumes: `chart/templates/runtime.yaml`, `deployment.yaml`
(the bootstrap env), `housekeeping.yaml`, `context-probe.yaml`, `pvc.yaml`.
`contextProbe.claimName` continues to default to the context claim only — it
probes the volume whose corruption is undetectable except at mount time, and
that is this one.

## Risks / Trade-offs

- **A partially-renamed tree reads as broken.** → The rename is mechanical but
  wide, and now includes the MOUNT PATH. Do it per-surface with the exclusion
  list above, and let the chart render tests and `runtimepod` fixtures be the
  check — both already pin these names.
- **THE MOUNT PATH MOVES AND THE PODS DO NOT RESTART TOGETHER.** → A runtime pod
  built before the upgrade has `HOME=/data/home`, one built after
  `/data/context`. They never coexist within a conversation: pods are
  per-conversation and strictly serial. An install with contextSync ON is safer
  still — the durable store is subPath'd per conversation and holds no absolute
  path.
- **A runtime CR still declaring `spec.home` silently loses its volume.** →
  Accepted, and made loud rather than aliased: the CHANGELOG says it plainly and
  the chart renders the new shape for the runtime it owns. An alias would point
  at a field that is no longer on that object at all.
- **The manager creating a PVC is a new power.** → Stated in the proposal rather
  than buried. The mitigation is ownership: no ownerRef on the Pipeline, so the
  one object whose loss is unrecoverable is never garbage-collected by a wiring
  edit.
- **A GitOps install upgrades straight past the guard** and gets an empty
  claim. → The single largest risk in this change. The `lookup` is silent
  without a cluster, so `docs/CHANGELOG.md` is the only thing standing between
  an Argo user and a silently context-less install: it is written first, leads
  with the values line, and says explicitly that no in-cluster check will stop
  it.
- **`"-"` is a magic string.** → Accepted deliberately: it is the magic string
  the ecosystem already uses, and the values comment is copied from the same
  four lines prometheus-community ships, so a reader recognises it.
- **A pre-created volume that will not bind fails as a `Pending` claim**, which
  looks exactly like no provisioner. → `docs/getting-started.md` already carries
  the RWX troubleshooting row naming the claim; it gains the static-binding case
  beside it, since the symptom is shared and the cause is not.

## Migration Plan

1. **The manager and the chart ship TOGETHER this time.** The earlier draft
   staged them — manager first, dual-reading both spellings — and that stopped
   being possible once the field moved to another CR. No version understands
   both shapes, so there is nothing to stage.
2. **Read `docs/CHANGELOG.md` before upgrading.** For a GitOps install it is the
   only warning that arrives: the claim-rename guard reads the cluster, and Argo
   renders without one.
3. **Move any `AgentRuntime.spec.home` onto the Pipelines using that runtime**,
   as `spec.persistence.context.claimName`. A runtime declaring the retired
   field contributes nothing after the upgrade.
4. **Rewrite the flat `persistence.*` values** under `persistence.context`. The
   render fails naming any that are missed, and that guard needs no cluster.
5. **Rebind the volume under the new claim name** — retain the PV, clear its
   `claimRef`, and let the chart's claim bind it by `volumeName` with
   `storageClassName: "-"`. No data moves.
6. **`helm upgrade`.**
7. **Rollback is a chart downgrade plus restoring the runtime field**, and the
   volume is untouched throughout, which is what makes it recoverable.
