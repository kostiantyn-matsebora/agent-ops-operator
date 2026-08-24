## MODIFIED Requirements

### Requirement: The HA bundle ships as a self-gated subchart, off by default
The bundle SHALL be named for the system it integrates — `home-assistant` — and
SHALL NOT carry a `-bundle` suffix, matching `kubernetes`, `prometheus` and
`telegram`.

The abbreviation `ha` SHALL NOT be the key: it collides in reading with the
`ha-ops` and `ha-control` PIPELINE names the bundle itself ships, and it is
ambiguous with high availability.

It SHALL remain self-gated and off by default. **The retired key SHALL FAIL the
render**, naming the replacement, because Helm reports no unread values key and
the quiet outcome is a bundle that simply does not render.

#### Scenario: The bundle is named for the house
- **WHEN** an install enables the Home Assistant bundle
- **THEN** it does so under a key naming Home Assistant, with no suffix

#### Scenario: The retired key is refused
- **WHEN** a values file supplies the retired suffixed key
- **THEN** the render fails naming that key and its replacement

#### Scenario: Default install renders nothing
- **WHEN** the chart renders with default values
- **THEN** no object from this bundle appears, while a runtime answering to the default name still renders because it is not the bundle's

#### Scenario: No substrate, no secrets
- **WHEN** the bundle is enabled with every component on
- **THEN** the output contains no `AgentRuntime`, no runtime defaults, no floor account and no `Secret` of any kind, while each route it ships carries its own ServiceAccount bound to nothing
