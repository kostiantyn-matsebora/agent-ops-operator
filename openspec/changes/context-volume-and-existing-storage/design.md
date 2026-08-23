## Context

See `proposal.md` — Why. Three facts shape the approach.

**The word is already right where the concept is described.**
`runtimeContextId`, `AgentRuntime.spec.contextStorage`, `platform/context-sync/`,
`contextProbe`, `CONTEXT_LIVE_DIR` — all correct. Only the OBJECT is misnamed, so
this is a rename inward toward vocabulary the repo already committed to, not a
new word being introduced.

**`/data/home` is load-bearing and `home` the word is not.**
`runtimes/claude/Dockerfile` sets `ENV HOME=/data/home`, `podspec.go` sets the
same, and `runtime.js` resolves `${HOME}/.claude/projects`. The path is the
process home directory in fact. Every OTHER appearance of the word is a label.

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

- One word for the volume, the field, the values block and the prose.
- An upgrade that needs no values edit and moves no data.
- Binding to a pre-created volume expressible without a tri-state the operator
  has to reason about.
- Both volumes, every consumer.

**Non-Goals**

- **Moving `/data/home`.** Stated as a goal's inverse because it is the change's
  most tempting over-reach.
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
- **The migration moves no data:** `persistence.context.existingClaim:
  agentops-home` keeps the volume that exists. The chart then renders no claim,
  and `resource-policy: keep` on the live object means Helm leaves it alone.
- **Rebinding the PV under the new name is the documented alternative**, for
  anyone who wants the tidy name — and it is done with the `volumeName` support
  this same change adds, which is a reason to ship them together.

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

### The dual-read follows the `sessionId` precedent exactly

`AgentRuntime.spec.context` is added; `spec.home` stays, marked deprecated, for
one release.

- **ONE read point**, an accessor on the type, mirroring
  `Conversation.ContextID()` — the retired field is read there and nowhere else.
  `conversation_context_test.go` is the shape of its test: new wins, old is
  honoured, adoption re-records under the new name.
- **`HOME_PVC` → `CONTEXT_PVC` is dual-read the same way** in
  `cmd/manager/main.go`, because the manager and the chart upgrade at different
  moments and a bootstrap default that vanishes for one reconcile provisions
  pods with no volume.
- **The pod's internal volume name changes with it.** It is invisible outside
  the pod spec and pods are per-conversation, so there is nothing to migrate —
  only `runtimepod` fixtures to update.

### Every consumer, or the rename is a bug

The claim name is resolved in five places and the change is not done until all
five follow both volumes: `chart/templates/runtime.yaml`, `deployment.yaml`
(the bootstrap env), `housekeeping.yaml`, `context-probe.yaml`, `pvc.yaml`.
`contextProbe.claimName` continues to default to the context claim only — it
probes the volume whose corruption is undetectable except at mount time, and
that is this one.

## Risks / Trade-offs

- **A partially-renamed tree reads as broken.** → The rename is mechanical but
  wide (~325 bare `home`, 32 `HomePVC`, 12 `homeVolume` outside Home Assistant).
  Do it per-surface with the exclusion list above, and let the chart render
  tests and `runtimepod` fixtures be the check — both already pin these names.
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

1. **Manager first, chart second.** The manager dual-reads both the CR field and
   the bootstrap env, so it is safe against an unmodified chart.
2. **Read `docs/CHANGELOG.md` before upgrading the chart** — for a GitOps
   install it is the only warning that arrives.
3. **`helm upgrade` FAILS** where a claim under the retired name exists and
   nothing said what to do with it. Set
   `persistence.context.existingClaim: agentops-home` and upgrade again — that
   is the whole migration, and it copies nothing.
4. **Optional, later and deliberate:** rebind the PV under the new claim name,
   using the `volumeName` support this change adds.
5. **Rollback is a chart downgrade**, and it is clean at every step because no
   data has moved.
