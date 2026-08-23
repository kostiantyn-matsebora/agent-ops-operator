# conversation-context-continuity Specification

## Purpose

What a conversation's MEMORY is worth and what agent-ops promises about it.

`runtimeContextId` is the runtime's opaque handle on everything a conversation
has accumulated. The manager stores it, hands it back on the next work unit, and
interprets nothing inside it — so this capability is about the PROMISE around the
handle rather than its contents:

- **Latest-wins**, because a run may legitimately end in a different context than
  it was asked to continue.
- **Promised only where possible** — `AgentRuntime.spec.contextStorage` against
  the configured home volume. Never-promised answers fresh and says so;
  promised-and-lost FAILS the run, because a conversation without its context is
  a new one wearing its name.
- **An outage before a loss.** One breaker, fed by both edges — runs that report
  an unreachable context and pods that cannot be provisioned for a storage
  reason — holds work rather than destroying every conversation's memory in one
  storage incident.
- **Reset is explicit**, never automatic, because an automatic reset is
  indistinguishable from the silent degradation the rest of this forbids.

## Requirements

### Requirement: The recorded context handle is the one that exists
The manager SHALL record the runtime context id reported by every completed run,
replacing any previously recorded one, rather than recording only the first.

Write-once is unsound because a run may legitimately end in a different context
than it was asked to continue — the runtime's stored state may be gone, and it
correctly continues with a new one. Keeping the first handle then names something
that no longer exists, and because both dispatch and ingest key their decisions
off it, every later message repeats the same failed continuation. A single
recoverable loss becomes a permanent one.

#### Scenario: A fallback is adopted, not ignored
- **WHEN** a run is asked to continue a context, cannot, and completes in a new one
- **THEN** the conversation records the new handle, and the next message continues from it

#### Scenario: One loss stays one loss
- **WHEN** a runtime legitimately branches to a new context and reports it
- **THEN** the new handle is recorded, and later messages continue from it rather than repeating the failed continuation

#### Scenario: A failed continuation leaves the handle alone
- **WHEN** a run fails because the context was unavailable and establishes nothing
- **THEN** there is no new handle to record, and the conversation keeps the one it had alongside the record that it can no longer be continued

#### Scenario: An ordinary run keeps its handle
- **WHEN** a run continues successfully and reports the same handle
- **THEN** the recorded handle is unchanged

### Requirement: Context continuity is a contract, not a runtime's mechanics
The work contract SHALL express continuity in terms of an OPAQUE runtime context
handle and an obligation, never in terms of any runtime's implementation of it.

The handle SHALL be named in agent-ops' own vocabulary — `runtimeContextId` — and
NOT after any runtime's word for it. `session` belongs to claude-code; another
backend calls it a thread, and another has no such concept at all. A vendor's
noun in the operator's API teaches every later reader that the operator knows
what is inside the handle, which it does not.

The manager SHALL store whatever handle a run reports and hand it back on the
next work unit for that conversation. It SHALL NOT interpret the handle, and
SHALL NOT assume where the context it names is stored — session files on a
mounted volume, a thread id at a vendor API, and rows in a database are all
valid, and the manager can distinguish none of them.

Given a handle, a runtime SHALL either continue that context or report that it
could not. A runtime with no continuation concept SHALL be able to conform by
always reporting that it started a new one.

`--resume` is one runtime's implementation of this obligation, and `session` is
that runtime's word for the thing; neither SHALL appear in the contract's
vocabulary.

#### Scenario: A fallback is distinguishable from a fresh start
- **WHEN** a run is asked to continue a context, cannot, and starts a new one
- **THEN** it reports the fallback, and the manager can tell that context was lost rather than never requested

#### Scenario: An older runtime keeps working
- **WHEN** a runtime that does not implement the report completes a run
- **THEN** the handle is recorded as before and no continuity claim is made

#### Scenario: A runtime without continuation conforms without pretending
- **WHEN** a runtime whose backend cannot continue anything receives a work unit carrying a handle
- **THEN** it reports that it started a new context, and the conversation records that its context does not carry between runs

#### Scenario: The manager stays out of the storage question
- **WHEN** a runtime keeps its context somewhere the operator cannot see, such as a vendor API
- **THEN** continuity works unchanged, because the manager only stores and returns the handle

### Requirement: Lost context is recorded on the conversation
A run that lost context SHALL set a continuity condition on the `Conversation`
naming when the thread restarted and the likely cause. A later run that resumes
successfully SHALL clear it.

This is the one failure that leaves a conversation looking entirely healthy —
same phase, same run history, answers still arriving — while every answer is
given without memory. Every other health fact about a conversation is a
condition, and this SHALL be one too, so the question "why does it not remember?"
is answerable from the object rather than from the logs of a pod that has exited.

The recorded cause SHALL be the one the RUNTIME reported. The manager SHALL NOT
infer a cause: it does not know whether a given runtime keeps sessions on a
volume, at a vendor API, or nowhere, so any explanation it invented would be
confident and sometimes wrong.

#### Scenario: A forgetful conversation says so
- **WHEN** a run falls back to a new session
- **THEN** the conversation carries a condition naming the restart and its likely cause

#### Scenario: Recovery clears the record
- **WHEN** a later run resumes the current session successfully
- **THEN** the condition is cleared, because it describes the present rather than accumulating history

#### Scenario: The reason comes from the runtime, not from a guess
- **WHEN** a run reports that it could not continue a session and supplies a reason
- **THEN** the manager records that reason verbatim and adds no explanation of its own, because it does not know where any runtime keeps its sessions

#### Scenario: An ephemeral install is diagnosed, not blamed
- **WHEN** the reference runtime cannot find its session files and the install has no home volume
- **THEN** the reason it reports names that, so a deliberate configuration is not presented as a malfunction

### Requirement: A failed run still surrenders its context handle
A run that fails SHALL report the runtime context id it established, when it
established one, and the manager SHALL record it.

Otherwise a crash after the session started strands it: the files exist on the
volume, nothing references them, and the next message starts over. Continuing
from where a failure happened is what someone retrying after an error expects.

#### Scenario: A crash does not discard the work before it
- **WHEN** a run establishes a context, does work, and then fails
- **THEN** the handle is recorded and the next message continues from it

### Requirement: A context that cannot be continued fails the run
When a runtime is given a handle and cannot continue the context it names, it
SHALL report the run as FAILED with an explicit reason. It SHALL NOT start a
fresh context and answer as though nothing were missing.

A conversation without its context is not that conversation — it is a new one
wearing the same name and the same thread. Answering anyway presents the second
as the first to everyone involved: the person replying, the operator reading the
object, and the agent itself. An agent asked to undo something it has no memory
of will guess or ask, and where the runtime holds broad cluster rights the guess
is the expensive outcome.

The failure SHALL be articulate. It SHALL carry a reason, and the bound threads
SHALL be told what happened and that a new conversation is the remedy — a failed
run with an empty result is the reason this behaviour was avoided before, and it
SHALL NOT be reintroduced as the fix.

A runtime SHALL distinguish a context store that reports the context GONE from
one that did not ANSWER, and SHALL retry briefly before declaring a context
unavailable. A shared filesystem can fail to answer for seconds — a restarting
share-manager, a stale handle after a pod moves, a cached directory listing that
has not yet seen a file written on another node — and failing a conversation
permanently on a lag of that kind would turn a storage nicety into a correctness
bug. Only an answer of "not there" is unavailability.

The input that triggered the failed run SHALL be consumed rather than retried, so
an unavailable context produces one clear failure instead of a loop.

#### Scenario: An uncontinuable conversation fails instead of answering
- **WHEN** a run is given a handle whose context no longer exists
- **THEN** the run fails with a stated reason and the agent is not invoked to answer without it

#### Scenario: A storage hiccup is not a lost conversation
- **WHEN** the context store does not answer — a restarting shared filesystem, a stale mount, a file not yet visible from this node — and answers on a retry moments later
- **THEN** the run continues the context normally and no failure is reported

#### Scenario: The person waiting is told what to do
- **WHEN** a run fails because the context was unavailable
- **THEN** every bound thread receives a message naming the cause and pointing at starting a new conversation, rather than a bare failure

#### Scenario: One failure, not a loop
- **WHEN** the input that triggered an uncontinuable run is processed
- **THEN** it is consumed and not redispatched, so the conversation reports one failure rather than failing repeatedly on the same message

#### Scenario: A conversation that cannot continue says so on the object
- **WHEN** a run has failed because its context was unavailable
- **THEN** the conversation records that it can no longer be continued, so the state is visible without reading the thread

### Requirement: Lost context is never simulated from run history
When context has been lost, the next run SHALL start genuinely fresh and the
manager SHALL NOT seed it with previous results.

`status.runs[]` holds truncated results and no tool calls or intermediate
reasoning. Replaying them as if they were the conversation would produce an agent
that believes it remembers and gives a plausible, wrong account of what it did.
An agent that knows it lost the thread is safer than one that half-remembers.

#### Scenario: A fresh context is honestly fresh
- **WHEN** a conversation's context is lost and a new message arrives
- **THEN** the new context receives the new message and no reconstructed history

### Requirement: Renaming the handle must not discard it
The field carrying the handle SHALL be renamed from the runtime-specific
`sessionId` — now the retired spelling — to `runtimeContextId`, and the rename
SHALL NOT itself lose context.

For one release the manager SHALL READ both fields — preferring the new one and
adopting the old when only it is present — and SHALL WRITE only the new. The work
unit SHALL carry both for the same period, so a runtime image can be upgraded
independently of the manager.

A rename that simply moved the field would strand every in-flight conversation's
handle at the moment of upgrade, restarting the context of all of them. That is
precisely the failure this capability exists to prevent, and it would be
self-inflicted.

#### Scenario: An in-flight conversation survives the upgrade
- **WHEN** a manager carrying this change first observes a conversation whose handle is recorded under the old field
- **THEN** that handle is used and re-recorded under the new field, and the conversation continues without restarting its context

#### Scenario: A runtime can be upgraded on its own schedule
- **WHEN** a work unit reaches a runtime that reads only the old field, or one that reads only the new
- **THEN** both find the handle, because the unit carries it under both names for the transition

### Requirement: Continuity is promised only where its prerequisite is met
An `AgentRuntime` SHALL declare where its context lives — on its home volume, in
storage the operator does not provide, or nowhere — and the manager SHALL check
that declaration against the deployment before promising continuity.

When a runtime keeps context on its home volume and no durable home volume is
configured, continuity is impossible in that deployment. The manager SHALL NOT
send a context handle, and the conversation SHALL record from the outset that it
cannot be continued.

Such a conversation SHALL run each input fresh rather than failing. A deployment
that cannot carry context is a configuration an operator chose, not a fault, and
failing every follow-up in it would make a supported configuration look broken —
with a short idle timeout the runtime pod exits between almost any two messages.

The distinction is between continuity that was NEVER PROMISED and continuity that
was promised and then lost. Only the second fails a run.

#### Scenario: An ephemeral install is single-run by declaration
- **WHEN** the runtime keeps context on its home volume and the deployment provides none
- **THEN** no handle is issued, the conversation states from its first run that it cannot be continued, and each message is answered fresh rather than failing

#### Scenario: A runtime that stores context elsewhere is unaffected
- **WHEN** a runtime declares that its context lives outside the operator's storage
- **THEN** continuity is promised regardless of whether a home volume is configured

#### Scenario: Losing what was promised still fails
- **WHEN** continuity was possible and a run finds the context gone
- **THEN** the run fails, because this is a loss rather than a configuration

### Requirement: Unavailability is treated as an outage before it is treated as a loss
A run SHALL fail for an unavailable context only after bounded retries within the
run have been exhausted AND the system is not experiencing widespread
unavailability.

The manager SHALL track reports of unavailable context across conversations. When
they exceed a threshold within a window, it SHALL treat the cause as
infrastructure rather than as many simultaneous losses: affected inputs SHALL be
HELD for later dispatch rather than failed, dispatch of continuations SHALL pause,
and the state SHALL be reported. Normal dispatch SHALL resume once continuation
succeeds again, and the held work SHALL proceed with its context intact.

The manager SHALL treat a repeated FAILURE TO PROVISION a runtime pod as the same
evidence, where the failure is attributable to context storage — a volume that
will not attach or mount. A pod that never starts reports nothing, so a mechanism
fed only by run reports cannot see the most total form of the outage it exists
for: every conversation blocked, and no run alive to say so.

Failures NOT attributable to storage — an image that will not pull, a pod that
cannot be scheduled for resources — SHALL NOT count toward this threshold, so
that an unrelated fault cannot hold every conversation for a storage reason.

While the manager is treating storage as unavailable it SHALL NOT provision
runtime pods, and it SHALL re-test availability with a single probe rather than
by letting every waiting conversation retry.

This exists because failing fast on every report turns a recoverable
infrastructure incident into the permanent destruction of every active
conversation's context — a worse outcome than the silent degradation it replaces,
and an irreversible one. One conversation reporting an unavailable context is a
loss; every conversation reporting one at the same moment is an outage.

#### Scenario: A storage outage holds work rather than destroying it
- **WHEN** many conversations report an unavailable context within a short window
- **THEN** their inputs are held rather than failed, continuation dispatch pauses, and the reason is reported

#### Scenario: Pods that cannot start are the same outage
- **WHEN** several runtime pods in a row fail to start because their context volume will not attach
- **THEN** the manager treats storage as unavailable, stops provisioning, and holds work rather than failing it

#### Scenario: An unrelated provisioning failure does not hold work
- **WHEN** runtime pods fail to start because their image cannot be pulled
- **THEN** storage is not treated as unavailable and conversations are not held for that reason

#### Scenario: Recovery is probed once, not by everyone
- **WHEN** storage is being treated as unavailable
- **THEN** availability is re-tested by a single probe rather than by every waiting conversation attempting a pod

#### Scenario: Held work resumes with its context
- **WHEN** continuation succeeds again after such an outage
- **THEN** the held inputs are dispatched and continue the contexts they were always meant to

#### Scenario: An isolated loss is still a loss
- **WHEN** one conversation reports an unavailable context while others continue normally
- **THEN** that run fails, because the evidence points at that conversation rather than at the infrastructure

#### Scenario: A genuinely absent store does not queue forever
- **WHEN** unavailability persists and continuation never succeeds
- **THEN** the state remains reported rather than silently accumulating work with no explanation

### Requirement: Durable context does not require distributed storage
Continuity SHALL be achievable without a distributed storage provider. A
single-node topology — a ReadWriteOnce claim, or a node-affine PersistentVolume,
with runtime pods pinned to that node — SHALL be a documented, supported way to
have it.

A configuration that pins nothing while using a claim that only one node may
attach SHALL be reported, because it works until a second conversation schedules
elsewhere and then fails on attachment, at a moment far removed from the setting
that caused it. It SHALL NOT fail the render, since a single-node cluster needs
no pinning.

The runtime pod SHALL NOT be given a host filesystem path as its context store:
it executes agent code, and reaching the node's filesystem is not a capability
that should follow from wanting durable context.

#### Scenario: A cluster without distributed storage can still continue conversations
- **WHEN** an operator uses a single-node claim and pins runtime pods to that node
- **THEN** context survives pod restarts and conversations continue normally

#### Scenario: An unpinned single-attach claim is called out
- **WHEN** persistence uses a claim only one node may attach and no placement constraint pins runtime pods
- **THEN** the install succeeds and the notes state that a second concurrent conversation will fail to attach, naming the remedy

### Requirement: A known-bad context volume costs continuity, not availability

When context storage is known to be unusable, the manager SHALL be able to start
a runtime pod WITHOUT its context volume rather than not at all, and SHALL mark
the affected conversation as having lost its context.

The conversation SHALL say so on its bound threads. It SHALL NOT silently answer
as though it had continued, which is the degradation the continuity rules exist
to prevent.

This SHALL NOT weaken the rule that a context which was promised and cannot be
reached fails the run. It applies where the storage is already established to be
unusable, and it exists so that a damaged filesystem stops the memory rather
than the service.

#### Scenario: A broken volume does not stop the system

- **WHEN** the context volume is established to be unusable and a conversation has pending input
- **THEN** a runtime pod starts without the context volume, the conversation is marked context-lost, and the answer says its memory was lost

#### Scenario: The loss is stated, never simulated

- **WHEN** a conversation runs without its context after such a loss
- **THEN** it states that it lost its context, rather than answering as if continuous

### Requirement: A conversation whose context is gone can be reset explicitly

An operator SHALL be able to reset a conversation whose context is
unrecoverable. The reset SHALL clear the recorded context handle, leave the
conversation and its threads intact, and cause the conversation to state that it
is continuing without its previous memory.

The reset SHALL be explicit and operator-initiated. It SHALL NOT be performed
automatically on a failed continuation, because an automatic version would be
indistinguishable from the silent degradation that the continuity rules forbid.

Without this, a conversation whose context is genuinely gone can only fail every
subsequent run or be deleted, and deleting a conversation to recover from a
storage fault destroys its history for an unrelated reason.

#### Scenario: An operator recovers a conversation after unrecoverable loss

- **WHEN** an operator resets a conversation whose context store was destroyed
- **THEN** the context handle is cleared, the conversation and its threads survive, and the next run starts fresh and says so

#### Scenario: Reset never happens on its own

- **WHEN** a run fails because its context could not be reached
- **THEN** the conversation is not reset automatically
