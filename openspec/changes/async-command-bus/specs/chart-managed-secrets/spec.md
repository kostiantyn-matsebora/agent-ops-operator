## ADDED Requirements

### Requirement: The command-bus credential is chart-generated and stable
The credential the manager presents to the command-bus server SHALL be one the
chart generates, with its own Secret and its own value. It SHALL NOT be shared
with the state store's credential or with any adapter token.

It SHALL follow the same three sources in the same order as every other
generated credential: an explicitly configured value, an existing Secret, then
generation. An operator MAY instead supply a whole Secret by name.

The Secret SHALL be rendered on install only when nothing is configured, SHALL
carry the retention policy, and SHALL NOT be rendered on an upgrade.

The upgrade guard SHALL refuse an upgrade that would orphan it without the
policy. An install over a retained Secret SHALL adopt the existing value.

When the install is given the URL of a server, it SHALL also be given the
credential for it by one of the same three sources, and the chart SHALL
generate none.

#### Scenario: An upgrade keeps the bus credential
- **WHEN** an unchanged install is upgraded or rendered by a tool with no cluster access
- **THEN** the bus credential is not rendered as changed and the manager still connects

#### Scenario: A pinned bus credential wins
- **WHEN** an operator configures the bus credential explicitly on an install that already has a generated one
- **THEN** the configured value is what the Secret holds after the upgrade, and the manager and the server both use it

#### Scenario: The credential is not shared
- **WHEN** the state store's credential is rotated
- **THEN** the command-bus credential is unchanged

#### Scenario: The notes name the source in effect
- **WHEN** the bus credential was configured explicitly and the install is upgraded
- **THEN** the notes name the configured source and print no command to read a generated value
