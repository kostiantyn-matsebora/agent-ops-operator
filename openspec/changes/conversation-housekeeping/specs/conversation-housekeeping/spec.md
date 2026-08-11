## ADDED Requirements

### Requirement: Autoclose is a flag and a window, disabled by default
The manager SHALL support closing finished conversations automatically, governed
by exactly two settings: an enable flag and an idle window. The flag SHALL
default to disabled, preserving today's behaviour in which a conversation lives
until closed by hand.

There SHALL NOT be a mode that closes a conversation as soon as its reply is
delivered. The `Conversation` is the only durable record of a result — the run
result lives in its status and nowhere else in the API — so closing at the
instant of delivery destroys what the operator is trying to read, and for a
conversation bound to no channel destroys it with no transport copy anywhere. An
install wanting aggressive reclamation SHALL configure a short window instead,
which leaves an interval in which the result is readable.

Autoclose SHALL NOT require any grant the manager does not already hold, and
SHALL NOT require the manager to mount any volume.

#### Scenario: Default install closes nothing
- **WHEN** an operator installs the chart without enabling autoclose
- **THEN** finished conversations live indefinitely, exactly as before

#### Scenario: The window is respected
- **WHEN** autoclose is enabled with a window of seven days and a finished conversation has been idle for three days
- **THEN** it is left alone, and it is closed once it passes the window

#### Scenario: A short window still leaves the result readable
- **WHEN** an operator wants a noisy observing lane reclaimed aggressively
- **THEN** the shortest available configuration is a small window, and the conversation and its recorded result remain readable for that window after the answer

### Requirement: The window is idle time, not lifetime
The window SHALL be measured from the conversation's last activity — its most
recent run or input — and SHALL NOT be measured from its creation timestamp. A
long-lived conversation that was active recently SHALL survive, regardless of how
long ago it was created.

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
these missing means the conversation is live work, and autoclose SHALL leave it
alone regardless of idle time.

The delivery clause is required for correctness, not caution: a conversation
reaches `Idle` the moment a run's result is recorded, while the reply may still
be an unclaimed outbound operation. Closing on `Idle` alone would archive the
thread before the answer reached it, reintroducing by configuration the loss that
per-thread delivery markers exist to prevent. A long window makes that unlikely,
not impossible — an adapter down for the length of the window is exactly the case
it happens in.

A run that is never delivered SHALL therefore hold its conversation open rather
than release it: retaining is the safe direction, and the outstanding operation
is visible in the queue statistics.

#### Scenario: Working conversations survive their window
- **WHEN** a conversation idle past the window has a run inflight
- **THEN** it is not closed, and it becomes eligible only once the run completes and its pod exits

#### Scenario: Queued input defers the close
- **WHEN** a conversation past its window has a pending input awaiting dispatch
- **THEN** it is held open until that input has been processed

#### Scenario: An undelivered reply holds the conversation open
- **WHEN** a conversation is past its window and one bound channel's adapter is down, so its thread is never marked delivered
- **THEN** the conversation is held open rather than closed, and the outstanding operation remains visible in the queue statistics

#### Scenario: The answer lands before the conversation goes
- **WHEN** a conversation bound to two channels becomes eligible
- **THEN** it is closed only after both bound threads are marked delivered

### Requirement: An automatic close announces itself and says why
Autoclose SHALL close conversations through the same path as `/close` — posting a
farewell to every bound thread, then deleting the object — and SHALL NOT issue a
bare deletion. The farewell SHALL state that the conversation was closed
automatically and name the idle window that elapsed, so a person reading the
thread can find the setting responsible.

A closed thread must read as closed. Archiving one with no message is
indistinguishable from a fault, and the person in it did not ask for the close.

Owner references SHALL therefore reclaim the conversation's inputs and its
compiled MCP ConfigMap, and the close-topics finalizer SHALL archive its bound
threads before the object disappears. Autoclose SHALL NOT introduce a second
close or deletion path.

#### Scenario: The thread is told
- **WHEN** autoclose closes a conversation bound to a chat surface
- **THEN** that thread receives a farewell naming the automatic close and the elapsed window, before the topic is archived

#### Scenario: Dependents go with the owner
- **WHEN** autoclose closes a conversation that owns input objects and an MCP ConfigMap
- **THEN** those objects are garbage-collected by owner reference, with no explicit deletion of each

#### Scenario: Threads are archived, not abandoned
- **WHEN** autoclose closes a conversation bound to a chat surface
- **THEN** its thread receives a close-topic operation before the object is removed

#### Scenario: One close implementation
- **WHEN** a conversation is closed by `/close`, by a console batch, or by autoclose
- **THEN** the farewell, the finalizer, the owner-reference teardown and the capacity release are the same in every case

### Requirement: Volume reclamation runs outside the manager
Reclaiming storage SHALL be performed by a workload separate from the manager,
because it requires mounting the claims at their ROOT and the manager mounts no
PersistentVolume.

That workload SHALL run no agent code, SHALL use its own ServiceAccount, and
SHALL hold read-only access to conversations — it performs no API writes, since
closing conversations belongs to the manager. The chart SHALL fail to render if
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
a dry-run mode that reports what it would remove and removes nothing. Autoclose's
first pass after enabling SHALL spread its closes rather than expiring every
eligible conversation simultaneously.

The first run on an established install is the dangerous one: unbounded, it can
archive hundreds of chat topics at once, which is alarming even when correct.

#### Scenario: Dry run changes nothing
- **WHEN** the reclaiming workload runs with dry run enabled and finds orphans
- **THEN** it reports each one and deletes none

#### Scenario: A large backlog is worked down across runs
- **WHEN** more orphans exist than the per-run bound allows
- **THEN** the run deletes up to the bound and reports how many remain, and the next run continues

#### Scenario: Enabling autoclose does not archive everything at once
- **WHEN** autoclose is enabled on an install whose conversations have all been idle longer than the window
- **THEN** the closes are spread rather than issued simultaneously
