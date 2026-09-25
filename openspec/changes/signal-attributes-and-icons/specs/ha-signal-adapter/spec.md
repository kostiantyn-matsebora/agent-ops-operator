## ADDED Requirements

### Requirement: The log level maps to the severity field
Every signal the adapter emits SHALL carry the contract's `severity` field mapped from the record's level, and the mapping SHALL be fixed.

| Level | Severity |
|---|---|
| `CRITICAL` | `critical` |
| `ERROR` | `error` |
| `WARNING` | `warning` |
| `INFO`, `DEBUG` | `info` |

- The `level` label SHALL keep the record's own value for grouping and rules.
- A surface condition SHALL take the level its record was shaped with, so it maps by the same table.
- A registry issue's own `severity` SHALL stay in the payload as Home Assistant's fact and SHALL NOT set the field. The record's level does.

#### Scenario: An error record is rated error
- **WHEN** an `ERROR` record passes the rules and its dwell
- **THEN** the emitted signal carries `severity: error` and the label `level: ERROR`

#### Scenario: A config-entry failure is rated by its shaped level
- **WHEN** a config entry enters a failed state and the adapter shapes it as an `ERROR` record
- **THEN** the emitted signal carries `severity: error`
