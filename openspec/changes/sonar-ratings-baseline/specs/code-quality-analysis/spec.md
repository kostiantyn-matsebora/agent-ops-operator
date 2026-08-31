## ADDED Requirements

### Requirement: Every project is held to at least a B rating, overall, for reliability, security and maintainability

Every component's project SHALL be assigned a quality gate that requires the
component's OVERALL reliability rating, OVERALL security rating and OVERALL
maintainability rating to each be A or B, in addition to the new-code and
overall-coverage conditions already on the gate. The gate SHALL be created,
conditioned and assigned by the same deliberate provisioning step that
creates the other conditions, idempotently, so that it is reproducible from
the repository and never set by hand.

The gate's verdict remains the service's own check and is NOT a check branch
protection requires — unchanged from every earlier condition on this gate. A
component whose overall rating is worse than B is visibly failing its gate on
its dashboard and on every pull request that touches it.

**A gate on new code alone lets an existing Blocker or High finding sit in the
tree forever.** New-code ratings judge only what a pull request adds; nothing
before this evaluates the backlog a repository already carries.

#### Scenario: The provisioning step is run

- **WHEN** the provisioning step runs against the organisation
- **THEN** the gate carries reliability, security and maintainability rating
  conditions on the OVERALL component, each failing worse than B, alongside
  its existing conditions

#### Scenario: The provisioning step is run again

- **WHEN** the step runs a second time
- **THEN** no duplicate condition is created and an existing rating threshold
  is left as it is unless it no longer matches B

#### Scenario: A component's overall rating is worse than B

- **WHEN** a component carries an open Blocker or High finding that lowers its
  overall reliability, security or maintainability rating below B
- **THEN** its project's gate fails on that condition, the verdict is visible
  on the pull request, and the always-present check is unaffected

#### Scenario: A component's overall rating reaches B

- **WHEN** every open finding that lowered a component's overall rating below
  B is fixed or resolved
- **THEN** the corresponding rating condition passes on the project's next
  analysis
