## ADDED Requirements

### Requirement: A conversation's end has two stages, each with its own flag and window
Ending a conversation SHALL be two distinct acts. **Closing** makes a conversation
inert while leaving it intact and reopenable. **Deleting** reclaims it and its
state for good. Each SHALL be governed by its own enable flag and its own
duration, and each flag SHALL default to disabled.

The two durations SHALL be measured from different origins and SHALL be named for
what each measures: the close window from the conversation's last activity, the
delete window from the moment it was closed.

Enabling automatic closing SHALL NOT enable automatic deletion. A lane that tidies
itself while keeping its record is the expected configuration, and it SHALL NOT
require declining the destructive stage by leaving a duration unset.

#### Scenario: Default install ends nothing automatically
- **WHEN** an operator installs the chart without enabling either flag
- **THEN** conversations are closed only by hand and deleted only by hand, and nothing is reclaimed that was not reclaimed before

#### Scenario: Closing automatically does not delete automatically
- **WHEN** automatic closing is enabled and automatic deletion is not
- **THEN** finished conversations become closed and remain readable and reopenable indefinitely

#### Scenario: The two clocks are independent
- **WHEN** a conversation is closed automatically after its idle window and the delete window is longer
- **THEN** it is deleted only once it has been CLOSED for the delete window, not once it has been idle for it

### Requirement: The close window is idle time, not lifetime
The close window SHALL be measured from the conversation's last activity — its
most recent run or input — and SHALL NOT be measured from its creation timestamp.
A long-lived conversation that was active recently SHALL survive, regardless of
how long ago it was created.

#### Scenario: Recent activity resets the window
- **WHEN** a conversation created ten days ago received a reply one hour ago and the window is seven days
- **THEN** it is not closed, because it has been idle for one hour

#### Scenario: An idle conversation closes regardless of age
- **WHEN** a conversation has been idle for longer than the window
- **THEN** it is closed, whether it was created a day or a year ago

### Requirement: Only a finished conversation may be closed automatically
A conversation SHALL be eligible only when ALL of the following hold: phase is
`Idle`, no inputs are pending, no run is inflight, no runtime pod exists for it,
and every recorded run is marked delivered to every bound channel. Any one of
these missing means the conversation is live work, and automatic closing SHALL
leave it alone regardless of idle time.

The delivery clause is required for correctness, not caution: a conversation
reaches `Idle` the moment a run's result is recorded, while the reply may still be
an unclaimed outbound operation. Closing on `Idle` alone would archive the thread
before the answer reached it, reintroducing by configuration the loss that
per-thread delivery markers exist to prevent. A long window makes that unlikely,
not impossible — an adapter down for the length of the window is exactly the case
it happens in.

A run that is never delivered SHALL therefore hold its conversation open rather
than release it: retaining is the safe direction, and the outstanding operation is
visible in the queue statistics.

This clause constrains CLOSING and not deleting, because it is about a message
reaching a thread and threads are archived at the close.

#### Scenario: Working conversations survive their window
- **WHEN** a conversation idle past the window has a run inflight
- **THEN** it is not closed, and it becomes eligible only once the run completes and its pod exits

#### Scenario: Queued input defers the close
- **WHEN** a conversation past its window has a pending input awaiting dispatch
- **THEN** it is held open until that input has been processed

#### Scenario: An undelivered reply holds the conversation open
- **WHEN** a conversation is past its window and one bound channel's adapter is down, so its thread is never marked delivered
- **THEN** the conversation is held open rather than closed, and the outstanding operation remains visible in the queue statistics

#### Scenario: The answer lands before the conversation closes
- **WHEN** a conversation bound to two channels becomes eligible
- **THEN** it is closed only after both bound threads are marked delivered

### Requirement: A closed conversation is inert but intact
A closed conversation SHALL have no runtime pod and no compiled MCP ConfigMap,
SHALL receive no dispatch, and SHALL consume neither active capacity nor a place
in the pending backlog.

It SHALL take part in no pipeline and SHALL be excluded from conversation REUSE: a
signal whose signature matches a closed conversation SHALL open a new conversation
rather than waking the closed one. A closed conversation is not somewhere work can
land.

Everything that makes it a record SHALL be retained: its spec, its materialized
profile, channel, toolset and MCP references, its runtime context handle, its
recorded runs and their results, and its state on any volume. The moment of
closing SHALL be recorded on the conversation, because it is the origin of the
delete window.

#### Scenario: A closed conversation runs nothing
- **WHEN** a conversation is closed
- **THEN** its runtime pod and MCP ConfigMap are gone, no work unit is dispatched for it, and the capacity it held admits a waiting conversation

#### Scenario: A matching signal does not wake a closed conversation
- **WHEN** a signal arrives whose signature matches a closed conversation inside the reuse window
- **THEN** a new conversation is opened and the closed one is untouched

#### Scenario: The record survives the close
- **WHEN** an operator views a closed conversation
- **THEN** its recorded runs, their results and its materialized wiring are all still readable

#### Scenario: Volume state survives the close
- **WHEN** a conversation with a persisted workspace is closed
- **THEN** its workspace directory and its session transcripts remain on their claims

### Requirement: A closed conversation can be reopened to Idle
A closed conversation SHALL be reopenable, returning it to phase `Idle` with its
materialized references unchanged. Reopening SHALL NOT re-resolve wiring: the same
profile, channels, toolsets and MCP configurations that the conversation carried
SHALL be the ones it carries afterwards, because those references are snapshots
and re-resolving them would let an unrelated Pipeline edit re-wire an existing
conversation.

Continuity SHALL be restored where it was promised and not otherwise: where the
runtime's context storage keeps a conversation's context across runs, a reopened
conversation resumes with it; where it does not, the reopened conversation answers
fresh and says so, exactly as a resume does.

A reopen SHALL fail, naming the missing reference, when a referenced profile or
channel no longer exists. It SHALL NOT partially reopen and SHALL NOT silently
drop a binding.

#### Scenario: Reopening restores the conversation, not a copy of it
- **WHEN** a closed conversation is reopened
- **THEN** it is `Idle` with the same profile, channels, toolsets and MCP configurations it had, and its recorded runs are still present

#### Scenario: Reopening resumes where continuity was promised
- **WHEN** a conversation whose runtime keeps context on a volume is reopened and given an input
- **THEN** the run continues from the conversation's retained context and workspace

#### Scenario: Reopening is honest where continuity was never promised
- **WHEN** a conversation whose runtime keeps no context across runs is reopened
- **THEN** it answers fresh and reports that it is doing so

#### Scenario: A reopen with missing wiring fails loudly
- **WHEN** a closed conversation is reopened after its profile has been deleted
- **THEN** the reopen fails naming the missing profile, and the conversation stays closed

### Requirement: Reopening asks each surface for somewhere to post, and the adapter decides
Reopening SHALL re-establish a thread on every bound channel through the ordinary
ensure-topic operation, carrying the conversation's archived thread identifier as
an optional hint. The adapter SHALL be free to honour the hint — returning the same
thread, so the conversation continues where it left off — or to ignore it and
return a new thread. Both SHALL be valid, and the conversation's recorded threads
SHALL be updated from what the adapter returns.

The manager SHALL NOT decide which is possible. Whether a transport can un-archive
is transport knowledge, and the manager holds none.

There SHALL NOT be a separate reopen operation kind. Most transports cannot
un-archive, so most implementations of one would do exactly what ensure-topic does,
and an adapter that never implements the hint SHALL already be correct.

#### Scenario: A transport that can un-archive continues the thread
- **WHEN** a conversation is reopened on a channel whose adapter can un-archive its topic
- **THEN** the adapter returns the same thread identifier and the conversation continues in it

#### Scenario: A transport that cannot un-archive opens a fresh thread
- **WHEN** a conversation is reopened on a channel whose transport has no un-archive
- **THEN** the adapter ignores the hint, returns a new thread identifier, and the conversation records it

#### Scenario: Mixed outcomes across channels are recorded honestly
- **WHEN** a conversation bound to two channels is reopened and only one adapter honours the hint
- **THEN** one thread continues and the other is new, and both are recorded

### Requirement: An automatic close announces itself and says it can be undone
Automatic closing SHALL take the same path as a hand-typed close — posting a
farewell to every bound thread and archiving those threads — and SHALL NOT bypass
it. The farewell SHALL state that the conversation was closed automatically, name
the idle window that elapsed so a person can find the setting responsible, and say
that the conversation can be reopened.

A closed thread must read as closed. Archiving one with no message is
indistinguishable from a fault, and the person in it did not ask for the close.
Saying it can be reopened is what distinguishes a pause from an ending, and a
farewell that omits it under-sells the only thing that makes automatic closing
safe to enable.

Automatic closing SHALL NOT introduce a second close implementation.

#### Scenario: The thread is told, and told it is not final
- **WHEN** automatic closing closes a conversation bound to a chat surface
- **THEN** that thread receives a farewell naming the automatic close, the elapsed window, and that the conversation can be reopened, before the topic is archived

#### Scenario: One close implementation
- **WHEN** a conversation is closed by a typed command, by a console batch, or by the manager's own timer
- **THEN** the farewell, the teardown of pod and ConfigMap, the topic archiving and the capacity release are the same in every case

### Requirement: The delete window is measured from the close, and deletion is refused otherwise
Automatic deletion SHALL apply only to conversations that are already closed, and
SHALL be measured from the moment of closing. A conversation that has never been
closed SHALL NOT be deleted automatically however long it has been idle, and a
deletion requested for a conversation that is not closed SHALL be refused with a
reason naming the missing step rather than closing it first.

Refusing is deliberate. A close-then-delete in one gesture would perform the
irreversible act on a conversation that was still working, behind a confirmation
that named only the deletion. Requiring the close first makes destruction
something the operator ordered twice.

Deleting a conversation SHALL destroy the only durable copy of its results: the
Kubernetes API holds them nowhere else, and metrics keep aggregates only. The
delete window SHALL therefore be documented AT the setting as "how long do I want
to be able to read this", never as "how long until it is tidy".

#### Scenario: A live conversation is not deleted by the timer
- **WHEN** automatic deletion is enabled and a conversation has been idle far longer than the delete window but was never closed
- **THEN** it is not deleted

#### Scenario: The clock starts at the close
- **WHEN** a conversation closed one day ago is subject to a seven-day delete window
- **THEN** it is deleted six days later, regardless of when it was created or last active

#### Scenario: Deleting a live conversation is refused, not escalated
- **WHEN** a deletion is requested for a conversation that is not closed
- **THEN** it is refused with a reason saying the conversation must be closed first, and the conversation is neither closed nor deleted

#### Scenario: Reopening resets the delete clock
- **WHEN** a closed conversation is reopened before its delete window elapses
- **THEN** it is not deleted, and a later close starts the window again

### Requirement: Deletion reclaims the object and the disk through two workloads
Deleting a conversation SHALL remove the object from the Kubernetes API, and its
state on any volume SHALL be reclaimed by the workload that mounts those volumes.
The two SHALL NOT coordinate: the object goes first, its state becomes an orphan,
and the reclaiming workload removes it on its next run.

An install that enables automatic deletion without the reclaiming workload SHALL
reclaim the API half and leave the disk. This SHALL be stated where the setting
is, because it is a correct configuration for an install with no persistence and a
silent leak for one with it.

#### Scenario: The disk follows the object
- **WHEN** a closed conversation is deleted and the reclaiming workload runs afterwards
- **THEN** its workspace directory and session transcripts are removed

#### Scenario: No handshake is required
- **WHEN** a conversation is deleted while the reclaiming workload is not installed
- **THEN** the deletion succeeds and its volume state remains until a reclaiming run happens

### Requirement: A closed conversation's state is live state
The reclaiming workload SHALL identify what to remove by the absence of a
`Conversation` object and SHALL NOT consider a conversation's phase. A closed
conversation has an object, so its workspace directory and its transcripts SHALL
be retained by the ordinary rule with no special case.

The listing SHALL remain phase-blind. Narrowing it to active conversations — to
"only look at live ones" — would reclaim the state of every conversation an
operator was keeping, and it is the optimisation most likely to be proposed.

#### Scenario: A closed conversation's workspace survives a reclaiming run
- **WHEN** a conversation has been closed and the reclaiming workload runs
- **THEN** its workspace directory and its transcripts are left alone

#### Scenario: A reopened conversation still has its workspace
- **WHEN** a conversation is closed, a reclaiming run happens, and the conversation is then reopened
- **THEN** it resumes with the workspace and context it had before the close

### Requirement: Volume reclamation runs outside the manager
Reclaiming storage SHALL be performed by a workload separate from the manager,
because it requires mounting the claims at their ROOT and the manager mounts no
PersistentVolume.

That workload SHALL run no agent code, SHALL use its own ServiceAccount, and SHALL
hold read-only access to conversations — it performs no API writes, since both
stages of a conversation's ending belong to the manager. The chart SHALL fail to
render if this identity is configured to be the runtime ServiceAccount.

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
listing taken after the scan therefore contains every conversation whose directory
was seen, and an absence is a deletion rather than a race.

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
appears in no conversation's recorded context AND the file is older than a grace
period that exceeds the longest plausible run.

Both conditions are required because the ordering argument that governs
directories runs backwards here: a context handle is recorded on the conversation
only after the run that created the file completes, so a transcript can
legitimately exist with no reference yet.

A closed conversation still carries its context handle, so its transcript is still
referenced and still retained by this rule. A sweep keyed on recently active
conversations rather than all conversations SHALL NOT be used.

#### Scenario: A transcript from a run in flight is kept
- **WHEN** a transcript exists whose session id has not yet been recorded on any conversation and whose file is newer than the grace period
- **THEN** it is retained

#### Scenario: An unreferenced old transcript is reclaimed
- **WHEN** a transcript's session id appears on no conversation and the file is older than the grace period
- **THEN** it is removed

#### Scenario: A closed conversation's transcript is still referenced
- **WHEN** a closed conversation's context handle names a transcript older than the grace period
- **THEN** the transcript is retained, because the reference exists

### Requirement: Every run is bounded and can be rehearsed
Each reclamation run SHALL accept a maximum number of deletions and SHALL support a
dry-run mode that reports what it would remove and removes nothing. Each timer's
first pass after being enabled SHALL spread its work rather than acting on every
eligible conversation simultaneously.

The first pass on an established install is the dangerous one. Enabling automatic
closing makes every conversation eligible at once, and each close enqueues a
farewell and a topic archive per bound thread — hundreds of chat topics archiving
simultaneously, which is alarming even when correct. Enabling automatic deletion on
an install that has been closing for a while is the same burst one stage later, and
that one cannot be undone.

#### Scenario: Dry run changes nothing
- **WHEN** the reclaiming workload runs with dry run enabled and finds orphans
- **THEN** it reports each one and deletes none

#### Scenario: A large backlog is worked down across runs
- **WHEN** more orphans exist than the per-run bound allows
- **THEN** the run deletes up to the bound and reports how many remain, and the next run continues

#### Scenario: Enabling a timer does not act on everything at once
- **WHEN** automatic closing is enabled on an install whose conversations have all been idle longer than the window
- **THEN** the closes are spread rather than issued simultaneously

#### Scenario: The irreversible burst is spread too
- **WHEN** automatic deletion is enabled on an install holding many long-closed conversations
- **THEN** the deletions are spread rather than issued simultaneously
