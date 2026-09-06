# Changelog archive — chart 9.0.0 through 10.0.0

Migration guides for chart versions **9.0.0** and **10.0.0**, in
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) format.

Moved here from [CHANGELOG.md](../CHANGELOG.md), which holds the ten most recent
versions.

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
