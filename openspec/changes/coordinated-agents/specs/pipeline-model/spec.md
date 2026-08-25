## ADDED Requirements

### Requirement: A Pipeline names its agent inline or by reference

A Pipeline SHALL declare its agent EITHER through the six inline capability
fields OR through `spec.agentRef` naming an `Agent`, and the API server SHALL
reject a manifest carrying both. The inline form is unchanged from before this
requirement existed. A referenced Agent's CONTENT is re-read as an inline
field's would be; the resolved identity and storage are snapshotted onto the
conversation exactly as before.

#### Scenario: Both forms refused
- **WHEN** a Pipeline carries `agentRef` and `profileRef`
- **THEN** the API server rejects it naming the exclusivity

#### Scenario: A dangling agentRef
- **WHEN** a Pipeline's `agentRef` names no Agent
- **THEN** its `Ready` is False naming the ref, and it claims nothing

#### Scenario: Existing Pipelines are untouched
- **WHEN** a Pipeline written before `agentRef` existed is applied
- **THEN** it validates and behaves exactly as it did
