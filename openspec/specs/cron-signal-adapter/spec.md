# cron-signal-adapter

## Purpose

The reference signal adapter in `signal-cron/`: a dependency-free module that serves `adapter: cron` SignalSources through the signal adapter contract, parsing five-field schedules and firing job-lane signals with a restart-safe, idempotent cursor.

## Requirements

### Requirement: Cron runs as the reference signal adapter
A reference adapter in `signal-cron/` (own dependency-free Go module and image, precedents `channel-telegram/` and `runtime-claude/`) SHALL serve SignalSources with `spec.adapter: cron` through the signal adapter contract: parsing `config: {schedule, input, title?}` (five-field cron expression subset), reporting invalid config on the source's Ready condition while continuing to serve other sources, and firing a `kind: job` normalized signal with the configured input as payload each time a schedule elapses.

#### Scenario: Scheduled input fires as a job conversation
- **WHEN** a `adapter: cron` source with `schedule: "0 6 * * *"` and an input text is served and 06:00 passes
- **THEN** a `kind: job` signal is posted for that source and a conversation runs the input through the task-lane prompt

#### Scenario: Invalid schedule surfaces on the source
- **WHEN** a source's `config.schedule` is not a valid five-field expression
- **THEN** the adapter sets a False Ready condition naming the problem and other sources keep firing

### Requirement: Firing is restart-safe and idempotent
The adapter SHALL persist each source's last-fired tick through the contract's state API and derive fingerprints as `<source>@<scheduled-tick>`, so restarts never double-fire a tick (at-least-once delivery collapses under the manager's cooldown and the deterministic fingerprint) and missed ticks during downtime fire at most once on recovery.

#### Scenario: Restart does not re-fire
- **WHEN** the adapter fires the 06:00 tick, restarts at 06:02, and re-evaluates schedules
- **THEN** the 06:00 tick is not fired again

#### Scenario: Recurring runs share one remembering conversation
- **WHEN** a cron source fires on consecutive days within the grouping window
- **THEN** the ticks land in the same conversation, later ones resuming the agent session as recurrences

### Requirement: Reference SignalAdapter declares the cron config schema
The reference cron `SignalAdapter` CR (in `config/samples/`) SHALL declare the config contract on its spec: a JSON Schema for `spec.config` declaring `schedule` (string, required — five-field cron expression), `input` (string, required), and `title` (string, optional), with no credential keys. The declaration SHALL live beside the `image` reference so schema and image update together. The adapter binary SHALL be unchanged.

#### Scenario: Declaration matches the parser
- **WHEN** the sample cron SignalAdapter is applied
- **THEN** its spec declares `schedule` and `input` as required and `title` as optional, discoverable via `kubectl get signaladapter cron -o yaml`

#### Scenario: Missing input flagged manager-side
- **WHEN** a `type: cron` SignalSource is created with only `config.schedule` while the declaring SignalAdapter exists
- **THEN** the source gains `ConfigValid=False` naming `input`, while the adapter's own Ready reporting remains authoritative
