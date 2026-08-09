## MODIFIED Requirements

### Requirement: Grouping policy stays manager-side for every source type
Signature grouping, fingerprint cooldown, window-based conversation reuse, and recurrence-on-session SHALL be applied by the manager from `spec.grouping` for signals of every source type. Adapters SHALL NOT need to implement any grouping logic.

Cooldown state SHALL be durable rather than process memory: the manager SHALL record fingerprint suppression on the owning `SignalSource` and SHALL load it before applying cooldown to that source after a restart, so a restart does not re-open conversations for signals inside an active window. An in-memory map MAY remain the hot path, but SHALL NOT be the record. Recorded entries SHALL be pruned once past their window so the object stays bounded.

#### Scenario: Cooldown suppresses adapter-fed duplicates
- **WHEN** an adapter re-delivers a signal with a fingerprint seen within `cooldownHours`
- **THEN** no new input is created (at-least-once delivery is safe)

#### Scenario: Cooldown survives a manager restart
- **WHEN** the manager restarts and an adapter re-delivers a signal whose fingerprint was suppressed before the restart and is still inside `cooldownHours`
- **THEN** the signal is still suppressed and no duplicate conversation is opened

#### Scenario: Adapter-fed signals group manager-side
- **WHEN** two normalized signals with different fingerprints but identical signature labels arrive via an adapter within the source's window
- **THEN** they land in the same conversation, the second as a recurrence when a session exists

#### Scenario: Same-signature batch collapses to one input
- **WHEN** one inbound batch carries several fresh signals sharing a signature
- **THEN** they land as ONE combined input on one conversation

#### Scenario: Suppression record stays bounded
- **WHEN** recorded suppression entries age past their cooldown window
- **THEN** they are pruned from the `SignalSource` rather than accumulating
