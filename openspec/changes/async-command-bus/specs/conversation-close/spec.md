## MODIFIED Requirements

### Requirement: Closing has exactly one implementation
Every close — however it is ordered — SHALL take the same path: a farewell
enqueued to every bound thread, the transition to phase `Closed` with
`status.closedAt` stamped, the teardown of the runtime pod and MCP ConfigMap.

That path SHALL be performed by a `close` task. A `close` task closes one
conversation. A request, whatever surface it came from, enqueues `close` tasks
and does not close a conversation itself.

The archiving of every bound thread, and the freed capacity admitting a
waiting conversation, SHALL follow.

RECURSIVELY, the same sequence SHALL also apply to every LIVE conversation
this one directly caused. There SHALL be exactly one implementation of that
sequence, cascade included, and every originator SHALL call it rather than
reproduce it.

The originators are the `/close` command on a thread, a surface closing
several conversations at once, the manager's own idle timer, a budget being
exceeded, a Coordinator's own MCP `close` verb, and a member's `escalate`. What differs between them
is WHO decided and what the farewell says.

A close issued by a Coordinator's root on itself or on a direct member it
caused still REQUIRES a reason, unchanged from `A close may carry a
reason, and a coordinator's close must carry one`.

A close driven by a member's `escalate` carries the escalation message as its
reason. A close driven by a budget being exceeded carries the reason
`budget-exceeded`, and no caller supplies one.

The cascade carries that reason VERBATIM to every conversation it reaches.

A close issued by any other originator — `/close`, a batch, the idle timer —
carries no required reason, and the cascade it drives propagates none.

The farewell SHALL be enqueued before the status write, because a thread that
simply stops is indistinguishable from a fault.

It SHALL carry a STABLE operation id per conversation × channel × reopen
count, so a close whose status write fails and is retried says goodbye once
rather than once per attempt. The close handler SHALL be idempotent, so a
task delivered twice closes once.

A cascaded close follows the SAME farewell rule on each conversation it
reaches — its own bound threads, if it has any. A member typically has none.

**NO REMOTE CLOSE VERB EXISTS.** No HTTP endpoint, no channel adapter
contract operation and no CRD field CLOSES a conversation: an external
caller reaches closing only by posting `/close` on a thread it holds.

The Coordinator's MCP `close` verb is not an external caller's. It is
reachable only from inside a coordination tree, and only on the calling
root itself or a direct member it caused.

Deleting and reopening are separate verbs with their own rules and are not
a way to close. Deleting's cascade is the requirement added in this file.

#### Scenario: A batch close is N ordinary closes
- **WHEN** a surface closes several conversations in one gesture
- **THEN** each conversation is closed through its own thread as an ordinary close, each becomes its own `close` task with its own invocation id, and a cascade's tasks share one

#### Scenario: Teardown is identical whatever ordered the close
- **WHEN** a conversation is closed by a batch, by a hand-typed `/close`, or by the manager's idle timer
- **THEN** its threads are archived, its runtime pod and MCP ConfigMap are reclaimed, and freed capacity admits a waiting conversation — identically in all three cases

#### Scenario: Every close says goodbye, exactly once
- **WHEN** a close is attempted more than once because its status write failed or its task was redelivered
- **THEN** the farewell reaches every bound thread exactly once, and a close following a reopen is owed its own

#### Scenario: No remote close verb exists
- **WHEN** an external caller looks for an endpoint or contract operation that closes a conversation
- **THEN** there is none: it can only post `/close` on a thread it holds

#### Scenario: /close cascades to a root's members
- **WHEN** a user sends `/close` in the thread of a root with two live members
- **THEN** the root and both members move to phase `Closed`, each torn down and archived exactly as the root is, with no reason recorded on any of them

#### Scenario: The idle timer's close cascades too
- **WHEN** the manager's idle timer closes a root that still has live members
- **THEN** every live member closes with it, through the same cascade, with no reason recorded

#### Scenario: A coordinator's own reasoned close still propagates its reason
- **WHEN** a Coordinator's root closes itself with a reason, through its MCP `close` verb
- **THEN** every member the cascade reaches is `Closed` with that SAME reason, verbatim

### Requirement: Deleting a conversation cascades to its already-closed descendants
Deleting a `Closed` conversation SHALL first recursively delete every
conversation reachable from it by `causedBy`, at any depth, tolerating one that is
already gone (`NotFound`). It SHALL do so before deleting the named
conversation itself.

That order SHALL be the order of tasks. The request enqueues one `delete` task
per descendant, deepest first, then one for the named conversation, all on the
ROOT conversation's ordering key. A task is claimable only after the one before
it settled.

This mirrors the close cascade above (`Closing has exactly one
implementation`): a coordination's members are never left orphaned,
unreachable and occupying the API after their root is gone.

Every descendant this cascade reaches SHALL already be `Closed` by the time
its ancestor is, because the close cascade closes an entire subtree before
any of it can be deleted.

This requirement therefore never meets a live descendant, and needs no
"descendant not yet closed" refusal state of its own.

The cascade acts on each descendant DIRECTLY, the way the manager closes
one directly rather than through a surface.

A member holds no channel binding of its own, so the surface-bound reach
that governs deleting the NAMED conversation does not apply to the
descendants the cascade reaches on its behalf.

A `delete` task for a conversation that is already gone SHALL succeed. It
SHALL NOT go dead for `not_found`. This is the one stated exception to
`not_found` being terminal, because an absent target is the outcome the caller
wanted.

Deletion's own per-descendant notice (`delete-conversation` on every bound
thread) still applies to each conversation the cascade deletes — which is
typically none, since a member binds no human channel. This is exactly how
it already applies to the one conversation named directly.

#### Scenario: Deleting a root deletes its closed members too
- **WHEN** a root with three closed members is deleted
- **THEN** all three members are deleted along with the root, each by its own task

#### Scenario: A nested member's own members are reached too
- **WHEN** a deleted root's member is itself a closed Coordinator's root with its own closed members
- **THEN** the cascade reaches every one of them, at every depth

#### Scenario: An already-gone descendant is tolerated
- **WHEN** a member was already deleted by some other path before its root's deletion is requested
- **THEN** the root's deletion proceeds, treating the missing member as already gone

#### Scenario: The cascade never meets a live descendant
- **WHEN** a root's deletion cascade reaches any of its descendants
- **THEN** each one is already `Closed`, because closing the root cascaded to it first

#### Scenario: A member's deletion is ordered before its root's
- **WHEN** a root with closed members is deleted
- **THEN** the root's task is not claimable until every member's task has settled

### Requirement: A cascading close closes descendants before the ancestor

When a close cascades over a `causedBy` subtree, the manager SHALL close
every descendant before the conversation whose close started the cascade.
Within the subtree the same order holds at every level: a member's own
members close before that member.

That order SHALL be the order of tasks.

- The request enqueues one `close` task per live descendant, deepest first.
- It then enqueues one for the conversation whose close started the cascade.
- All of them carry the ROOT conversation's ordering key.
- A task is claimable only after the one before it settled.

Every closed conversation in the cascade SHALL carry the same `closeReason`
verbatim, unchanged.

A failure part-way through the walk SHALL leave an OPEN ancestor over a
partly closed subtree, never a closed ancestor over open members. The open
ancestor stays reachable, and a repeated close resumes the walk.

A member task that goes dead settles its key, so the ancestor's task becomes
claimable.

The ancestor's `close` task SHALL therefore check its descendants when it runs.
It SHALL close any descendant still open itself before closing the ancestor.
It SHALL fail with `precondition_failed` rather than close the ancestor over a
descendant it could not close.

The walk SHALL sweep once more after the ancestor closes. A run on the
still-open ancestor may have invoked a member after the first walk listed.

A Closed conversation SHALL be refused when it calls `invoke`.

A member created by an `invoke` that read its caller as open just before the
close landed SHALL be closed by that same call.

- The call re-reads the caller after the member exists.
- It closes the member with the caller's reason when it finds the caller
  Closed.

That re-read and the second sweep cover each other:

- If the close landed before the re-read, the call closes the member.
- If it lands after, the second sweep runs after the close write, which is
  after the member was created, and finds it.

No third sweep is needed.

The same holds at every level. Each member's own close repeats the walk and
the refusal, so a grandchild invoked by a member that is being closed is
found by that member's second sweep, and not after it.

#### Scenario: Children first
- **WHEN** a root with two open members, one of which has an open member of its own, is closed with a reason
- **THEN** the grandchild closes first, then its parent, then the sibling, and the root last, all with that reason

#### Scenario: A failure leaves the root open
- **WHEN** closing one member fails part-way through a root's cascade
- **THEN** the root is still open, the members already walked are closed, and a second close on the root resumes from the open ones

#### Scenario: A dead member task does not close the root over an open member
- **WHEN** a member's `close` task goes dead and the root's task then becomes claimable
- **THEN** the root's task closes the open member itself, or fails as `precondition_failed` and leaves the root open

#### Scenario: Already-closed descendants are skipped
- **WHEN** a cascade reaches a member that is already `Closed`
- **THEN** it is left as it is, with its own `closeReason`, and the walk continues

#### Scenario: A member invoked during the walk is closed too
- **WHEN** a run on the still-open ancestor invokes a new member after the cascade listed its members
- **THEN** the second sweep after the ancestor closes finds that member and closes it with the same reason

#### Scenario: A close lands while a member is being created
- **WHEN** a conversation's close lands after its `invoke` read the caller as open and before the member exists
- **THEN** the call closes the new member with the caller's reason and is refused naming the closed caller

#### Scenario: A closed caller creates no member
- **WHEN** a conversation that is already `Closed` calls `invoke`
- **THEN** the call is refused, no member is created, and the refusal names the closed caller

### Requirement: A closed conversation can be reopened
The manager SHALL offer a reopen that returns a `Closed` conversation to phase
`Idle`, clears `status.closedAt` — which stops the delete clock — and advances
`status.reopens`.

Reopening SHALL leave every materialized ref EXACTLY as it is. Refs are snapshots whose CONTENT is re-read at every use, so a reopen that re-resolved wiring would let a Pipeline edit re-wire a conversation that already exists.

A reopened conversation is the SAME conversation with the same profile and the same capabilities, or it is a new conversation wearing an old name.

A reopen whose referenced `AgentProfile` or `Channel` no longer exists SHALL FAIL, naming the missing object. A ref already missing at accept SHALL be refused with `409`. A ref that disappears after accept SHALL fail the task with `precondition_failed`.

It SHALL NOT partially reopen and SHALL NOT silently drop a binding.

Continuity SHALL be restored only where it was promised: under a runtime whose
`contextStorage` keeps context on a volume the agent resumes with its workspace
and its context handle; under `none` it answers fresh and says so.

The manager SHALL announce the reopen on every bound thread.

The announcement is the MANAGER's, not each adapter's — a closing thread already receives a farewell from the manager, and synthesizing the reopen adapter-side would put it on whichever surface implemented it and leave the others silently starting to work again.

#### Scenario: Reopen restores the same conversation
- **WHEN** a closed conversation is reopened
- **THEN** it returns to `Idle` with `status.closedAt` cleared, and its profile, channel bindings, toolsets, MCP configs, recorded runs and context handle are unchanged

#### Scenario: Reopen names a missing ref
- **WHEN** a reopen is requested for a conversation whose profile no longer exists
- **THEN** it is refused at accept with `409` naming that profile and the conversation stays `Closed`

#### Scenario: Every bound thread is told
- **WHEN** a conversation bound to two channels is reopened
- **THEN** both threads receive a notice that the conversation is live again, exactly once per reopen

## ADDED Requirements

### Requirement: A close, a delete and a reopen are performed by tasks
A close, a delete and a reopen SHALL be accepted and answered as
`async-operations` states. The conversation reaches its new phase when the task
succeeds, and the task is the record of whether it did.

A delete SHALL NOT enqueue a close as a side effect.

A reopen whose referenced `AgentProfile` or `Channel` is already missing SHALL
be refused at accept as the modified reopen requirement states. One that goes
missing after accept SHALL fail its task with `precondition_failed`, naming the
missing object. Either way the conversation SHALL stay `Closed`.

#### Scenario: A close reaches Closed when its task succeeds
- **WHEN** a close is accepted
- **THEN** the conversation reaches `Closed` when the task succeeds

#### Scenario: A reopen over a profile deleted after accept fails its task
- **WHEN** a reopen is accepted and the conversation's profile is deleted before the task runs
- **THEN** the task goes dead as `precondition_failed` naming the profile and the conversation stays `Closed`
