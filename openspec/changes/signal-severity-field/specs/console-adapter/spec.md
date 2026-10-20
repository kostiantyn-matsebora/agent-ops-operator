## ADDED Requirements

### Requirement: The signal card draws severity when present

The transcript's signal card SHALL draw the `signal` message's `severity`
field where present, tinted from the theme's existing status tokens.

| Severity | Token |
|---|---|
| `critical` | `--ao-danger` |
| `error` | `--ao-danger` |
| `warning` | `--ao-warning` |
| `info` | `--ao-neutral` |

No new theme token SHALL be added for this. A card whose message carries
no `severity` SHALL render exactly as it does today.

#### Scenario: A critical signal is tinted

- **WHEN** a `signal` message carries `severity: critical`
- **THEN** the card draws the severity tinted with `--ao-danger`

#### Scenario: A warning signal is tinted

- **WHEN** a `signal` message carries `severity: warning`
- **THEN** the card draws the severity tinted with `--ao-warning`

#### Scenario: An unrated signal is unchanged

- **WHEN** a `signal` message carries no `severity`
- **THEN** the card renders exactly as it did before this requirement
  existed
