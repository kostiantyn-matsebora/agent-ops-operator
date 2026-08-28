## ADDED Requirements

### Requirement: Origination prefers a reservation over a new object
When the signal core would create a new Conversation for a Pipeline, it SHALL
first attempt to adopt that Pipeline's oldest `Reserved` conversation, and
create a new object only when none exists or every attempt is lost to a
concurrent eviction. Adoption SHALL produce a conversation indistinguishable
from a created one: same inputs, signature label, title, provenance and
wiring snapshot. Neither the pending-backlog bound nor reuse SHALL change.

#### Scenario: Adoption is invisible downstream
- **WHEN** a conversation is adopted rather than created
- **THEN** its topics, inputs, `pipelineRef` and signature label are those the signal would have produced on a new object

#### Scenario: No reservation, unchanged path
- **WHEN** the Pipeline holds no reservation
- **THEN** the conversation is created exactly as before
