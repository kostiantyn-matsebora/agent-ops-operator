## MODIFIED Requirements

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

**A runtime whose context location is known SHALL declare it, and the chart
shipping that runtime SHALL render the declaration.** Requiring an operator to
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
- **THEN** the reference runtime's context paths are already declared
- **AND** synchronisation is active without an operator having set a value

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

## ADDED Requirements

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

- **WHEN** continuity resolution reports that continuity is not possible
- **THEN** the pod that is built for that conversation starts and runs with ephemeral context

#### Scenario: The synchronising image is missing

- **WHEN** a runtime declares context paths and no synchronising image is configured
- **THEN** the pod falls back to the unsynchronised pod, by the same rule that governs a missing durable volume
