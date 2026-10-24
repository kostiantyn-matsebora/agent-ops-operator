## ADDED Requirements

### Requirement: Node state is the fourth suppression axis
Beside rules (`for:`), inhibition and time, suppression SHALL have a node-STATE
axis: events on a draining node are suppressed for as long as the node drains,
per the `k8s-drain-awareness` capability. It SHALL be evaluated before
inhibition and the dwell queue, and SHALL need no cause event and no TTL,
because the state it reads begins before the consequences and ends when the
drain does.

#### Scenario: Order of evaluation
- **WHEN** an event arrives for a pod on a draining node that also matches an inhibit rule's target
- **THEN** it is suppressed by the drain axis and never reaches inhibition or the queue
