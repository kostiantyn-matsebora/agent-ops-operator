## MODIFIED Requirements

### Requirement: Closing has exactly one implementation
Every close — however it is ordered — SHALL take the same path: a farewell
posted to every bound thread, the transition to phase `Closed` with
`status.closedAt` stamped, the teardown of the runtime pod and MCP ConfigMap.

The archiving of every bound thread, and the freed capacity admitting a
waiting conversation, SHALL follow.

RECURSIVELY, the same sequence SHALL also apply to every LIVE conversation
this one directly caused. There SHALL be exactly one implementation of that
sequence, cascade included, and every originator SHALL call it rather than
reproduce it.

The originators are the `/close` command on a thread, a surface closing
several conversations at once, the manager's own idle timer, and a
Coordinator's own MCP `close` verb. What differs between them is WHO
decided and what the farewell says.

A close issued by a Coordinator's root on itself or on a direct member it
caused still REQUIRES a reason, unchanged from `A close may carry a
reason, and a coordinator's close must carry one`.

The cascade carries that reason VERBATIM to every conversation it reaches.

A close issued by any other originator — `/close`, a batch, the idle timer —
carries no required reason, and the cascade it drives propagates none.

The farewell SHALL be posted before the status write, because a thread that
simply stops is indistinguishable from a fault.

It SHALL carry a STABLE operation id per conversation × channel × reopen
count, so a close whose status write fails and is retried says goodbye once
rather than once per attempt.

A cascaded close follows the SAME farewell rule on each conversation it
reaches — its own bound threads, if it has any. A member typically has none.

**NO REMOTE CLOSE VERB EXISTS.** No HTTP endpoint, no channel adapter
contract operation and no CRD field CLOSES a conversation: an external
caller reaches closing only by posting `/close` on a thread it holds.

Deleting and reopening are separate verbs with their own rule (below) and
are not a way to close.

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

#### Scenario: /close cascades to a root's members
- **WHEN** a user sends `/close` in the thread of a root with two live members
- **THEN** the root and both members move to phase `Closed`, each torn down and archived exactly as the root is, with no reason recorded on any of them

#### Scenario: The idle timer's close cascades too
- **WHEN** the manager's idle timer closes a root that still has live members
- **THEN** every live member closes with it, through the same cascade, with no reason recorded

#### Scenario: A coordinator's own reasoned close still propagates its reason
- **WHEN** a Coordinator's root closes itself with a reason, through its MCP `close` verb
- **THEN** every member the cascade reaches is `Closed` with that SAME reason, verbatim

## ADDED Requirements

### Requirement: Deleting a conversation cascades to its already-closed descendants
Deleting a `Closed` conversation SHALL first recursively delete every
conversation reachable from it by `causedBy`, at any depth, tolerating one that is
already gone (`NotFound`). It SHALL do so before deleting the named
conversation itself.

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

Deletion's own per-descendant notice (`delete-conversation` on every bound
thread) still applies to each conversation the cascade deletes — which is
typically none, since a member binds no human channel. This is exactly how
it already applies to the one conversation named directly.

#### Scenario: Deleting a root deletes its closed members too
- **WHEN** a root with three closed members is deleted
- **THEN** all three members are deleted along with the root

#### Scenario: A nested member's own members are reached too
- **WHEN** a deleted root's member is itself a closed Coordinator's root with its own closed members
- **THEN** the cascade reaches every one of them, at every depth

#### Scenario: An already-gone descendant is tolerated
- **WHEN** a member was already deleted by some other path before its root's deletion is requested
- **THEN** the root's deletion proceeds, treating the missing member as already gone

#### Scenario: The cascade never meets a live descendant
- **WHEN** a root's deletion cascade reaches any of its descendants
- **THEN** each one is already `Closed`, because closing the root cascaded to it first
