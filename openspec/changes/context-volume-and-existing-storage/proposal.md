## Why

The volume holding an agent's accumulated context is named `home` — after a
filesystem accident, not after what it holds. The name came up from
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

## What Changes

- **BREAKING** — **`home` becomes `context` across the API and the chart.** The
  concept is the agent's accumulated context, so the object holding it is named
  for that:

  | Was | Becomes |
  |---|---|
  | `AgentRuntime.spec.home` | `AgentRuntime.spec.context` |
  | `persistence:` (top-level block) | `persistence.context` |
  | `runtime.homePvcRef` | `runtime.contextPvcRef` |
  | `HOME_PVC` (manager bootstrap env) | `CONTEXT_PVC` |
  | claim `agentops-home` | claim `agentops-context` |

- **`/data/home` and `HOME=/data/home` DO NOT MOVE.** The mount path really is
  the process home directory and claude-code keys `~/.claude/projects` off it
  (`runtimes/claude/runtime.js`). Renaming it would break context resume for
  every existing conversation to win a word. The split is the point: the PATH
  stays honest about being `$HOME`, the API stops pretending the volume is ABOUT
  being `$HOME`.
- **`AgentRuntime.spec.home` is DUAL-READ for one release**, exactly as retired
  `sessionId` was. A rename that merely moved the field would strand every
  installed runtime on upgrade.
- **THE CLAIM IS RENAMED TOO** — `agentops-home` becomes `agentops-context`.
  Nothing copies a volume, so this is guarded rather than trusted: an upgrade
  that would silently point a runtime at a new empty claim FAILS THE RENDER
  naming the one value to set, following the `agentops.generatedSecretGuard`
  precedent already in `chart/templates/_helpers.tpl`.
- **The migration moves no data and is one values line.** An existing install
  sets `persistence.context.existingClaim: agentops-home` and keeps the volume
  it has; the old claim already carries `helm.sh/resource-policy: keep`, so it
  survives leaving the manifest. Rebinding the PV under the new name is the
  documented alternative for anyone who wants the tidy name, and it uses this
  change's own `volumeName` support.
- **`storageClassName: "-"` disables dynamic provisioning**, adopting the
  convention prometheus-community, Bitnami and most charts already use: a name
  means that class, `"-"` renders an explicit `storageClassName: ""`, and
  undefined or `""` omits the field for the default provisioner. It is purely
  ADDITIVE — `""` keeps the meaning our template already gives it, so no install
  changes behaviour.
- **`selector` matches a pre-created volume by LABEL**, beside the `volumeName`
  that already exists — the same pair those charts ship, because a fleet of
  pre-created volumes is the shape this takes and naming one per release defeats
  the point.
- **Both, on BOTH volumes.**
- **The chart still renders NO `PersistentVolume`.** A PRE-created PV is by
  definition not the chart's to make; what was missing is the ability to point at
  one, and that is what this adds.
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
  `persistence` block; both spellings change, and the parent's ownership of the
  substrate now covers pointing at storage it did not create.
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
