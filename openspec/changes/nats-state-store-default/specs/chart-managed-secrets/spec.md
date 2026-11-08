## ADDED Requirements

### Requirement: The state server credential is generated once
The state server's credential SHALL follow the three-source order and SHALL
be generated on install only. A render without a cluster, such as a GitOps
sync of an upgrade, SHALL NOT mint a new one.

#### Scenario: An upgrade keeps the credential
- **WHEN** the chart is rendered as an upgrade without a cluster
- **THEN** the credential Secret body is not rendered with a new value

#### Scenario: An explicit value wins
- **WHEN** the adopter supplies the credential
- **THEN** it is used and nothing is generated
