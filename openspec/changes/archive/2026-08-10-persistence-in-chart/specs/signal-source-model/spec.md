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

#### Scenario: Alerts keep the default labels when a source declares none
- **WHEN** two `kind: alert` signals with different fingerprints but the same `alertname` and `namespace` arrive at a source with `grouping: {}`
- **THEN** they land in the same conversation, exactly as before — the default labels still apply to the alert lane

#### Scenario: Recurring jobs keep folding into one conversation
- **WHEN** a job source with no `signatureLabels` fires successive ticks carrying distinct fingerprints
- **THEN** the ticks land in the same conversation and later ones resume the agent session as recurrences

#### Scenario: One-shot lanes key on the fingerprint
- **WHEN** two `kind: task` signals with different fingerprints arrive at a source with no `signatureLabels`
- **THEN** each opens its own conversation, rather than sharing the empty default signature

#### Scenario: Explicit labels override the lane default
- **WHEN** a source declares `signatureLabels` and receives `kind: task` signals sharing those label values
- **THEN** they group under that signature — an operator who asks for grouping gets it in every lane

#### Scenario: Suppression record stays bounded
- **WHEN** recorded suppression entries age past their cooldown window
- **THEN** they are pruned from the `SignalSource` rather than accumulating
