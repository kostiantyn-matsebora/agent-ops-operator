## ADDED Requirements

### Requirement: The contract serves the command vocabulary
The contract SHALL expose the manager-published command vocabulary to an
authenticated adapter, so an adapter with no Kubernetes access can tell a person
what may be typed.

The response SHALL carry the vocabulary entries and the revision identifying
them. Access SHALL require nothing beyond the adapter authentication the
contract already defines.

#### Scenario: An adapter reads the vocabulary
- **WHEN** an authenticated adapter requests the vocabulary
- **THEN** it receives the entries and the current revision

#### Scenario: Authentication is unchanged
- **WHEN** an unauthenticated request is made for the vocabulary
- **THEN** it is refused exactly as every other contract endpoint refuses one

### Requirement: The outbound long-poll carries the vocabulary revision
Every response to the outbound operation long-poll SHALL carry the current
vocabulary revision — a delivered operation and an empty result alike. An
adapter observing a revision different from the one it last fetched SHALL
refetch the vocabulary.

The revision SHALL be additive and OPTIONAL to act on. An adapter that ignores
it SHALL remain fully conformant, and the outbound message contract version
SHALL NOT change: nothing an existing adapter reads is altered, moved or
removed.

This carries a change to an adapter that is otherwise idle, without a second
connection and without the manager needing to reach the adapter — adapters are
not required to be addressable, and the contract stays pull-only.

#### Scenario: An empty poll still carries the revision
- **WHEN** the long-poll times out with no operation to deliver
- **THEN** the empty result carries the current revision

#### Scenario: A delivered operation carries it too
- **WHEN** an operation is delivered
- **THEN** the same response carries the current revision

#### Scenario: An older adapter is unaffected
- **WHEN** an adapter built before the vocabulary existed long-polls
- **THEN** it receives operations unchanged, declares the same contract version,
  and is not refused

#### Scenario: The manager never dials the adapter
- **WHEN** the vocabulary changes
- **THEN** the manager opens no connection to any adapter, and the change is
  observed on the next long-poll response
