## MODIFIED Requirements

### Requirement: Firing alerts normalize into the signal contract, resolved alerts do not
Matching `signals/alertmanager`'s firing-only parity, the adapter SHALL
forward only Pulse's `"alert"` (firing) events as normalized signals.

`"resolved"` events SHALL be accepted, so Pulse sees a 2xx and does not
retry, but SHALL produce no signal.

The `initiationId` SHALL be derived from the Pulse alert id carried in the
registered template's fields — the resource identity joined with the alert
identity — using one of two strategies selected by the served source's
`spec.config.correlationStrategy`:

- `"id+startTime"` (the default, used when the field is absent, empty, or
  any value other than `"id"`): the resource, the alert id, AND the
  alert's start time. Pulse's own alert id can be reused when the same
  condition fires again later as a new occurrence, so start time SHALL be
  included to keep a later re-fire from colliding with an earlier,
  unrelated occurrence that happened to reuse the same alert id.
- `"id"`: the resource and the alert id alone, with no time component —
  today's behavior, kept for a source that needs to opt back into it.

Either strategy SHALL yield distinct ids for two unrelated alerts (never
sharing an id), and SHALL yield the SAME id for retries and for a
firing/resolved pair of one occurrence.

The no-ID fallback — used only when Pulse sends no alert id at all, a hash
of resource/type/severity/message — is unaffected by `correlationStrategy`
and SHALL continue to omit any time component.

Cooldown and routing stay manager-side, keyed on the resulting id.

#### Scenario: A firing event produces one signal
- **WHEN** a webhook body carries an `"alert"` event
- **THEN** exactly one normalized signal is pushed, its `initiationId` derived from the resource and alert identity per the source's `correlationStrategy`

#### Scenario: A resolved event produces no signal
- **WHEN** a webhook body carries a `"resolved"` event
- **THEN** the adapter responds 200 and pushes no signal

#### Scenario: Unrelated Pulse alerts never merge
- **WHEN** a node-memory threshold alert and an unreachable-device incident both fire
- **THEN** their signals carry distinct `initiationId` values and open distinct conversations

#### Scenario: Default strategy distinguishes a re-fire from a continuation
- **WHEN** a source sets no `correlationStrategy`, and the same Pulse alert id fires, resolves, and later fires again with a new start time
- **THEN** the firing/resolved pair shares one `initiationId`, and the later re-fire carries a different `initiationId` that opens a new conversation

#### Scenario: Explicit "id" strategy keeps today's collapsing behavior
- **WHEN** a source sets `spec.config.correlationStrategy: "id"`, and the same Pulse alert id fires, resolves, and later fires again with a new start time
- **THEN** every one of those signals carries the same `initiationId`

#### Scenario: An unrecognized strategy value falls back to the default
- **WHEN** a source sets `spec.config.correlationStrategy` to a value other than `"id"` or `"id+startTime"`
- **THEN** the adapter derives `initiationId` as if `"id+startTime"` had been set

#### Scenario: The no-ID fallback ignores correlationStrategy
- **WHEN** a webhook body carries no Pulse alert id, regardless of the source's `correlationStrategy`
- **THEN** `initiationId` is the existing hash of resource/type/severity/message, with no start time included
