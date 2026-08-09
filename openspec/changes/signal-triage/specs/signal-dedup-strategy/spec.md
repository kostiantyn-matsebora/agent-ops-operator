# signal-dedup-strategy

## ADDED Requirements

### Requirement: Deduplication is a manager-side decision behind one seam
The manager SHALL make the deduplication decision at one explicit seam, reached only when a signature group has no live Conversation to attach to. The decision SHALL be one of three verdicts: **create** a Conversation, **attach** to a named existing Conversation, or **drop** with a reason.

The manager SHALL supply the deciding strategy with the set of candidate Conversations it may attach to: live, within the source's reuse window, in the manager's namespace. Only the manager can see every source, every open Conversation, and the window they live in — which is the concrete sense in which deduplication is manager-side and cannot be delegated to an adapter.

Adapters SHALL NOT deduplicate beyond their own delivery-retry needs.

#### Scenario: The seam is only reached for would-be new conversations
- **WHEN** a signature group matches a Conversation inside the reuse window
- **THEN** the input is attached without consulting any strategy

#### Scenario: A strategy may only attach to a supplied candidate
- **WHEN** a strategy returns an attach verdict naming a Conversation that was not in the supplied candidate set
- **THEN** the verdict is rejected and treated as a create

### Requirement: The default strategy preserves today's behavior exactly
The default strategy SHALL be deterministic and SHALL return **create** whenever the seam is reached. Fingerprint cooldown, signature grouping, and window-based reuse SHALL continue to be applied by the manager before the seam, unchanged.

An installation that configures nothing SHALL observe behavior identical to before this capability existed. This equivalence SHALL be demonstrated by test rather than asserted by inspection.

#### Scenario: Default configuration is behavior-identical
- **WHEN** a source declares no dedup strategy and signals arrive that previously created Conversations
- **THEN** the same Conversations are created, with the same signatures, inputs, and input types

#### Scenario: Cheap checks still run first
- **WHEN** a signal's fingerprint is within the cooldown window
- **THEN** it is suppressed before the seam is reached, regardless of which strategy is configured
