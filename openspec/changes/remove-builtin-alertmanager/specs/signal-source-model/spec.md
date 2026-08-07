# signal-source-model — delta

## REMOVED Requirements

### Requirement: Built-in Alertmanager webhook remains in-process and unchanged
**Reason**: User decision — the manager hosts no signal transports; the `signal-vmalertmanager` adapter accepts the identical Alertmanager webhook format, and two implementations of one format meant duplicate sources and special cases.
**Migration**: Replace `type: alertmanagerWebhook` sources with `type: vmAlertmanagerWebhook` served by the adapter (vm-bundle or a plain SignalAdapter CR); repoint senders from `/ingest/alertmanager/{source}` to the adapter Service's `/webhook/{source}` BEFORE upgrading (senders' retries cover the switch itself).

## MODIFIED Requirements

### Requirement: Grouping policy stays manager-side for every source type
Signature grouping, fingerprint cooldown, window-based conversation reuse, and recurrence-on-session SHALL be applied by the manager from `spec.grouping` for signals of every source type. Adapters SHALL NOT need to implement any grouping logic.

#### Scenario: Adapter-fed signals group manager-side
- **WHEN** two normalized signals with different fingerprints but identical signature labels arrive via an adapter within the source's window
- **THEN** they land in the same conversation, the second as a recurrence when a session exists

#### Scenario: Cooldown suppresses adapter-fed duplicates
- **WHEN** an adapter re-delivers a signal with a fingerprint seen within `cooldownHours`
- **THEN** no new input is created (at-least-once delivery is safe)

#### Scenario: Same-signature batch collapses to one input
- **WHEN** one inbound batch carries several fresh signals sharing a signature
- **THEN** they land as ONE combined input on one conversation
