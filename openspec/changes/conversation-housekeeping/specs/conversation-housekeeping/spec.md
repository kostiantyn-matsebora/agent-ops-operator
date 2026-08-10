## ADDED Requirements

### Requirement: Conversation retention is an explicit mode, off by default
The manager SHALL support three retention modes for `Conversation` objects:
`off`, `age` and `immediate`. `off` SHALL be the default and SHALL preserve
today's behaviour, in which a conversation lives until closed by hand.

Retention SHALL be expressed as a mode rather than as a duration alone: a bare
window cannot distinguish "delete immediately" from "never delete", and the two
are the most and least aggressive settings available.

Retention SHALL NOT require any grant the manager does not already hold, and
SHALL NOT require the manager to mount any volume.

#### Scenario: Default install deletes nothing
- **WHEN** an operator installs the chart without setting retention
- **THEN** finished conversations are retained indefinitely, exactly as before

#### Scenario: Age mode deletes only past the window
- **WHEN** retention is `age` with a window of seven days and a finished conversation is three days old
- **THEN** it is retained, and it is deleted once it passes the window

### Requirement: Only a finished conversation may be deleted by retention
A conversation SHALL be eligible for retention only when ALL of the following
hold: phase is `Idle`, no inputs are pending, no run is inflight, and no runtime
pod exists for it. Any one of these missing means the conversation is live work,
and retention SHALL leave it alone regardless of age.

#### Scenario: Working conversations survive their window
- **WHEN** a conversation older than the retention window has a run inflight
- **THEN** it is not deleted, and it becomes eligible only once the run completes and its pod exits

#### Scenario: Queued input defers deletion
- **WHEN** a conversation past its window has a pending input awaiting dispatch
- **THEN** it is retained until that input has been processed

### Requirement: Immediate retention waits for the reply to be delivered
In `immediate` mode a conversation SHALL be deleted as soon as it is finished
AND every recorded run is marked delivered to every bound channel.

The delivery clause is required for correctness, not caution: a conversation
reaches `Idle` the moment a run's result is recorded, while the reply may still
be an unclaimed outbound operation. Deleting on `Idle` alone would archive the
thread before the answer reached it, reintroducing by configuration the loss that
per-thread delivery markers exist to prevent.

A run that is never delivered SHALL therefore retain its conversation rather than
release it — retaining is the safe direction, and `age` remains available as a
ceiling for installs that want one regardless.

#### Scenario: The answer lands before the conversation goes
- **WHEN** retention is `immediate` and a run completes on a conversation bound to two channels
- **THEN** the conversation is deleted only after both bound threads are marked delivered

#### Scenario: An undelivered reply holds the conversation open
- **WHEN** retention is `immediate` and one bound channel's adapter is down, so its thread is never marked delivered
- **THEN** the conversation is retained rather than deleted, and the outstanding operation remains visible in the queue statistics

#### Scenario: Immediate mode makes threads reply-dead
- **WHEN** retention is `immediate` and a person replies in the thread after the agent's answer
- **THEN** the reply reaches no conversation, because the thread was archived with it — the behaviour is documented at the setting, and the setting is off by default

### Requirement: Deleting a conversation reclaims its dependents and archives its threads
Retention SHALL delete conversations through the same path as `/close`, so that
owner references reclaim the conversation's inputs and its compiled MCP
ConfigMap, and the close-topics finalizer archives its bound threads before the
object disappears. Retention SHALL NOT introduce a second deletion path.

#### Scenario: Dependents go with the owner
- **WHEN** retention deletes a conversation that owns input objects and an MCP ConfigMap
- **THEN** those objects are garbage-collected by owner reference, with no explicit deletion of each

#### Scenario: Threads are archived, not abandoned
- **WHEN** retention deletes a conversation bound to a chat surface
- **THEN** its thread receives a close-topic operation before the object is removed

### Requirement: Volume reclamation runs outside the manager
Reclaiming storage SHALL be performed by a workload separate from the manager,
because it requires mounting the claims at their ROOT and the manager mounts no
PersistentVolume.

That workload SHALL run no agent code, SHALL use its own ServiceAccount, and
SHALL hold read-only access to conversations — it performs no API writes, since
deleting conversations belongs to retention. The chart SHALL fail to render if
this identity is configured to be the runtime ServiceAccount.

#### Scenario: The claim root is not handed to agents
- **WHEN** an operator configures the reclaiming workload to run as the runtime ServiceAccount
- **THEN** the render fails, because that identity executes agent code and per-conversation isolation exists to keep the claim root away from it

#### Scenario: Reclamation needs no write access
- **WHEN** the reclaiming workload runs
- **THEN** it lists conversations and writes nothing through the Kubernetes API

### Requirement: An orphan directory is identified by ordering, not by a timeout
Workspace directories SHALL be reclaimed only when no `Conversation` of that name
exists, determined by scanning directory entries BEFORE listing conversations and
deleting only entries absent from the later listing.

This ordering is what makes the decision sound: a directory is created by the
kubelet mounting a subPath for a runtime pod, and that pod exists only for a
conversation created earlier, so a conversation always predates its directory. A
listing taken after the scan therefore contains every conversation whose
directory was seen, and an absence is a deletion rather than a race.

The reverse order SHALL NOT be used, because a conversation created between the
listing and the scan would appear to be an orphan.

#### Scenario: A conversation created mid-run is not reclaimed
- **WHEN** a conversation is created after the directory scan begins and its directory appears before the run ends
- **THEN** the conversation appears in the listing taken after the scan and its directory is left alone

#### Scenario: A deleted conversation's directory is reclaimed
- **WHEN** a directory exists for a conversation that no longer exists
- **THEN** the directory and its contents are removed

### Requirement: Session transcripts are reclaimed by reference and age together
Transcripts on the home volume SHALL be reclaimed only when their session id
appears in no conversation's recorded session AND the file is older than a grace
period that exceeds the longest plausible run.

Both conditions are required because the ordering argument that governs
directories runs backwards here: a session id is recorded on the conversation
only after the run that created the file completes, so a transcript can
legitimately exist with no reference yet.

#### Scenario: A transcript from a run in flight is kept
- **WHEN** a transcript exists whose session id has not yet been recorded on any conversation and whose file is newer than the grace period
- **THEN** it is retained

#### Scenario: An unreferenced old transcript is reclaimed
- **WHEN** a transcript's session id appears on no conversation and the file is older than the grace period
- **THEN** it is removed

### Requirement: Every run is bounded and can be rehearsed
Each reclamation run SHALL accept a maximum number of deletions and SHALL support
a dry-run mode that reports what it would remove and removes nothing. Retention's
first pass after enabling SHALL spread its deletions rather than expiring every
eligible conversation simultaneously.

The first run on an established install is the dangerous one: unbounded, it can
archive hundreds of chat topics at once, which is alarming even when correct.

#### Scenario: Dry run changes nothing
- **WHEN** the reclaiming workload runs with dry run enabled and finds orphans
- **THEN** it reports each one and deletes none

#### Scenario: A large backlog is worked down across runs
- **WHEN** more orphans exist than the per-run bound allows
- **THEN** the run deletes up to the bound and reports how many remain, and the next run continues

#### Scenario: Enabling retention does not archive everything at once
- **WHEN** retention is enabled on an install whose conversations are all older than the window
- **THEN** deletions are spread rather than issued simultaneously
