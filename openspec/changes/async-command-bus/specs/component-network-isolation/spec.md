## ADDED Requirements

### Requirement: Only the manager may reach the command-bus server
When the chart renders a command-bus server, restriction SHALL admit ingress to
that server from the manager's pods alone, on the server's client port. No
other component the chart deploys, and no pod outside the installation, SHALL
be admitted.

When the install is given the URL of a server it does not render, no rule is
rendered for it, and the manager's egress to that URL stays the install's
decision.

The server's other ports, such as cluster and monitoring ports, SHALL be
admitted only to the pods of the server itself, and a metrics collector only
where the install permits one in values.

#### Scenario: The manager reaches the bus
- **WHEN** restriction is enabled and a manager replica dials the bus server's client port
- **THEN** the connection is admitted

#### Scenario: No other component reaches the bus
- **WHEN** restriction is enabled and an adapter, the console or a runtime pod dials the bus server
- **THEN** the connection does not reach it

#### Scenario: The bus's own replicas reach each other
- **WHEN** restriction is enabled and a bus server pod dials a peer on the cluster port
- **THEN** the connection is admitted

#### Scenario: Every wired flow still works
- **WHEN** restriction is enabled and a signal opens a conversation that answers on a bound channel
- **THEN** the work flows through the bus and the answer is delivered
