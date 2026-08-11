## ADDED Requirements

### Requirement: The events component exposes maintenance windows as values
The bundle SHALL expose the events source's time intervals and mute references as chart values, so a recurring maintenance window is release configuration rather than a hand-edited CR that the next upgrade overwrites.

The shipped example SHALL name an IANA location rather than relying on the UTC default, because the value most likely to be copied unchanged is the one that must not be wrong.

#### Scenario: A maintenance window survives an upgrade
- **WHEN** an operator declares a nightly window in the bundle's values and upgrades the release
- **THEN** the rendered SignalSource carries the window, unchanged by the upgrade

#### Scenario: No window is configured by default
- **WHEN** the bundle renders with default values
- **THEN** the source declares no time intervals and nothing is muted
