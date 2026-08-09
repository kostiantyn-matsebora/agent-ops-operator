## MODIFIED Requirements

### Requirement: Grouping policy stays manager-side for every source type
Signature grouping, fingerprint cooldown, window-based conversation reuse, and recurrence-on-session SHALL be applied by the manager from `spec.grouping` for signals of every source type. Adapters SHALL NOT need to implement any grouping logic.

When a source declares `signatureLabels`, they SHALL compose the signature for signals of every kind. When a source declares NONE, the fallback SHALL depend on what the lane is about: `alert` and `job` are recurring-subject lanes, where a later signal is more news about the same thing, and SHALL fall back to the default alert labels (`alertgroup`/`alertname`/`namespace`) so they group and resume an existing session; `task` and `chat` are one-shot lanes, where a later signal is a separate request, and SHALL key on the signal's own fingerprint so each opens its own conversation. The default labels are alert vocabulary, so applying them to a one-shot lane would hash every request to one empty signature and pile unrelated work into a single conversation.

#### Scenario: Cooldown suppresses adapter-fed duplicates
- **WHEN** an adapter re-delivers a signal with a fingerprint seen within `cooldownHours`
- **THEN** no new input is created (at-least-once delivery is safe)

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
