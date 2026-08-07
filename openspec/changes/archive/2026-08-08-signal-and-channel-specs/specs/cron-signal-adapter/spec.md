# cron-signal-adapter — delta

## ADDED Requirements

### Requirement: Reference SignalAdapter declares the cron config schema
The reference cron `SignalAdapter` CR (in `config/samples/`) SHALL declare the config contract on its spec: a JSON Schema for `spec.config` declaring `schedule` (string, required — five-field cron expression), `input` (string, required), and `title` (string, optional), with no credential keys. The declaration SHALL live beside the `image` reference so schema and image update together. The adapter binary SHALL be unchanged.

#### Scenario: Declaration matches the parser
- **WHEN** the sample cron SignalAdapter is applied
- **THEN** its spec declares `schedule` and `input` as required and `title` as optional, discoverable via `kubectl get signaladapter cron -o yaml`

#### Scenario: Missing input flagged manager-side
- **WHEN** a `type: cron` SignalSource is created with only `config.schedule` while the declaring SignalAdapter exists
- **THEN** the source gains `ConfigValid=False` naming `input`, while the adapter's own Ready reporting remains authoritative
