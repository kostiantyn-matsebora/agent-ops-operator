## MODIFIED Requirements

### Requirement: A conversation adopts its correlation id at creation

`Conversation.spec.correlationId` SHALL record the id that opened the conversation, written once at creation and never changed. A root adopts the signal's `initiationId`. A member adopts the id its `invoke` mints instead (`coordination-loop`'s own requirement).

The derived signature and the `agentops.dev/signature-hash` label are removed. `correlationId` is the conversation's correlation identity now.

```yaml
spec:
  correlationId: "{}:{alertname=\"NodeDown\"}"   # adopted verbatim, opaque
```

The id is OPAQUE. The manager SHALL compare it only for equality and SHALL NOT parse, derive or interpret it.

The manager SHALL stamp the `correlationId` on everything leaving the conversation — member spawns, channel topic bindings and the runtime context — so a return trip arrives already correlated, the same id rather than a second addressing mode.

A conversation created before this field existed SHALL NOT be backfilled. It carries no `correlationId`, so equality routing never matches it and a recurrence opens a new conversation — a bounded one-time duplication, never a wrong merge.

#### Scenario: Written once

- **WHEN** a conversation with a `correlationId` is reopened, re-wired or reconciled
- **THEN** its `correlationId` is unchanged

#### Scenario: A legacy conversation is never matched

- **WHEN** a signal's `initiationId` would match an open conversation created before `correlationId` existed
- **THEN** a new conversation is created adopting the id, and the legacy conversation is untouched

#### Scenario: The id is carried verbatim on the way out

- **WHEN** the manager spawns a member or binds a channel topic for a conversation
- **THEN** the `correlationId` is stamped on it verbatim, uninterpreted

### Requirement: Reuse is scoped by causedBy and the correlation id

Conversation reuse SHALL match only conversations with the same `causedBy` AND the same `correlationId` — an uncaused conversation reuses only other uncaused ones, a member only members of the same PARENT invoked as the same entry. Depth plays no part in the comparison.

A member's correlation id is minted by the invoking `invoke` — the sender of that work — never computed by the manager.

#### Scenario: Two incidents, one id
- **WHEN** two uncaused conversations each invoke the same AgentCapability and the invokes carry one correlation id
- **THEN** two member conversations exist, one per parent

#### Scenario: Same parent, nested
- **WHEN** a nested Coordinator invokes the same AgentCapability twice with invokes carrying one correlation id
- **THEN** the second invoke attaches to the first member rather than creating a second
