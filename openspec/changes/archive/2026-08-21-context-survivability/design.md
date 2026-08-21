## Context

The incident is documented in the proposal. What matters for design is which
parts of the system were ALREADY right and only pointed the wrong way.

`internal/httpapi/continuity.go` is the clearest case. Its comment states the
doctrine this change needs, in full:

> Unavailability is treated as an OUTAGE before it is treated as a LOSS. […] A
> runtime pod cannot see this — it handles one unit and exits. The manager can,
> because it sees every run, and it already holds cross-conversation state for
> admission and cooldown, so this is the same shape rather than a new idea.

That is exactly the reasoning the outage needed. It never ran, because the
breaker's only input is a run REPORTING an unreachable context, and no run ever
started. The design instinct here is therefore to EXTEND, not to add a second
mechanism with overlapping meaning — two breakers disagreeing about whether
storage is down would surface as a bug report far from either.

The same is true of `internal/activity`: the Prometheus registry is registered
as an `Observer` so that "a metric observation and its event cannot occur
independently or drift apart". Telemetry for the new component is therefore one
call, not two.

And `/exit` already means precisely "release the runtime pod and nothing else —
object, threads, inputs, runs and `runtimeContextId` all survive". That is the
verb a node drain needs. It does not need a new one.

Three facts about the current pod shape drive the rest:

- `podspec.go:186` mounts the home PVC at the CLAIM ROOT, with no subPath —
  unlike workspace at `podspec.go:171`, which is subPath'd per conversation.
- `runtime-claude` sets `HOME=/data/home` (`Dockerfile:20`) and files sessions
  under `$HOME/.claude/projects/` (`runtime.js:302`).
- cwd is `/data/workspace` for EVERY conversation, so claude-code derives one
  directory name — `-data-workspace` — and every conversation's sessions land in
  the same tree, separated only by session id.

Together those mean the shared home is not incidental. It is load-bearing today,
and any copy-in/copy-out design must dismantle it first.

## Goals / Non-Goals

**Goals**

- A storage failure of any kind costs continuity, never availability.
- No runtime image has to change, including ones adopters bring themselves.
- The live context is not on the fragile filesystem.
- Every context operation is visible in the console and in metrics.
- An install that does not opt in behaves exactly as it does today.

**Non-Goals**

- Preventing filesystem corruption outright. That is Longhorn's and the node's
  business, and this change assumes reboots keep happening.
- Per-conversation RWO volumes. A stronger isolation story, deliberately
  deferred — it changes provisioning, and this change is already wide.
- Backing up the home volume. Real and needed, and a separate concern.
- Automatic repair of a corrupt filesystem. `fsck -y` can lose data, and that is
  an operator's decision, not a reconciler's.
- Any change to `contextStorage: external` or `none` behavior.

## Decisions

### 1. The breaker is extended, not duplicated

Provisioning failures feed the SAME breaker as continuity reports. The
threshold and window semantics stay as they are: three signals inside two
minutes is infrastructure, not coincidence.

A provisioning failure counts only when the pod's own failure reason is
storage-attributable — an attach or mount failure. A pod stuck for an image pull
or unschedulable for resources is a different fault and must not open a STORAGE
breaker, or the first busy afternoon holds all work for the wrong reason.

While open: admit nothing, hold in `Pending`, and probe with ONE canary rather
than letting every waiting conversation retry. That is the difference between a
recovery and a thundering herd against a filesystem that is already unwell.

### 2. Reap the stuck pod; never exempt it from the cap

`liveRuntimePods` counting `Pending` is correct and stays. The capacity rules
say a stuck pod must not invent capacity, and un-counting one would do exactly
that — the reconciler would provision a sixth pod against a cap of five while
the fifth still holds a slot the cluster has not released.

The fix is that the pod stops existing, which frees the slot through the DELETE
watch that already promotes the FIFO-first waiter. No new scheduling path.

### 3. The deadline records the kubelet's reason, not its own

A `RuntimeStarted=False` condition whose message is "start deadline exceeded"
would reproduce the outage's real failure — fifteen hours with nothing that said
why. The condition carries the pod's own evidence: the unmet pod condition and
the most recent related event, verbatim.

This is also what makes the breaker's attribution possible at all, so the two
decisions are one mechanism read twice.

### 4. The sidecar proxies the work contract

Two rejected alternatives first.

**The runtime image does its own copying.** It knows where its context lives, so
this is the least machinery. Rejected because every backend an adopter brings —
aider, a local model, something custom — must reimplement it with its own
failure handling, and the whole point of a sidecar is to write it once.

**The runtime signals the sidecar over a socket.** Better semantics: dumps
happen when context is quiesced. Rejected for the same reason — it reintroduces
per-runtime implementation, only smaller.

The proxy gets the second option's semantics at the first option's cost. The
runtime already receives `CONTROL_URL` from env and already long-polls
`GET /work` and posts `POST /work/done`, because the work contract requires it.
Point `CONTROL_URL` at localhost and the sidecar observes every boundary while
the image stays untouched.

It also buys an ordering guarantee: checkpoint BEFORE forwarding `/work/done`
means the manager cannot record a context handle the volume does not have.
Without it, the CR could name a handle whose bytes were never dumped, and the
next run would fail a continuation that should have worked.

The costs are real and are accepted: the sidecar is on the critical path for
work dispatch, the 25-second long-poll needs correct pass-through and
cancellation, and a hung sidecar presents as a hung `/work`. A sidecar failure
makes the pod fail, which the existing reap path already handles.

### 5. Per-conversation subPath on home, before anything else

Without it, two concurrent pods restore the same shared tree and dump it back,
each erasing the other's writes. With it the trees are disjoint and both
directions are trivially correct.

The usual objection — every conversation now keeps its own tool caches — does
not apply, because caches live on the emptyDir and are never dumped at all.

This IS a migration: existing context sits at the claim root and must be moved
under per-conversation directories, or every conversation starts fresh on
upgrade.

### 6. Include globs, not exclude globs

`contextSync.paths` is an INCLUDE list. Caches, telemetry and lock files are
then excluded by construction rather than by an exclude list that must chase
every new file a vendor invents. `exclude` exists only for churn INSIDE the
included tree.

The runtime declares it, following `contextStorage`'s own stated reasoning —
the chart would have to know which images keep context where, and it must not.

### 7. Conditional and incremental are two different requirements

**Conditional**: a stat-walk manifest of `(path, size, mtime)` over the
emptyDir, compared with the manifest from the last dump. Cheap because the live
copy is local disk. inotify may skip the walk, but the scan remains the source
of truth — watch limits and queue overflow make inotify a good accelerator and a
poor oracle.

**Incremental**: `rsync --link-dest` against the previous generation, so
unchanged files become hardlinks and only changed bytes cross the wire.

The second is load-bearing. A conditional-but-full dump every two minutes would
push the whole context over NFS on every change, INCREASING writes to the
filesystem this change exists to protect.

### 8. Generations are atomic, retained, and honestly labelled

Write a new generation directory, then swap a `current` symlink by rename. A
rename is atomic, so a reader never sees a half-written context.

Each generation records whether it was `quiesced` — taken at a work boundary,
with no run in flight — or `bestEffort`, taken mid-run and possibly holding a
torn tail. The proxy knows which, because it handed out `/work` and has not seen
`/work/done`.

Mid-run dumps are NOT skipped. A long run is exactly the case a crash would
otherwise lose entirely, and retaining N generations means a torn newest one
costs a fallback rather than the context.

### 9. Telemetry is one call; durable state is one field

The event stream is declared-lossy telemetry and belongs in `internal/activity`,
where the console Sequence tab and Prometheus both already read from a single
emission.

The FACT of the latest successful checkpoint is not telemetry — it decides
whether continuity is possible after a crash — so it lives on the CR as
`status.contextCheckpoint`.

It is written ONLY when a dump actually happened. Writing on a skip would patch
every conversation every interval forever, which is the write storm the cooldown
rules already forbid for the same reason.

### 10. Context loss gets a verb, and the rule keeps its teeth

`A context that cannot be continued fails the run` stays exactly as it is. What
is missing today is a way OUT: a conversation whose context is genuinely gone
can currently only fail forever or be deleted.

The reset is explicit and operator-chosen, clears the handle, and the
conversation continues while SAYING it lost its memory. Nothing about it is
automatic, because an automatic version would be the silent degradation the rule
was written to stop.

Paired with it: when the volume is known-bad, the pod is created WITHOUT the
home mount rather than not at all. A broken filesystem then costs continuity
instead of availability, which is the whole thesis of this change.

## Risks / Trade-offs

- **The sidecar is on the dispatch critical path.** A bug there stops work for
  that conversation. Mitigated by keeping it a forwarding proxy with no parsing
  beyond the two paths it acts on, and by the existing pod-failure reap.
- **The subPath migration can lose context if botched.** It is the one
  irreversible step. It needs an explicit migration, a CHANGELOG entry, and a
  dry-run.
- **A wrong `paths` value silently persists nothing**, and looks fine until a
  resume fails. Validation on the `AgentRuntime` Ready condition, and the
  `context.checkpoint` event carrying bytes, are what make it visible early.
- **Interval sets the crash loss window.** SIGKILL loses everything since the
  last dump, and no design removes that — only shortens it.
- **More moving parts in the runtime pod**: an emptyDir that can fill (hence
  `sizeLimit`), a longer termination grace, and a second container's resources.
- **The drain rule shrinks the corruption window without closing it.** Longhorn
  chooses the share-manager's node independently of where runtime pods run.
  Claiming otherwise in the docs would be worse than the gap.

## Migration Plan

1. Ship the containment half (A) first. It is independent, needs no CRD change,
   and would have turned a fifteen-hour outage into a visible pause.
2. Ship `contextSync` OFF. Absent means today's behavior, so nothing changes for
   an existing install.
3. Migrate home layout on opt-in: move existing context under per-conversation
   directories, dry-runnable, with the CHANGELOG naming the exact steps.
4. Enable it for the reference runtime in the chart, with the shipped `paths`.
5. Only then make the emptyDir switch, so the durable copy exists before the
   live one stops being durable.

Rollback: clearing `contextSync` restores the direct mount. Anything dumped
under `current` is a plain directory tree and readable without the sidecar.

## Open Questions

- The exact `exclude` set for claude-code. Where sessions live is known
  (`runtime.js:302`); what else churns beside them needs an empirical pass
  against a live runtime before the chart ships a value.
- Whether the read-only filesystem check belongs in `housekeeping/` (already a
  CronJob, already mounts the claim root) or in the sidecar. Housekeeping looks
  right, but it reaches the volume over the same NFS path and so may fail in the
  same way rather than reporting on it.
- Whether `retain` should be a count or an age. A count is simpler; an age
  survives a burst of dumps better.
- Whether the canary probe should be a real conversation or a synthetic
  attachment test. A synthetic one is cleaner but adds a code path that only
  runs during incidents, which is when untested code is least welcome.
