## MODIFIED Requirements

### Requirement: Alerts normalize with built-in-path parity
The adapter SHALL drop non-`firing` alerts and, per firing alert, emit one signal:

- the Alertmanager `fingerprint` verbatim, or — when absent — a deterministic fallback derived by hashing the sorted label pairs (empty fingerprints are rejected by `/signal/inbound`)
- the raw alert label map as `labels`, so a rule's `type` label reaches routing unchanged
- title `"🔍 " + alertname` plus ` — <namespace>` when that label is present
- a per-alert JSON payload carrying labels, annotations, `startsAt`, and `generatorURL`
- no `kind` (alert lane)
- the `severity` FIELD mapped from the alert's `severity` label through the source's `config.severityMap`

The map is `{<label value>: <vocabulary value>}`. Absent, it is the identity for `critical`, `warning` and `info`. A label value the map does not cover SHALL leave the signal unrated and SHALL be reported on the source's Ready condition message, once per value, without failing the source.

The adapter SHALL NOT group, deduplicate, or apply cooldown — signature grouping, fingerprint cooldown, window reuse, and recurrence stay manager-side from `SignalSource.spec.grouping`.

#### Scenario: Only firing alerts produce signals
- **WHEN** a webhook body contains two `firing` and one `resolved` alert
- **THEN** exactly two signals are pushed and the response reports what was queued

#### Scenario: Missing fingerprint gets a stable fallback
- **WHEN** an alert arrives without a `fingerprint` field twice with identical labels
- **THEN** both posts carry the same derived fingerprint and the manager's cooldown collapses the repeat

#### Scenario: Grouping matches the built-in path
- **WHEN** two firing alerts share values for the source's `signatureLabels`
- **THEN** they land in the same conversation, exactly as the same alerts would via the built-in endpoint

#### Scenario: The Prometheus convention maps by default
- **WHEN** a source declares no `severityMap` and an alert carries `severity: critical`
- **THEN** the signal's severity field is `critical`

#### Scenario: A house scale maps through the source
- **WHEN** a source declares `severityMap: {page: critical, ticket: warning}` and an alert carries `severity: page`
- **THEN** the signal's severity field is `critical`

#### Scenario: An uncovered value is reported, not refused
- **WHEN** an alert carries `severity: sev1` and no map covers it
- **THEN** the signal is sent unrated, and the source's Ready condition message names `sev1` as unmapped while Ready stays True
