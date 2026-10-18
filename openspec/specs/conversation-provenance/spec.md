# conversation-provenance

## Purpose
`Conversation.spec.causedBy` records the immediate PARENT that started this
one, one hop, so a chain of these links is the causal tree — a fact in the
API rather than an inference, and one that may nest to any depth.

## Requirements

### Requirement: A conversation adopts its correlation id at creation

`Conversation.spec.conversationId` SHALL record the id that opened the conversation, written once at creation and never changed. A root adopts the signal's `initiationId`. A member adopts the id its `invoke` mints instead (`coordination-loop`'s own requirement).

The derived signature and the `agentops.dev/signature-hash` label are removed. `conversationId` is the conversation's correlation identity now.

```yaml
spec:
  conversationId: "{}:{alertname=\"NodeDown\"}"   # adopted verbatim, opaque
```

The id is OPAQUE. The manager SHALL compare it only for equality and SHALL NOT parse, derive or interpret it.

The manager SHALL stamp the `conversationId` on everything leaving the conversation — member spawns, channel topic bindings and the runtime context — so a return trip arrives already correlated, the same id rather than a second addressing mode.

A conversation created before this field existed SHALL NOT be backfilled. It carries no `conversationId`, so equality routing never matches it and a recurrence opens a new conversation — a bounded one-time duplication, never a wrong merge.

#### Scenario: Written once

- **WHEN** a conversation with a `conversationId` is reopened, re-wired or reconciled
- **THEN** its `conversationId` is unchanged

#### Scenario: A legacy conversation is never matched

- **WHEN** a signal's `initiationId` would match an open conversation created before `conversationId` existed
- **THEN** a new conversation is created adopting the id, and the legacy conversation is untouched

#### Scenario: The id is carried verbatim on the way out

- **WHEN** the manager spawns a member or binds a channel topic for a conversation
- **THEN** the `conversationId` is stamped on it verbatim, uninterpreted

### Requirement: causedBy names the parent and resolves nothing

`Conversation.spec.causedBy` SHALL name the PARENT conversation that invoked
this one, never the tree's ultimate root, plus the `agents[]` entry name it
was invoked as. It SHALL be written once at creation and never changed.

```yaml
spec:
  causedBy:
    conversation: incident-7f3a   # the PARENT, one hop
    entry: triage                 # the parent's agents[] entry name
```

Nothing SHALL resolve a profile, a channel set, a capability or a delivery
decision through it.

The conversation `causedBy` names MAY itself carry a `causedBy`. A member may
be a Coordinator's own root for a further level of members, with no depth
limit.

#### Scenario: Written once
- **WHEN** a conversation with `causedBy` is reopened, re-wired or reconciled
- **THEN** `causedBy` is unchanged

#### Scenario: Provenance decides no delivery
- **WHEN** a caused conversation binds a channel through any path
- **THEN** delivery to that channel follows the ordinary per-destination rule, and `causedBy` is not consulted

#### Scenario: A member is itself a parent
- **WHEN** a member conversation is a Coordinator's own root and invokes further members
- **THEN** those members' `causedBy` names that member, not the tree's uncaused root

### Requirement: coordinatorRef names the Coordinator a conversation opened under

`Conversation.spec.coordinatorRef` SHALL be set when the conversation's own
entry point is a Coordinator — addressed directly, or opened as a member whose
`agents[]` entry wires one (nesting). It SHALL be empty for a
Pipeline-addressed conversation.

It SHALL be written once at creation and never changed, exactly as `causedBy`
is. It resolves nothing beyond identifying that Coordinator.

It is ordinary Kubernetes-API state, so it survives a manager restart with
nothing to derive or recompute.

The cycle guard reads it to walk a calling conversation's `causedBy` chain to
the uncaused root, collecting each ancestor's `coordinatorRef`. The console's
incident view reads it too, to tell a member that is itself a Coordinator's
root from a plain member (`console-coordination-view`).

#### Scenario: Absent on a Pipeline-addressed conversation
- **WHEN** a conversation's entry point is a Pipeline, not a Coordinator
- **THEN** `coordinatorRef` is empty

#### Scenario: Set on a nested Coordinator's own root
- **WHEN** an `agents[]` entry wires a Coordinator and is invoked
- **THEN** the created member's `coordinatorRef` names that Coordinator, independent of its `causedBy`

### Requirement: Reuse is scoped by causedBy and the correlation id

Conversation reuse SHALL match only conversations with the same `causedBy` AND the same `conversationId` — an uncaused conversation reuses only other uncaused ones, a member only members of the same PARENT invoked as the same entry. Depth plays no part in the comparison.

A member's correlation id is minted by the invoking `invoke` — the sender of that work — never computed by the manager.

#### Scenario: Two incidents, one id
- **WHEN** two uncaused conversations each invoke the same AgentCapability and the invokes carry one correlation id
- **THEN** two member conversations exist, one per parent

#### Scenario: Same parent, nested
- **WHEN** a nested Coordinator invokes the same AgentCapability twice with invokes carrying one correlation id
- **THEN** the second invoke attaches to the first member rather than creating a second

### Requirement: The tree is derivable from the API alone, walked one hop at a time

The set of conversations descended from an uncaused conversation, their order,
their depth and their phases SHALL be derivable by following `causedBy` links
and creation timestamps, with no other state and no depth limit.

#### Scenario: A viewer rebuilds the tree
- **WHEN** a client walks an uncaused conversation's tree
- **THEN** it lists selecting on that name, then recursively on each result's
  name, until it has collected every descendant at every depth, and nothing
  that belongs to another tree

#### Scenario: A subtree is derivable on its own
- **WHEN** a client walks a nested member's tree the same way
- **THEN** it collects that member's own descendants, independent of its ancestors
