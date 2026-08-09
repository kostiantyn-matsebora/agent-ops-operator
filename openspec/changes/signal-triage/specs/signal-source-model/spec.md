# signal-source-model

## MODIFIED Requirements

### Requirement: Grouping policy stays manager-side for every source type
Signature grouping, fingerprint cooldown, window-based conversation reuse, recurrence-on-session, the Conversation creation cap, and the deduplication decision SHALL be applied by the manager from `spec.grouping` for signals of every source type. Adapters SHALL NOT need to implement any grouping, capping, or deduplication logic.

`spec.grouping` SHALL therefore carry, in addition to `signatureLabels`, `windowDays`, and `cooldownHours`: the maximum NEW Conversations the source may create per window (unset = no cap), and the opt-in for agent-decided triage (default off).

`SignalSourceStatus` SHALL carry a `Throttled` condition reporting refused creations, and a bounded record of recent triage verdicts including the reason for every drop.

#### Scenario: Cooldown suppresses adapter-fed duplicates
- **WHEN** an adapter re-delivers a signal with a fingerprint seen within `cooldownHours`
- **THEN** no new input is created (at-least-once delivery is safe)

#### Scenario: Adapter-fed signals group manager-side
- **WHEN** two normalized signals with different fingerprints but identical signature labels arrive via an adapter within the source's window
- **THEN** they land in the same conversation, the second as a recurrence when a session exists

#### Scenario: Same-signature batch collapses to one input
- **WHEN** one inbound batch carries several fresh signals sharing a signature
- **THEN** they land as ONE combined input on one conversation

#### Scenario: The cap and triage apply to every source type
- **WHEN** a cap or triage opt-in is set on any source, whatever its adapter
- **THEN** the manager enforces it without the serving adapter implementing anything

#### Scenario: Suppression is visible on the source
- **WHEN** creations are refused by the cap, or signals are dropped by a triage verdict
- **THEN** the source's status reports the throttling and retains the recent verdicts with their reasons
