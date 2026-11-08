## ADDED Requirements

### Requirement: The state server credential is generated once
The state server's credential lives in the Secret `agentops-nats-credentials`
and is supplied by the chart values `nats.auth.user` and `nats.auth.password`
or by `nats.auth.existingSecret`. It SHALL be resolved in this order: an explicitly
configured value, then an existing Secret, then generation. Generation SHALL
happen on install only.

A render without cluster access finds no existing Secret, so the second step
yields nothing. On an upgrade the third step is then skipped as well, and the
Secret body is not rendered. A render without a cluster, such as a GitOps
sync of an upgrade, SHALL NOT mint a new one.

#### Scenario: An install generates the credential
- **WHEN** the chart is rendered as an install, with or without cluster access
- **THEN** a credential is generated, as for any chart-managed credential

#### Scenario: An upgrade keeps the credential
- **WHEN** the chart is rendered as an upgrade without a cluster
- **THEN** the credential Secret body is not rendered with a new value

#### Scenario: An explicit value wins
- **WHEN** the adopter supplies the credential
- **THEN** it is used and nothing is generated

#### Scenario: An upgrade that finds no Secret fails loudly
- **WHEN** the chart is rendered as an upgrade with cluster access, the state
  server Secret does not exist and no explicit value is set
- **THEN** the render fails, naming `agentops-nats-credentials` and `nats.auth.existingSecret`,
  and generates nothing
