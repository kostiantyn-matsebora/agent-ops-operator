## ADDED Requirements

### Requirement: The account a route inherits is a reference, and the floor stays nameable
Where a `Pipeline` names no service account it SHALL inherit the release's
DEFAULT account. That default SHALL be a REFERENCE the chart does not create:
naming is not creating, which is the posture adapters already have.

The chart SHALL always render its own FLOOR account, bound to nothing, whatever
the inherited default is. A route SHALL therefore be able to NAME the floor in
order to hold nothing — which is the only way to restrict one route on an
install whose inherited default is an account carrying rights.

There SHALL be no preset posture a route can select by naming a mode. An install
wanting more than nothing declares an account, or uses one a bundle renders for
its own routes.

#### Scenario: An unnamed route inherits the default
- **WHEN** a Pipeline names no service account
- **THEN** it runs as the release's default account

#### Scenario: A route restricts itself to nothing
- **WHEN** the inherited default carries rights and a Pipeline names the chart's floor account
- **THEN** that route holds no Kubernetes permissions

#### Scenario: The named default is never created
- **WHEN** the inherited default names an account the operator already owns
- **THEN** the chart references it and does not attempt to create it
