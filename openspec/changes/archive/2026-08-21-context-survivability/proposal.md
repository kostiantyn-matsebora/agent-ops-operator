## Why

On 2026-08-20 a node reboot corrupted the ext4 filesystem on the RWX
`agentops-home` volume. Longhorn reported the volume `healthy` throughout, and
was right to: it replicates BLOCKS, and all three replicas agreed on the corrupt
ones. Storage reporting healthy is not evidence that a filesystem is.

Every runtime pod mounts that one volume. So five pods sat in
`ContainerCreating` for fifteen hours, held every capacity slot
(`MAX_ACTIVE_CONVERSATIONS` is 5), and starved six `Pending` conversations
behind them. The install was completely down.

It reported NOTHING. No condition, no event, no manager log line. The only
condition present on the stuck conversations was
`DeliveryPending=False / AllDelivered`, which reads as healthy. An operator
watching the console saw a queue that had stopped moving and nothing that said
why.

Three separate defects made that possible, and each is worth naming because each
has its own fix:

1. **A runtime pod that never STARTS is immortal.** Reaping handles
   `Succeeded` and `Failed`; `Pending` is counted as active — correctly, since a
   stuck pod must not invent capacity — but nothing bounds how long it may sit
   there.
2. **The storage breaker is armed on the wrong edge.** `internal/httpapi/continuity.go`
   already holds exactly the right doctrine — unavailability is an OUTAGE before
   it is a LOSS — but it fires only when a run REPORTS an unreachable context. A
   pod that never starts files no report, so the breaker built for this incident
   never saw it.
3. **The live context IS the shared volume.** There is no copy, no generation,
   and no isolation: the agent container mounts the claim ROOT, so one damaged
   filesystem takes every conversation's context with it.

Reboots are not going away. This change makes agent-ops survive one.

## What Changes

**A — the wedge cannot happen again**

- **Runtime pods get a start deadline.** A pod that has not reached `Running`
  within `RUNTIME_START_DEADLINE` is reaped, with per-conversation exponential
  backoff. The kubelet's OWN reason is recorded verbatim into a new
  `RuntimeStarted` condition — never a bare "timed out", which would have left
  the operator exactly as blind as they were.
- **Stuck pods are REAPED, never exempted from the cap.** Un-counting them is
  the invent-capacity mistake the capacity rules already forbid, and it is the
  tempting wrong fix here.
- **The existing breaker gains the provisioning edge.** Repeated start failures
  attributable to storage open the SAME breaker: admit nothing, hold work in
  `Pending`, probe with one canary. Held, never failed.
- **It is surfaced as STATUS, never as a SIGNAL** — `Conversation.RuntimeStarted`,
  Kubernetes events, an install-level METRIC for the open breaker, and the
  console. Routing agent-ops' own health back through ingest is the loop the
  self-exclusion rules exist to prevent.
  The install-wide fact is a metric rather than a condition on `AgentRuntime`:
  that CR has no reconciler, so a condition there would mean inventing a
  controller whose only purpose was to carry it.

**B — the corruption is made unlikely, and survivable when it happens**

- **Runtime pods release ahead of a node going down.** A cordon or `NoSchedule`
  taint stops admission on that node and releases idle pods there, through the
  `/exit` path that already means "release the runtime, keep the conversation".
  The last consumer leaving lets the volume detach and the filesystem unmount
  CLEANLY before the reboot.
- **A new dependency-free module, `context-sync/`** — a sidecar in the runtime
  pod, in the same family as the channel and signal adapters, `telegram-router`
  and `housekeeping`. It is NOT called a manager: in this codebase that word
  means the operator.
- **The home claim gains a per-conversation subPath.** It is mounted at the
  claim ROOT today, unlike workspace, so every conversation's sessions land in
  one shared tree. Copying and dumping that tree from concurrent pods would have
  them clobber each other, so this is a PREREQUISITE, not a tidy-up.
- **The agent container gets an emptyDir; the PVC is mounted on the SIDECAR
  ONLY.** The live context is local disk. A consequence worth having on its own
  merits: an agent can no longer read another conversation's context or write to
  durable storage at all.
- **The sidecar proxies the work contract.** `CONTROL_URL` points at it, and it
  forwards to the manager. That is how it learns work boundaries with ZERO
  changes to any runtime image: restore before answering the first `GET /work`,
  and checkpoint on `POST /work/done` BEFORE forwarding — which buys the
  invariant that if the CR names a context handle, the volume provably has it.
- **Periodic dumps are configurable, conditional and incremental.** Skipped
  entirely when a stat manifest shows nothing changed. When something did,
  `rsync --link-dest` against the previous generation copies only what changed.
  That second half is load-bearing rather than an optimisation: a naive periodic
  full copy over NFS would ADD writes to the fragile filesystem and make the
  original problem worse.
- **Dumps are atomic and generational** — a new generation directory, then a
  `current` symlink swapped by rename, retaining N. Each is tagged `quiesced`
  (taken at a work boundary) or `bestEffort` (taken mid-run). Mid-run dumps are
  NOT skipped: a long run is precisely the crash case worth covering, and a torn
  newest generation is recoverable by falling back one.

**C — the filesystem is watched, and its loss has a verb**

- **Corruption is detected at rest**, on a period, instead of at the next mount
  attempt. `fsck` runs only at mount, and an idle-detached volume can carry
  latent damage for a day — which is exactly what happened.
- **A conversation whose context is gone can be RESET explicitly**, by an
  operator, and the conversation continues SAYING SO. Promised-and-lost still
  fails rather than quietly answering fresh — this adds a way out, it does not
  soften the rule.
- **A known-bad volume no longer stops the system.** The pod starts WITHOUT the
  home mount and the conversation is marked context-lost, so the failure costs
  continuity rather than availability.

**D — every context operation is visible**

- **The sidecar reports each operation to the manager**, over the channel the
  proxy already gives it.
- **The manager emits it into `internal/activity`**, which yields the console
  Sequence tab AND Prometheus from ONE instrumentation pass — the registry is
  registered as an `Observer` precisely so a metric and its event cannot drift
  apart. Kinds: `context.restore`, `context.checkpoint`, `context.skip`,
  `context.failed`, carrying duration and bytes.
- **The durable half stays on the CR**: `status.contextCheckpoint` (time,
  generation, quiesced). Written ONLY when a dump actually happened, never on a
  skip — the rule cooldown already follows, and what keeps a two-minute period
  from becoming a write storm.

**New API surface**, on `AgentRuntime.spec.contextSync` — the RUNTIME declares
it, exactly as `contextStorage` already does, because the chart cannot infer
where a backend keeps its context: `paths` (include globs relative to `HOME`),
`exclude` (churn inside them), `interval` (0 means work-boundary checkpoints
only) and `retain`. An INCLUDE list means caches are excluded by construction.

Not in scope: per-conversation RWO volumes, `contextStorage: external` backends,
backing up the home volume, and any change to how Longhorn is configured.

## Impact

- `internal/controller/conversation_controller.go`: the start deadline and its
  backoff, the `RuntimeStarted` condition, node-cordon-aware admission and idle
  release. Capacity counting in `liveRuntimePods` is UNCHANGED.
- `internal/httpapi/continuity.go`: the breaker gains a provisioning-failure
  input and a canary probe; its window and threshold semantics are unchanged.
- `internal/runtimepod/podspec.go`: home gains a per-conversation subPath, the
  agent container's `/data/home` becomes an emptyDir with a `sizeLimit`, the
  sidecar container is built and the PVC moves onto it, `CONTROL_URL` is
  redirected to localhost, and `terminationGracePeriodSeconds` grows enough for
  a final dump.
- `context-sync/`: new module, own `go.mod`, no dependencies outside itself.
- `api/v1alpha1`: `AgentRuntime.spec.contextSync`, `Conversation.status.contextCheckpoint`,
  the `RuntimeStarted` condition; deepcopy and CRDs regenerated.
- `internal/activity`: four new event kinds; metrics follow with no second pass.
- `console/ui`: the Sequence tab renders them, and a blocked conversation says
  why. Screenshots are build output and must be regenerated.
- `chart/`: `runtime.contextSync.*` values, the sidecar image, the reference
  runtime's shipped `paths`, and the AgentRuntime template.
- `housekeeping/`: must respect the cordon rule, since it mounts the same claim
  root.
- Docs: `docs/concepts.md`, `docs/contracts.md`, `docs/installation.md`,
  `CHANGELOG.md` (the home subPath is a migration), and `CLAUDE.md`.
- An install that does not set `contextSync` keeps today's behavior exactly.
