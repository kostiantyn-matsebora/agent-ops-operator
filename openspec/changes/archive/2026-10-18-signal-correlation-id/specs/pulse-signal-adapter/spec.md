# pulse-signal-adapter — delta

## MODIFIED Requirements

### Requirement: Firing alerts normalize into the signal contract, resolved alerts do not
Matching `signals/alertmanager`'s firing-only parity, the adapter SHALL
forward only Pulse's `"alert"` (firing) events as normalized signals.

`"resolved"` events SHALL be accepted, so Pulse sees a 2xx and does not
retry, but SHALL produce no signal.

The `initiationId` SHALL be the Pulse alert id carried in the registered
template's fields — the resource identity joined with the alert identity.
That is Pulse's own incident model doing the grouping, so two unrelated
alerts never share an id.

The same underlying Pulse alert occurrence SHALL always yield the same id,
across retries and across a firing/resolved pair.

Cooldown and routing stay manager-side, keyed on that id.

#### Scenario: A firing event produces one signal
- **WHEN** a webhook body carries an `"alert"` event
- **THEN** exactly one normalized signal is pushed, its `initiationId` the Pulse alert id from the resource and alert identity

#### Scenario: A resolved event produces no signal
- **WHEN** a webhook body carries a `"resolved"` event
- **THEN** the adapter responds 200 and pushes no signal

#### Scenario: Unrelated Pulse alerts never merge
- **WHEN** a node-memory threshold alert and an unreachable-device incident both fire
- **THEN** their signals carry distinct `initiationId` values and open distinct conversations
