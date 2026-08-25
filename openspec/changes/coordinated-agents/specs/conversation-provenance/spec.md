## Purpose
`Conversation.spec.causedBy` records which conversation started this one, making the causal tree a fact in the API rather than an inference.

## ADDED Requirements

### Requirement: causedBy is provenance and resolves nothing

`Conversation.spec.causedBy` SHALL name the ROOT conversation of the
coordination that created this one, plus the `agents[]` entry name it was
invoked as. It SHALL be written once at creation and never changed. Nothing
SHALL resolve a profile, a channel set, a capability or a delivery decision
through it.

#### Scenario: Written once
- **WHEN** a conversation with `causedBy` is reopened, re-wired or reconciled
- **THEN** `causedBy` is unchanged

#### Scenario: Provenance decides no delivery
- **WHEN** a caused conversation binds a channel through any path
- **THEN** delivery to that channel follows the ordinary per-destination rule, and `causedBy` is not consulted

### Requirement: Reuse is scoped by causedBy

Conversation reuse by signature SHALL match only conversations with the same
`causedBy` — a root conversation reuses only uncaused ones, a member only
members of the same root invoked as the same entry.

#### Scenario: Two incidents, one signature
- **WHEN** two roots each invoke the same AgentCapability with inputs of one signature
- **THEN** two member conversations exist, one per root

### Requirement: The tree is derivable from the API alone

The set of conversations caused by a root, their order and their phases SHALL
be derivable from `causedBy` and creation timestamps with no other state.

#### Scenario: A viewer rebuilds the tree
- **WHEN** a client lists conversations selecting on the root's name
- **THEN** it receives every member, and nothing that belongs to another root
