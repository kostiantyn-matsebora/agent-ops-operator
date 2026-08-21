## ADDED Requirements

### Requirement: Context operations are recorded as activity

Restoring, checkpointing, skipping and failing a context synchronisation SHALL
each be emitted as an activity event against the conversation they belong to,
carrying at least the duration and the volume of data transferred.

They SHALL be emitted through the existing activity log rather than through a
separate telemetry path, so that the console's per-conversation view and the
metrics registry continue to derive from ONE instrumentation pass. Adding a
second path would allow a metric and its event to disagree, which is precisely
what the single-observer arrangement exists to prevent.

A skipped checkpoint SHALL be emitted, because "nothing changed" and "nothing
ran" are different facts and an operator diagnosing a stale context needs to
tell them apart.

The synchronising process runs in the runtime pod rather than in the manager, so
it SHALL report its operations to the manager, which emits them.

#### Scenario: A checkpoint is visible per conversation

- **WHEN** a conversation's context is checkpointed
- **THEN** the event appears in that conversation's activity, with its duration and size, and the corresponding metric is observed

#### Scenario: A skip is distinguishable from silence

- **WHEN** a periodic checkpoint is skipped because nothing changed
- **THEN** an event records the skip, rather than the interval passing with no trace

#### Scenario: A failure is recorded

- **WHEN** a checkpoint or restore fails
- **THEN** the failure is recorded as activity against the conversation with its reason

### Requirement: The latest checkpoint is durable state, not telemetry

The fact of a conversation's most recent successful checkpoint — when it
happened, which copy it produced, and whether it was taken at a work boundary —
SHALL be recorded on the conversation itself, not only in the activity log.

The activity log is bounded and lossy by design. Whether a conversation has a
usable durable context decides whether continuity is possible after a crash, so
it cannot depend on a record that may have been evicted.

This SHALL be written ONLY when a checkpoint actually transferred data. A
skipped checkpoint SHALL NOT write it. Recording every skip would patch every
conversation on every interval indefinitely, which is the write amplification
that suppressed signals are already required to avoid.

#### Scenario: The conversation knows its own context is safe

- **WHEN** a conversation's context is checkpointed
- **THEN** the conversation records the time, the copy and whether it was quiesced

#### Scenario: A skip costs no write

- **WHEN** a periodic checkpoint is skipped because nothing changed
- **THEN** the conversation is not patched
