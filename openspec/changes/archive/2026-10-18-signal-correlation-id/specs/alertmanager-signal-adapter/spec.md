# alertmanager-signal-adapter — delta

## REMOVED Requirements

### Requirement: Alerts normalize with built-in-path parity

**Reason**: The per-alert `fingerprint` and manager-side signature grouping it normalized for are retired. Correlation is the sender's `group_by` decision, carried as `initiationId`.

**Migration**: Replaced by "Alerts normalize carrying the sender's group identity" in this change — the webhook `groupKey` becomes the id, cooldown and routing stay manager-side on it.

## ADDED Requirements

### Requirement: Alerts normalize carrying the sender's group identity
The adapter SHALL drop non-`firing` alerts and, per firing alert, emit one signal with:

- `initiationId`: the webhook's `groupKey` verbatim — Alertmanager's `group_by` is the grouping decision, made upstream, so every alert of one group correlates to one conversation. When the body carries no `groupKey`, a deterministic fallback derived by hashing the sorted `groupLabels` pairs (an empty id is rejected by `/signal/inbound`)
- the raw alert label map as `labels`
- title `"🔍 " + alertname` plus ` — <namespace>` when that label is present
- a per-alert JSON payload carrying labels, annotations, `startsAt`, and `generatorURL`
- no `kind` (alert lane)

The adapter SHALL NOT deduplicate, apply cooldown, or route. Which alerts are one problem is the sender's `group_by` decision, carried in the id — the dedup window stays manager-side cooldown on that id, and routing stays manager-side id equality.

#### Scenario: Only firing alerts produce signals
- **WHEN** a webhook body contains two `firing` and one `resolved` alert
- **THEN** exactly two signals are pushed and the response reports what was queued

#### Scenario: One group is one conversation
- **WHEN** two firing alerts of one Alertmanager group arrive, in one webhook or across two
- **THEN** every signal carries that group's `groupKey` as its `initiationId` and they land in one conversation

#### Scenario: Regrouping upstream re-routes
- **WHEN** `group_by` changes upstream so an alert arrives under a `groupKey` no open conversation carries
- **THEN** a new conversation is created adopting that id

#### Scenario: Missing groupKey gets a stable fallback
- **WHEN** a nonstandard sender posts twice without a `groupKey` and with identical `groupLabels`
- **THEN** both posts carry the same derived `initiationId` and the manager's cooldown collapses the repeat
