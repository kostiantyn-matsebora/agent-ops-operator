## ADDED Requirements

### Requirement: The signal card draws its severity
The transcript's signal card SHALL draw the `signal` message's `severity`
field, where present.

- The severity leads the card with its value, tinted per value from the
  theme's existing status tokens: `critical` and `error` use `--ao-danger`,
  `warning` uses `--ao-warning`, `info` uses `--ao-neutral`. No new theme
  token is added.
- A card whose message carries no severity SHALL render exactly as today.

#### Scenario: A critical alert
- **WHEN** a `critical` alert opens a conversation
- **THEN** the card leads with the critical mark and value

#### Scenario: A task card is unchanged
- **WHEN** a `kind: task` signal with no severity opens a conversation
- **THEN** the card renders as before
