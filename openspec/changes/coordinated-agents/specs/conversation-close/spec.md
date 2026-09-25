## ADDED Requirements

### Requirement: A close may carry a reason, and a coordinator's close must

`status.closeReason` SHALL be stamped beside `closedAt` when a close supplies
one. A close issued by a conversation that is itself a Coordinator's root —
on itself, or on a DIRECT member it caused — SHALL require a reason and
SHALL be refused without one.

CLOSE REACH IS ONE HOP, the same `causedBy` link result-routing and the
cycle guard use, never the whole subtree. A conversation SHALL be unable to
close anything it did not directly cause, EXCEPT ITSELF: a Coordinator's
root MAY always close its own conversation.

Reaching a deeper descendant means asking the direct member to close it. That
member's own close then cascades to ITS members, keeping the same
`closeReason` verbatim (design.md's recursive close-cascade rule).

#### Scenario: Reason recorded
- **WHEN** a coordinator closes a direct member with a reason
- **THEN** the member is `Closed` with that `closeReason`

#### Scenario: Out of scope — another conversation's member
- **WHEN** a coordinator asks to close a conversation another conversation caused
- **THEN** the request is refused and the conversation is unchanged

#### Scenario: Out of scope — a member's own member
- **WHEN** a conversation asks to close a conversation two hops below it (its member's own member)
- **THEN** the request is refused naming the target as out of scope, and closing the direct member instead cascades to it
