## ADDED Requirements

### Requirement: A cascading close closes descendants before the ancestor

When a close cascades over a `causedBy` subtree, the manager SHALL close
every descendant before the conversation whose close started the cascade.
Within the subtree the same order holds at every level: a member's own
members close before that member.

Every closed conversation in the cascade SHALL carry the same `closeReason`
verbatim, unchanged.

A failure part-way through the walk SHALL leave an OPEN ancestor over a
partly closed subtree, never a closed ancestor over open members. The open
ancestor stays reachable, and a repeated close resumes the walk.

The walk SHALL sweep once more after the ancestor closes. A run on the
still-open ancestor may have invoked a member after the first walk listed.

A Closed conversation SHALL be refused when it calls `invoke`. Closed is
inert, so nothing can be created under it after the close lands, and the
second sweep is the last word. No third sweep is needed.

#### Scenario: Children first
- **WHEN** a root with two open members, one of which has an open member of its own, is closed with a reason
- **THEN** the grandchild closes first, then its parent, then the sibling, and the root last, all with that reason

#### Scenario: A failure leaves the root open
- **WHEN** closing one member fails part-way through a root's cascade
- **THEN** the root is still open, the members already walked are closed, and a second close on the root resumes from the open ones

#### Scenario: Already-closed descendants are skipped
- **WHEN** a cascade reaches a member that is already `Closed`
- **THEN** it is left as it is, with its own `closeReason`, and the walk continues

#### Scenario: A member invoked during the walk is closed too
- **WHEN** a run on the still-open ancestor invokes a new member after the cascade listed its members
- **THEN** the second sweep after the ancestor closes finds that member and closes it with the same reason

#### Scenario: A closed caller creates no member
- **WHEN** a conversation that is already `Closed` calls `invoke`
- **THEN** the call is refused, no member is created, and the refusal names the closed caller
