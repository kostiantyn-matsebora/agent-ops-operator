# runtime-context-sync Specification

## Purpose

The sidecar that keeps a conversation's LIVE context on pod-local storage and a
SNAPSHOT on the durable volume — opt-in per `AgentRuntime`, and absent by default,
where the pod is exactly what it was before.

It learns work boundaries by PROXYING the work contract, which is what lets it
checkpoint without any runtime image changing. Two orderings are guarantees
rather than details: restore completes before the first `/work` is answered, and
checkpoint completes before `/work/done` reaches the manager — a handle whose
bytes were never written names something gone.

Checkpoints are conditional AND incremental, because a conditional-but-full copy
every two minutes would push the whole context over NFS on every change.

## Requirements

### Requirement: The live context is local, and the durable copy is separate

A runtime's context SHALL be able to live on pod-local ephemeral storage while a
durable copy is maintained on a persistent volume by a separate process.

The agent container SHALL NOT mount the persistent context volume when this mode
is in use. Only the synchronising process SHALL hold it. An agent therefore
cannot read another conversation's context and cannot write to durable storage
at all, which is a property worth having independently of the fault this
addresses.

The persistent volume SHALL be addressed per conversation, so that concurrent
runtime pods never read or write one another's context. A single shared
directory SHALL NOT be used, because concurrent copy-out from several pods would
have each erase the others' writes.

#### Scenario: The agent cannot reach durable storage

- **WHEN** a runtime pod runs with context synchronisation enabled
- **THEN** the agent container has only ephemeral storage mounted, and the persistent volume is mounted solely by the synchronising process

#### Scenario: Concurrent conversations do not collide

- **WHEN** several conversations run at once with context synchronisation enabled
- **THEN** each reads and writes only its own durable context

### Requirement: Context is restored before work and checkpointed after it

The synchronising process SHALL restore a conversation's durable context to the
local store BEFORE the runtime is given its first unit of work, and SHALL
checkpoint the local store to durable storage when a unit of work completes.

A checkpoint SHALL be completed BEFORE the completion of that work unit is
reported to the manager. The manager records the runtime's context handle from
that report, so reporting first would allow a recorded handle whose context was
never persisted — and the next run would then fail a continuation that should
have succeeded.

This SHALL be achieved without requiring changes to any runtime image. A runtime
already implements the work contract, and observing that contract is sufficient
to know where the boundaries are.

#### Scenario: A run starts from its durable context

- **WHEN** a runtime pod starts for a conversation with a stored context
- **THEN** that context is present locally before the runtime receives any work

#### Scenario: The recorded handle always has its bytes

- **WHEN** a work unit completes and the runtime reports its context handle
- **THEN** the context was checkpointed to durable storage before that report reached the manager

#### Scenario: An unmodified runtime image is synchronised

- **WHEN** a runtime image that knows nothing about context synchronisation runs in this mode
- **THEN** its context is still restored and checkpointed

### Requirement: Periodic dumps are configurable, conditional and incremental

The synchronising process SHALL also checkpoint on a configurable period, and
the period SHALL be settable to a value meaning "work boundaries only".

A periodic checkpoint SHALL be SKIPPED entirely when the local context has not
changed since the last one. Change detection SHALL be based on a scan of the
local store, which is ephemeral local storage and therefore cheap to walk. A
filesystem-notification mechanism MAY accelerate this, but SHALL NOT be the sole
source of truth, because such mechanisms have watch limits and can drop events.

A checkpoint that does occur SHALL copy only what changed, relative to the
previous durable copy. A full copy on every change SHALL NOT be used: it would
increase writes to the durable filesystem, which is the failure this capability
exists to reduce.

A periodic checkpoint SHALL be suppressed when a checkpoint has just occurred,
so that a work-boundary checkpoint and a periodic one do not run back to back.

#### Scenario: Nothing changed, nothing is written

- **WHEN** the periodic interval elapses and the local context is unchanged
- **THEN** no data is written to durable storage

#### Scenario: Only changed data is transferred

- **WHEN** one file in a large context changes and a checkpoint runs
- **THEN** only that file is transferred, and unchanged files are not rewritten

#### Scenario: Work boundaries alone can be chosen

- **WHEN** the interval is configured to mean work boundaries only
- **THEN** no periodic checkpoints occur and context is still checkpointed as work units complete

### Requirement: Durable context is atomic, generational and honestly labelled

A checkpoint SHALL be made visible atomically, so that a reader never observes a
partially written context.

A configurable number of previous copies SHALL be retained, so that a copy which
turns out to be unusable costs a fallback rather than the context.

Each copy SHALL record whether it was taken at a work boundary with nothing
inflight, or during a run. A copy taken during a run may contain a partially
written file, and labelling it is what allows an operator and a restore path to
tell the two apart.

A checkpoint during a run SHALL NOT be skipped on the grounds that it may be
inconsistent. A long-running unit is precisely the case that a crash would
otherwise lose in full.

#### Scenario: A reader never sees a half-written context

- **WHEN** a checkpoint is interrupted before it completes
- **THEN** the previously visible durable context remains the one a restore would use

#### Scenario: Copies say how trustworthy they are

- **WHEN** a checkpoint is taken while a run is inflight
- **THEN** it is retained and labelled as taken mid-run rather than at a work boundary

### Requirement: The runtime declares what its context is

An `AgentRuntime` SHALL declare which paths constitute its context, as an
INCLUDE list relative to the runtime's home, together with any churn to exclude
within them, the checkpoint period, and how many copies to retain.

The declaration SHALL belong to the runtime rather than to the chart or the
manager, for the reason context storage already belongs there: neither can know
where a given agent backend keeps its context, and guessing produces a
configuration that appears to work until a resume fails.

An include list SHALL be used rather than an exclude list, so that caches and
tool state are excluded by construction rather than by a list that must chase
every file a vendor adds.

When the declaration is ABSENT, the runtime SHALL behave exactly as it does
without this capability. When PRESENT, the paths SHALL be validated and reported
on the runtime's readiness condition.

**A runtime whose context location is known SHALL declare it, and the BUNDLE
shipping that runtime SHALL render the declaration** — beside that runtime's
image and its model credential, which belong there for the same reason. An
include list is one vendor's filesystem layout, so the release-wide runtime
defaults every backend inherits are the wrong place for it. Requiring an operator to
type a fact the runtime already holds leaves the mechanism inert on the install
best able to use it, and leaves the durable context volume mounted into every
agent container until somebody notices. The declaration belonging to the runtime
means the runtime states it — not that a human states it on the runtime's
behalf.

A runtime whose context location is NOT known — any backend the project does not
ship — SHALL still declare its own paths, and SHALL get the unsynchronised pod
until it does. Nothing infers a path, and an empty include list remains invalid
rather than being read as a request to copy everything.

#### Scenario: An unconfigured runtime is unchanged

- **WHEN** an AgentRuntime declares no context synchronisation
- **THEN** it runs exactly as before, with its context volume mounted directly and no synchronising process

#### Scenario: The shipped runtime is installed with default values

- **WHEN** an install accepts the chart's defaults and a context volume exists
- **THEN** the reference runtime's context paths are already declared, in that runtime's own bundle values
- **AND** synchronisation is active without an operator having set a value

#### Scenario: Another backend inherits no vendor's paths

- **WHEN** an install disables the shipped runtime bundle and declares its own runtime
- **THEN** it inherits no context paths, because they were never in the release-wide runtime defaults

#### Scenario: A third-party runtime declares nothing

- **WHEN** a runtime the project does not ship declares no context paths
- **THEN** it gets the unsynchronised pod, and no path is guessed on its behalf

#### Scenario: An empty include list is written by hand

- **WHEN** an AgentRuntime declares context synchronisation with an empty path list
- **THEN** it is rejected as a declaration that would persist nothing while appearing configured
- **AND** it is not interpreted as a request to synchronise the whole context tree

#### Scenario: A misdeclared context is reported

- **WHEN** an AgentRuntime enables context synchronisation with paths that cannot be valid
- **THEN** the runtime reports the problem on its readiness condition rather than silently persisting nothing

### Requirement: Synchronisation requires a durable volume, and falls back without one

Context synchronisation SHALL be built only where a durable context volume
exists to snapshot to. Where no durable volume is configured, the runtime pod
SHALL be exactly the unsynchronised pod: no synchronising process, no durable
mount, and no promise of continuity.

A pod SHALL NOT be constructed that references a durable context volume by an
empty name. Doing so fails provisioning outright, which is worse than the
configuration the operator chose: an install that deliberately runs without
persistence is asking for ephemeral context, not for a conversation that cannot
start.

This SHALL agree with how continuity is resolved. Continuity resolution already
answers that no durable volume means no promise, so a pod builder that fails
instead of falling back makes the manager promise a fresh answer and then fail
to provide one. **The two SHALL NOT disagree**, and where they do the reply is a
message the reader can act on while the run produces nothing.

The condition is the ABSENT VOLUME, not the absent promise. Continuity is also
impossible for a backend that keeps no context on disk, and such a runtime SHALL
still mount whatever durable volume it is given — that mount is where the pod's
filesystem lives, not a promise about what survives the pod. Reading every
impossible promise as a demand for ephemeral storage would take a configured
volume away from the one case that never asked for continuity.

The same rule already governs a missing synchronising image, which falls back to
the unsynchronised pod. A missing durable volume SHALL be treated identically —
one fallback rule, two conditions, not one condition with an exception.

#### Scenario: Persistence is disabled and a runtime declares synchronisation

- **WHEN** a runtime declares context paths and no durable context volume is configured
- **THEN** the pod is built with ephemeral context, no synchronising process and no durable mount
- **AND** the conversation is told its context is not promised, rather than failing to start

#### Scenario: A pod is built without a durable claim

- **WHEN** any runtime pod is constructed while no durable context volume is configured
- **THEN** no volume in that pod references a persistent claim by an empty name

#### Scenario: The promise and the pod agree

- **WHEN** continuity resolution reports that continuity is not possible because no durable context volume is configured
- **THEN** the pod that is built for that conversation starts and runs with ephemeral context

#### Scenario: A backend that keeps no context on disk is given a volume

- **WHEN** a runtime declares that it holds no context on disk and a durable context volume is configured
- **THEN** continuity is still not promised, and the pod still mounts the configured volume

#### Scenario: The synchronising image is missing

- **WHEN** a runtime declares context paths and no synchronising image is configured
- **THEN** the pod falls back to the unsynchronised pod, by the same rule that governs a missing durable volume

### Requirement: Context operations survive every ordinary end of a pod

A final checkpoint SHALL be taken when a runtime pod ends by any means the
manager controls: an idle timeout, an operator-issued release, an eviction for
capacity, a conversation being closed, and a release ahead of a node draining.

A conversation that is closed and later reopened SHALL find its context, which
requires that closing checkpoint it.

Termination SHALL allow enough time for that final checkpoint to complete.

Loss of the work done since the last checkpoint SHALL be accepted only where the
pod is killed without warning. The configured period bounds that loss and no
mechanism removes it.

#### Scenario: Closing a conversation preserves its context

- **WHEN** a conversation is closed and its runtime pod torn down
- **THEN** its context was checkpointed, and reopening it later continues from that context

#### Scenario: An idle timeout checkpoints on the way out

- **WHEN** a runtime pod exits because it has been idle
- **THEN** its context was checkpointed before the pod ended
