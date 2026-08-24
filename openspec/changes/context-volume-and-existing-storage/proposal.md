## Why

**Two things are wrong, and they are the same mistake at two layers.**

**The volume is misnamed.** It holds an agent's accumulated context and is
called `home` — after a filesystem accident, not after what it holds. The name came up from
`runtimes/claude/Dockerfile` (`ENV HOME=/data/home`) and propagated outward into
the CRD field, the chart values and every sentence describing them. It is the
same mistake `terminology.md` already bans twice: `session` is claude-code's
noun for the handle, `worker` was the vendor-shaped name for a runtime. The
repo already uses the right word everywhere the concept is DESCRIBED —
`runtimeContextId`, `contextStorage`, `context-sync`, `contextProbe`,
`CONTEXT_LIVE_DIR` — and the wrong one everywhere it is NAMED.
`internal/runtimepod/contextsync.go` sets `CONTEXT_LIVE_DIR` to `/data/home` in
one line, which is the whole problem in one line.

And the volume cannot actually be pointed at a pre-created PV. `volumeName`
exists on both claims, but binding to a static PV requires `storageClassName:
""` — an EXPLICIT empty string, which is what disables dynamic provisioning.
The template treats empty as "omit the field", the admission plugin then injects
the cluster default StorageClass, and the claim is dynamically provisioned
against a PV the operator already made. The value is documented as `"" = cluster
default StorageClass`, so there is no spelling of "no storage class" at all.

**And the volume is declared on the wrong object.** `AgentRuntime` says where a
conversation keeps its state, but a runtime is an ENGINE — an image and its
pod-level defaults. Where a route's conversations persist is a property of the
ROUTE, decided beside the tools it grants, the channels it delivers to, the
runtime it selects and the identity it executes under. All four of those are
already Pipeline fields.

This is the same argument that moved `serviceAccountName` off `AgentRuntime`,
and it failed the same symmetry test: two Pipelines sharing one runtime cannot
keep their conversations on different volumes without cloning the runtime, which
is exactly what cloning a runtime to get a second trust level used to require.
Whoever is trusted to grant an agent tools and a cluster identity is more
qualified to say where its context lives, not less.

## What Changes

- **BREAKING** — **`home` becomes `context`, EVERYWHERE, including the mount
  path.** The concept is the agent's accumulated context, so every object,
  field, value and path holding it is named for that:

  | Was | Becomes |
  |---|---|
  | `AgentRuntime.spec.home` | **deleted** — see below, it moves to the Pipeline |
  | mount path `/data/home`, `HOME=/data/home` | `/data/context`, `HOME=/data/context` |
  | `persistence:` (top-level block) | `persistence.context` |
  | `runtime.homePvcRef` | **deleted** with the runtime field |
  | `HOME_PVC` (manager bootstrap env) | `CONTEXT_PVC` |
  | claim `agentops-home` | `agentops-context` |

- **THE MOUNT PATH MOVES, and the earlier draft was wrong to keep it.** That
  draft argued `/data/home` was load-bearing because claude-code keys stored
  context off `$HOME`. Verified against the live volume, it is not:

  - A claim's contents appear AT the mount path, so mounting the same volume at
    `/data/context` shows the same files. Nothing inside the volume relocates.
  - The transcript directory is named for the WORKING DIRECTORY —
    `.claude/projects/-data-workspace/` — not for `$HOME`. That path is the one
    `invariants.md` calls load-bearing, and it does not move.
  - `contextSync.paths` are RELATIVE to `$HOME`, so they follow it.

  **`/data/workspace` still does not move.** It is the genuinely load-bearing
  path, and this change does not touch it.

- **NO DUAL-READ. The retired spellings are DELETED, not deprecated.** An
  earlier draft kept `spec.home` and `HOME_PVC` honoured for one release. They
  are gone: the field they aliased has itself moved to another CR, so an alias
  would point at a concept that no longer lives there.

- **BREAKING** — **PERSISTENCE IS WIRING. It moves from `AgentRuntime` to
  `Pipeline`.** A runtime does not decide where a route's conversations keep
  their state, for the same reason it stopped deciding their identity: the
  Pipeline already grants tools, MCP servers, channels, the execution identity
  and the runtime itself, and storage belongs beside them.

  | Kind | Keeps | Loses |
  |---|---|---|
  | `AgentRuntime` | `spec.contextStorage` — only the runtime knows whether its BACKEND uses a disk at all | the volume |
  | `Pipeline` | `spec.persistence.context` / `.workspace` | — |

- **A BINDING NAMES A CLAIM OR A VOLUME**, at either level, and what gets
  created follows from which:

  | Set where | Who creates the claim | The conversation gets |
  |---|---|---|
  | Pipeline, `claimName` | nobody — it exists | that claim |
  | Pipeline, `volumeName` | **the manager** | the claim it created |
  | Chart, `existingClaim` | nobody | it, for pipelines binding neither |
  | Chart, `volumeName` | **the chart** | same |
  | Chart, neither, persistence on | **the chart**, as the release default | same |
  | persistence off | nobody | EPHEMERAL — except a pipeline binding its own |

- **THE MANAGER CREATES A CLAIM, and that is a new power stated plainly.** It
  gains `persistentvolumeclaims` create/get, and this is the one place
  `NAMING IS NOT CREATING` does not hold — because a pod cannot mount a
  `PersistentVolume`, only a claim on one, so supporting a PV at all means
  something renders the claim.
  - **The created claim carries NO ownerRef on the Pipeline.** Deleting a
    Pipeline must never delete the accumulated context of the conversations it
    started.
- **The resolved claim is SNAPSHOTTED into the Conversation**, exactly as
  `serviceAccountName` is. Otherwise editing a Pipeline moves an INFLIGHT
  conversation's storage out from under it.
- **`storageClassName: "-"` disables dynamic provisioning**, adopting the
  convention prometheus-community, Bitnami and most charts already use. Purely
  ADDITIVE — `""` keeps the meaning our template already gives it.
- **`selector` matches a pre-created volume by LABEL** beside `volumeName`, at
  chart level, on both volumes.
- **The chart still renders NO `PersistentVolume`.** A pre-created volume is by
  definition not the release's to make.
- **Every consumer follows both volumes** — `runtime.yaml`, `deployment.yaml`,
  `housekeeping.yaml`, `context-probe.yaml` — or the rename recreates the
  two-spellings-of-one-fact problem.

## Capabilities

### New Capabilities
<!-- none: this renames and completes existing behaviour -->

### Modified Capabilities

- `runtime-workspace-persistence`: the home volume is renamed to the context
  volume throughout; the chart gains the ability to bind either claim to a
  pre-created PersistentVolume rather than only to a pre-created claim.
- `agent-runtime-ownership`: the wiring requirement names `home.pvcRef` and the
  `persistence` block; both spellings change, the parent's ownership of the
  substrate now covers pointing at storage it did not create, and the volume
  itself stops being the runtime's to declare.
- `pipeline-model`: the Pipeline gains `spec.persistence`, so a route declares
  where its conversations keep state beside what they may reach and whose
  credentials they run under.
- `conversation-context-continuity`: `contextStorage` is described against "its
  home volume"; the sentence becomes coherent once the volume carries the same
  word as the setting that governs it.

## Impact

**Code and API**

- `platform/manager/api/v1alpha1` — `AgentRuntime.spec.context`, `spec.home`
  retained as deprecated and dual-read. Regenerate deepcopy and
  `chart/files/crds/agentops.dev_agentruntimes.yaml`.
- `platform/manager/internal/runtimepod/` — `podspec.go`, `contextsync.go`, the
  volume name, and the `HOME_PVC` bootstrap env.
- `chart/templates/` — `pvc.yaml`, `runtime.yaml`, `deployment.yaml`,
  `housekeeping.yaml`, `context-probe.yaml`, `_helpers.tpl`.
- `chart/values.yaml` — the `persistence` block.
- `platform/console/` — wherever a claim name or volume label is displayed.
- Test fixtures pinning `home` in `runtimepod` and the chart render tests.

**Reference docs**

- `docs/concepts.md` — the volume and its field.
- `docs/cr-reference.md` — GENERATED; re-run `.github/scripts/docs-generate.py`.
- `docs/guides/agent-runtime.md` — prose is hand-written, its `yaml` blocks are
  generated from markers.
- `docs/CHANGELOG.md` — breaking rename plus the upgrade step, newest first.
- `docs/k8s-bundle.md`, `docs/console.md` — incidental mentions.

**Adopter site**

- `docs/installation.md` — the four `persistence.*` rows in the values table,
  plus a new decision: pointing at storage the chart did not create.
- `docs/getting-started.md` — the `agentops-home` PVC named in the RWX
  troubleshooting row.

**Context rules**

- `.claude/rules/terminology.md` — the context volume joins the banned-word
  table beside `session`, `worker` and `wake`.
- `.claude/rules/invariants.md`, `wiring.md` — "home volume" appears in the
  substrate-ownership statements.
