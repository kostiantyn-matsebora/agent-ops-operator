## ADDED Requirements

### Requirement: A close may carry a reason, and a coordinator's close must

`status.closeReason` SHALL be stamped beside `closedAt` when a close supplies
one. A close issued by a coordinator — on its root or on a member it caused —
SHALL require a reason and SHALL be refused without one. A coordinator SHALL be
unable to close a conversation it did not cause.

#### Scenario: Reason recorded
- **WHEN** a coordinator closes a member with a reason
- **THEN** the member is `Closed` with that `closeReason`

#### Scenario: Out of scope
- **WHEN** a coordinator asks to close a conversation another root caused
- **THEN** the request is refused and the conversation is unchanged
