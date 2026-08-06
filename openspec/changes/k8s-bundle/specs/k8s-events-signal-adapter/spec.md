# k8s-events-signal-adapter

## ADDED Requirements

### Requirement: Kubernetes events run as a signal adapter
A signal adapter in `signal-k8s-events/` (own dependency-free Go module and image, precedents `signal-cron/` and `channel-telegram/`) SHALL serve SignalSources with `spec.type: k8sEvents` through the signal adapter contract, watching core `v1` Events via the in-cluster Kubernetes API using its own ServiceAccount token (no client-go). Per-source `config` SHALL support `severities` (Event `type` values, default `["Warning"]`), `namespaces` (empty or absent = all namespaces the granted RBAC allows), and optional `includeReasons`/`excludeReasons` exact-match filters. Invalid config (unknown severity value or malformed shape) SHALL be reported as a False Ready condition on that source via the contract status API while other sources keep being served. A `SourceK8sEvents = "k8sEvents"` constant SHALL name the type in `api/v1alpha1`; the type is adapter-served, not in-process.

#### Scenario: Warning events flow once the adapter serves a source
- **WHEN** a `type: k8sEvents` SignalSource with empty `config` is served and a pod in the cluster emits a `Warning` event (e.g. `BackOff`)
- **THEN** a normalized signal for that event is posted to `/signal/inbound` and a conversation is created through the manager's ingest pipeline

#### Scenario: Severity filter honors configuration
- **WHEN** a source's `config.severities` is `["Normal", "Warning"]`
- **THEN** events of both types produce signals, while a source with default config produces signals only for `Warning` events

#### Scenario: Invalid severity surfaces on the source
- **WHEN** a source's `config.severities` contains a value that is not a valid Event type
- **THEN** the adapter sets a False Ready condition naming the problem and other sources keep producing signals

### Requirement: Events normalize with stable fingerprints for manager-side grouping
The adapter SHALL emit `kind: alert` signals with a deterministic fingerprint `<source>@<namespace>/<involvedObject.kind>/<involvedObject.name>/<reason>` (stable across restarts and event-object recreation) and labels covering at least `alertgroup: k8sEvents`, `alertname: <reason>`, `namespace`, `kind`, `name`, and `severity`, so `SignalSource.spec.grouping` can group by object, reason, or namespace. The adapter SHALL NOT deduplicate, group, or apply cooldown beyond its restart cursor — repetition collapses manager-side.

#### Scenario: Crash-loop repeats collapse into one conversation
- **WHEN** the same pod emits repeated `BackOff` warning events within the source's cooldown and grouping window
- **THEN** all occurrences carry the same fingerprint and land in (or are suppressed into) the existing conversation rather than spawning new ones

#### Scenario: Labels support object-level grouping
- **WHEN** a source's `grouping.signatureLabels` is `["namespace", "kind", "name"]`
- **THEN** distinct failing objects produce distinct conversations while different reasons on one object share a conversation

### Requirement: Watching is restart-safe
The adapter SHALL persist a per-source cursor (the maximum event `lastTimestamp` observed) through the contract state API and skip already-seen events on startup's initial list; on watch expiry (`410 Gone`) or stream error it SHALL relist and resume. Delivery is at-least-once: duplicates after an ill-timed restart are acceptable because deterministic fingerprints collapse under the manager's cooldown.

#### Scenario: Restart does not replay old events
- **WHEN** the adapter restarts and the initial Events list returns events at or before the persisted cursor
- **THEN** no signals are emitted for those events

#### Scenario: Watch expiry recovers without losing new events
- **WHEN** the API server ends the watch with `410 Gone`
- **THEN** the adapter relists, resumes watching, and events emitted after the expiry still produce signals
