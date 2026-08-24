## Why

**Context synchronisation ships inert on the install that knows how to use it.**

The mechanism exists because a node reboot corrupted the shared context
filesystem on 2026-08-20, taking every conversation's context AND stopping every
runtime pod from starting. Enabled, it fixes both: the live context moves to
pod-local storage, the durable volume holds a snapshot, and the agent container
holds **no mount of the durable volume at all** — so a corrupt volume cannot stop
a run already going, and no agent can read another conversation's context.

It is off. `chart/values.yaml` ships `runtime.contextSync.paths: []`, with the
correct value for the reference runtime **commented out two lines beneath it**.

The reasoning on record is that only the runtime knows where its backend keeps
context, and the chart cannot infer it. That is true, and it was applied one
layer too high. It is an argument about the CHART, and the declaration already
belongs to the runtime: the manager reads `AgentRuntime.spec.contextSync` and
hands the paths to the sidecar as `CONTEXT_SYNC_PATHS`. Nothing needs inferring —
`runtime-claude` knows where claude-code files transcripts, and the chart that
ships that runtime can say so. What ships instead is a chore for the operator,
and until they do it every install runs with the whole shared context volume
mounted into every agent container.

**And the mode cannot currently be defaulted, because it is broken without a
durable volume.** `sidecar` is gated on the runtime's declaration and the sidecar
image, but NOT on a context claim existing, and the branch then mounts
`ClaimName: cfg.ContextPVC` unconditionally. With persistence disabled that
renders a PersistentVolumeClaim reference whose name is the empty string.

Two things hide it. Every test in `contextsync_test.go` shares one config that
sets a claim, so the combination is never constructed. And `ContinuityPossible()`
gets it right — it returns false with no claim — so the manager correctly decides
to answer fresh and say so, then builds a pod that cannot start. The promise
logic and the pod builder disagree, and the reply promises a fresh answer while
the run fails to provision.

Rare today, because it needs someone to declare `contextSync` while running
without persistence. **Default the mode on and it is every conversation of every
persistence-disabled install.**

## What Changes

- **The no-durable-volume case falls back safely.** With no context claim, no
  sidecar is built and the pod is exactly today's ephemeral pod — no store mount,
  no durable promise. This joins the fallback that already exists for a missing
  sidecar image: same rule, one condition short.
- **The chart declares the reference runtime's context paths by default**, so
  synchronisation is on for a default install rather than waiting for an operator
  to type a fact the runtime already knows.
- **Nothing about the include list changes.** Empty paths remain invalid and
  remain loud: the chart renders no stanza for an empty list, and a hand-written
  empty list is refused by validation as one that "would persist nothing while
  appearing configured". The sidecar's own refusal to start on an empty list is
  untouched.
- **No migration is provided, and none is owed.** Opting in relocates the durable
  layout from the claim root to a per-conversation path, so a conversation
  holding a context handle fails its next run rather than answering without
  memory — the continuity rule working, with the existing reset verb as recovery.
  The project is pre-1.0 and unpublished; this is stated in the changelog rather
  than guarded.
- **The consequence of the default is stated rather than discovered**: `$HOME`
  becomes ephemeral on every conversation, holding caches and tool state as well
  as transcripts, bounded by `liveSizeLimit`. That becomes node ephemeral-storage
  pressure on every install rather than only on installs that opted in.

## Capabilities

### New Capabilities
<!-- none: this changes the behaviour of an existing capability -->

### Modified Capabilities
- `runtime-context-sync`: synchronisation requires a durable volume and falls
  back to the unsynchronised pod without one; and the mode is the default for a
  runtime whose context paths are known, rather than absent by default.

## Impact

**Code**

- `platform/manager/internal/runtimepod/podspec.go` — the `sidecar` predicate
  gains the missing condition. The `case sidecar:` branch stops being reachable
  without a claim.
- `platform/manager/internal/runtimepod/contextsync_test.go` — its shared config
  always sets a claim, so the fallback case needs a test that constructs the
  absence. Without one this defect is re-introducible.

**Chart**

- `chart/values.yaml` — `runtime.contextSync.paths` gains the reference
  runtime's value as its default, and the comment explaining why it was empty is
  replaced by one explaining what declaring it means.
- `chart/templates/runtime.yaml` — no change expected: it already renders the
  stanza from whatever `paths` holds.

**Documentation — the reference docs**

- `docs/CHANGELOG.md` — **required.** Behaviour changes for every install: the
  durable layout moves, and conversations holding a context handle fail their
  next run. The reset verb is the recovery and is named there.
- `docs/concepts.md` — `AgentRuntime.spec.contextSync` is no longer described as
  absent by default, and the no-volume fallback is part of how continuity
  resolves.
- `docs/installation.md` — the storage section, where the context volume and its
  values are decided. A reader turning persistence off must know what they get.

**Documentation — the adopter site**

- `docs/getting-started.md` — the read-only demo runs a real conversation; if it
  installs without a context volume it now takes the fallback path, and the page
  should not imply continuity it does not have.
- `docs/guides/agent-runtime.md` — it teaches running an agent on another
  backend, which is exactly the case where paths are NOT known and the operator
  must declare them.
- `docs/index.md` and `docs/introduction.md` — reviewed, not necessarily edited.

**Not affected**

- `platform/context-sync/` — no change. Its config contract, its refusal of an
  empty include list and its scan semantics are all untouched.
- `platform/manager/api/v1alpha1/contextsync_validate.go` — unchanged. It already
  rejects the empty list with the right message.
