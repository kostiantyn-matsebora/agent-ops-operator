## MODIFIED Requirements

### Requirement: /close ends a conversation from its thread
A message whose text is the command `/close` (optionally bot-suffixed, e.g.
`/close@SomeBot`) sent in a conversation's thread SHALL end that conversation:
the manager SHALL intercept it on the reply path BEFORE it becomes a reply
input, post a farewell message to every bound thread, and move the
`Conversation` to phase `Closed`. The command SHALL be accepted from any sender
able to post in the thread — no additional authorization is required.

Closing SHALL NOT delete the object. A closed conversation is inert but intact:
its `spec`, its materialized refs, its `status.runs[]`, its `runtimeContextId`
and its volume state all survive, which is what makes it reopenable. Deletion is
a SEPARATE verb with its own trigger, window and flag.

A message arriving in a CLOSED conversation's thread SHALL be answered with a
notice saying the conversation is closed and can be reopened, and SHALL create
no input. A closed conversation dispatches nothing, so an appended input would
wait forever on a surface where somebody is expecting an answer.

#### Scenario: Close does not delete the conversation
- **WHEN** a user sends `/close` in a conversation's thread
- **THEN** the `Conversation` object remains at phase `Closed` with `status.closedAt` stamped, and no reply input is appended to it

#### Scenario: Close is not handed to the agent
- **WHEN** a user sends `/close` in a thread
- **THEN** no runtime work unit is dispatched for that text

#### Scenario: Bot-suffixed form works
- **WHEN** a user sends `/close@AgentOpsBot` in a thread
- **THEN** the conversation is closed exactly as for `/close`

#### Scenario: A reply into a closed thread is answered, not swallowed
- **WHEN** a user posts in the thread of a conversation that is already closed
- **THEN** the surface receives a notice that the conversation is closed and can be reopened, and no input is created

### Requirement: Closing tears down the conversation's resources
Reaching phase `Closed` SHALL remove the conversation's runtime pod and its
`agentops-mcp-conv-<name>` ConfigMap. The freed capacity SHALL become available
to any waiting `Pending` conversation. A closed conversation SHALL consume no
capacity, SHALL NOT be counted in the pending backlog, and SHALL NOT be a member
of the FIFO waiting set — it will never be given a pod, so leaving it there
would starve a conversation that could use one.

The teardown is driven by the PHASE, not by deletion, since the object now
survives its close.

#### Scenario: Runtime pod and ConfigMap are reclaimed at the close
- **WHEN** a conversation with a runtime pod and an MCP ConfigMap is closed
- **THEN** both objects are removed while the `Conversation` itself remains

#### Scenario: Closing admits waiting work
- **WHEN** the cap is full, a conversation is closed, and a `Pending` conversation is waiting
- **THEN** the pending conversation is admitted

#### Scenario: A closed conversation cannot starve a pending one
- **WHEN** a closed conversation still carries unprocessed inputs and a `Pending` conversation is older
- **THEN** the pending conversation is admitted, because the closed one is not in the waiting set

### Requirement: Closing a working conversation abandons its inflight work
`/close` SHALL be honored immediately even when the conversation is inflight:
the runtime pod is removed mid-run and the farewell message SHALL state that
in-progress work was abandoned.

#### Scenario: Inflight run is abandoned with notice
- **WHEN** `/close` is sent while a work unit is inflight
- **THEN** the conversation reaches phase `Closed` and the farewell message says the in-progress work was abandoned

### Requirement: Topics are archived at the close, and the archive is derivable
The manager SHALL enqueue one `close-topic` operation per bound thread when the
conversation reaches phase `Closed`, and SHALL record each completed archive in
`status.threadsArchived[]`. A bound thread absent from that list is an archive
still owed and SHALL be re-derived by reconciliation.

`close-topic` SHALL therefore NO LONGER be the one operation that is not
re-derivable from CR state. It was the exception only because it was enqueued
while the object was disappearing, leaving nothing to record against; the object
now survives its close, so a failed archive is an ordinary owed operation. EVERY
operation kind is now derivable.

The `agentops.dev/close-topics` finalizer SHALL remain for the one path where
the object really does go away — a `Conversation` deleted without having been
closed, by `kubectl delete` or by autodelete — releasing once the outstanding
operations complete or after a bounded grace period of 2 minutes, so deletion
never wedges on an adapter that is down. Deleting an already-closed conversation
SHALL find its threads archived and release immediately.

#### Scenario: Topic archived on close
- **WHEN** a conversation bound to a channel with a thread is closed
- **THEN** a `close-topic` operation carrying that thread id is delivered to the serving adapter and the channel is recorded in `status.threadsArchived[]`

#### Scenario: A failed archive is re-derived
- **WHEN** an adapter reports a `close-topic` operation as failed for a closed conversation
- **THEN** reconciliation re-enqueues it, because the thread is still missing from `status.threadsArchived[]`

#### Scenario: Down adapter cannot block deletion
- **WHEN** a conversation is deleted and no adapter claims the `close-topic` operation within the grace period
- **THEN** the finalizer is removed and the object is deleted anyway

#### Scenario: Manual deletion of a never-closed conversation archives too
- **WHEN** an operator runs `kubectl delete conversation <name>` on a conversation that was never closed
- **THEN** the bound threads receive `close-topic` operations before the object disappears

#### Scenario: Deleting a closed conversation does not archive twice
- **WHEN** a conversation that was properly closed is deleted
- **THEN** its threads are already recorded as archived, no further `close-topic` is enqueued, and the finalizer releases immediately

### Requirement: Closing has exactly one implementation
Every close — however it is ordered — SHALL take the same path: a farewell
posted to every bound thread, the transition to phase `Closed` with
`status.closedAt` stamped, the teardown of the runtime pod and MCP ConfigMap,
the archiving of every bound thread, and the freed capacity admitting a waiting
conversation. There SHALL be exactly one implementation of that sequence, and
every originator SHALL call it rather than reproduce it.

The originators are the `/close` command on a thread, a surface closing several
conversations at once, and the manager's own idle timer. What differs between
them is only WHO decided and what the farewell says.

The farewell SHALL be posted before the status write, because a thread that
simply stops is indistinguishable from a fault, and SHALL carry a STABLE
operation id per conversation × channel × reopen count, so a close whose status
write fails and is retried says goodbye once rather than once per attempt.

**NO REMOTE CLOSE VERB EXISTS.** No HTTP endpoint, no channel adapter contract
operation and no CRD field CLOSES a conversation: an external caller reaches
closing only by posting `/close` on a thread it holds. Deleting and reopening
are separate verbs with their own rule (below) and are not a way to close.

#### Scenario: A batch close is N ordinary closes
- **WHEN** a surface closes several conversations in one gesture
- **THEN** each conversation receives `/close` on that surface's thread with it and takes the ordinary close path

#### Scenario: Teardown is identical whatever ordered the close
- **WHEN** a conversation is closed by a batch, by a hand-typed `/close`, or by the manager's idle timer
- **THEN** its threads are archived, its runtime pod and MCP ConfigMap are reclaimed, and freed capacity admits a waiting conversation — identically in all three cases

#### Scenario: Every close says goodbye, exactly once
- **WHEN** a close is attempted more than once because its status write failed
- **THEN** the farewell reaches every bound thread exactly once, and a close following a reopen is owed its own

#### Scenario: No remote close verb exists
- **WHEN** an external caller looks for an endpoint or contract operation that closes a conversation
- **THEN** there is none: it can only post `/close` on a thread it holds

### Requirement: A surface's close reach is bounded by the threads it holds
A channel surface SHALL be able to CLOSE only the conversations it holds a
thread on. A conversation it merely observes SHALL NOT be closeable from it,
because `/channel/inbound` is reply-only and there is no thread to post the
command on. This SHALL be reported as a bounded reach — naming the binding that
would extend it — and never as a permission error.

DELETING and REOPENING are bounded differently, by the BINDING: a surface may
delete or reopen a conversation whose `spec.channelRefs` names its channel, read
from the conversation and never taken from the request. That is not a weakening
of the rule above but the same property proven another way — *you may only end a
conversation you are part of* — because a CLOSED conversation holds no thread,
so holding one cannot be the proof. Delete SHALL additionally refuse any
conversation that is not already `Closed`.

The bound applies to SURFACES. It does not describe the manager, which holds no
threads and closes conversations directly.

#### Scenario: An observed conversation cannot be closed
- **WHEN** a surface attempts to close a conversation it holds no thread on
- **THEN** the close does not happen and the reason names the missing channel binding

#### Scenario: Reach follows the binding
- **WHEN** a channel is added to a conversation's pipeline and a thread is bound
- **THEN** that surface can close the conversation

#### Scenario: Delete and reopen reach through the binding
- **WHEN** a surface asks to delete or reopen a closed conversation whose `spec.channelRefs` names its channel
- **THEN** the manager performs it; a surface not named in those refs is refused with a reason naming the binding

#### Scenario: Delete refuses a conversation that is not closed
- **WHEN** a surface asks to delete a conversation that is `Idle`, `Queued` or `Working`
- **THEN** the request is refused with a reason naming the missing step, and the conversation is NOT closed as a side effect

## ADDED Requirements

### Requirement: A closed conversation can be reopened
The manager SHALL offer a reopen that returns a `Closed` conversation to phase
`Idle`, clears `status.closedAt` — which stops the delete clock — and advances
`status.reopens`.

Reopening SHALL leave every materialized ref EXACTLY as it is. Refs are
snapshots whose CONTENT is re-read at every use, so a reopen that re-resolved
wiring would let a Pipeline edit re-wire a conversation that already exists. A
reopened conversation is the SAME conversation with the same profile and the
same capabilities, or it is a new conversation wearing an old name.

A reopen whose referenced `AgentProfile` or `Channel` no longer exists SHALL
FAIL, naming the missing object. It SHALL NOT partially reopen and SHALL NOT
silently drop a binding.

Continuity SHALL be restored only where it was promised: under a runtime whose
`contextStorage` keeps context on a volume the agent resumes with its workspace
and its context handle; under `none` it answers fresh and says so.

The manager SHALL announce the reopen on every bound thread. The announcement is
the MANAGER's, not each adapter's — a closing thread already receives a farewell
from the manager, and synthesizing the reopen adapter-side would put it on
whichever surface implemented it and leave the others silently starting to work
again.

#### Scenario: Reopen restores the same conversation
- **WHEN** a closed conversation is reopened
- **THEN** it returns to `Idle` with `status.closedAt` cleared, and its profile, channel bindings, toolsets, MCP configs, recorded runs and context handle are unchanged

#### Scenario: Reopen names a missing ref
- **WHEN** a reopen is attempted for a conversation whose profile no longer exists
- **THEN** the reopen fails naming that profile and the conversation stays `Closed`

#### Scenario: Every bound thread is told
- **WHEN** a conversation bound to two channels is reopened
- **THEN** both threads receive a notice that the conversation is live again, exactly once per reopen

### Requirement: Reopening re-establishes threads through ensure-topic
A reopen SHALL re-establish each bound thread by enqueueing an ordinary
`ensure-topic` operation carrying the archived thread id as an OPTIONAL HINT,
and SHALL update `status.threads[]` from whatever the adapter returns.

There SHALL be no `reopen-topic` operation kind. Most transports cannot
un-archive, so most implementations of a new kind would be a second name for
`ensure-topic`; the hint inverts the cost, because an adapter that ignores it is
already correct. Whether a transport can un-archive is transport knowledge, and
the manager holds none.

The operation id SHALL remain stable per conversation × channel while
incorporating the reopen count, so the request is still derivable by
reconciliation and cannot dedup against the original topic creation.

The reopen announcement SHALL be enqueued AFTER the thread is re-established, so
it lands somewhere that can receive it.

#### Scenario: An adapter that can un-archive continues the same thread
- **WHEN** a conversation is reopened on a transport whose adapter honours the hint
- **THEN** the same thread id is returned and recorded, and the conversation continues where it left off

#### Scenario: An adapter that ignores the hint is still correct
- **WHEN** a conversation is reopened on a transport that cannot un-archive
- **THEN** a fresh thread is opened, its id is recorded, and nothing fails

#### Scenario: A reopen's ensure-topic is not suppressed
- **WHEN** a conversation that already had a topic is reopened
- **THEN** the `ensure-topic` operation reaches the adapter rather than deduping against the original topic creation
