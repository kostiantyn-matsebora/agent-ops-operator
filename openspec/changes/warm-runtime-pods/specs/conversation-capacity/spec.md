## ADDED Requirements

### Requirement: A reservation holds a slot and is the first to give it up
A `Reserved` conversation's pod SHALL count as active. When a conversation
needs a runtime pod and the cap is reached, the manager SHALL evict in this
order: a reservation (any Pipeline's, the newest first) before the longest-idle
conversation, and a `Pending` phase only when neither exists. Free-capacity
accounting SHALL treat every reservation as free, so a reservation is NEVER the
reason a conversation reports `Pending`.

#### Scenario: A workspace conversation takes a warm pod's slot
- **WHEN** the cap is full and one of the pods is a reservation, and a conversation with a workspace claim needs a pod
- **THEN** the reservation is deleted and the conversation's pod is created, and it never enters `Pending`

#### Scenario: A reservation is evicted before an idle conversation
- **WHEN** the cap is full with one reservation and one idle conversation, and a third conversation needs a pod
- **THEN** the reservation is deleted and the idle conversation keeps its pod

#### Scenario: A reservation's own Pipeline needs a pod
- **WHEN** a new conversation on a Pipeline needs a pod and that Pipeline holds a reservation
- **THEN** it adopts the reservation rather than evicting it
