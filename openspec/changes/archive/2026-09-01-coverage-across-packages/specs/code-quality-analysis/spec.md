## MODIFIED Requirements

### Requirement: Coverage comes from the tests CI already runs

The analysis SHALL include line coverage produced by the same test invocation
the pull request's test job ran, and no test suite SHALL be run a second time
to obtain it. Coverage SHALL be attributed to every package a test exercises,
not only to the package the test belongs to, so a package driven by another
package's suite reports the coverage it actually has. A component whose tests
did not run, or which has no tests, SHALL still be analysed, reporting no
coverage figure rather than failing.

#### Scenario: The operator's coverage reaches the analysis

- **WHEN** the operator's test job ran its envtest suite
- **THEN** the operator's analysis reports the coverage that run produced
- **AND** the envtest suite ran exactly once in that workflow run

#### Scenario: A package is exercised only by another package's tests

- **WHEN** a package holds no tests of its own and a suite elsewhere in the
  module drives it
- **THEN** the analysis reports that package's lines as covered where the
  suite reached them, not as uncovered

#### Scenario: A component without tests

- **WHEN** a component with no test suite is analysed
- **THEN** the analysis is submitted and the coverage figure is absent or zero,
  and the job does not fail

## ADDED Requirements

### Requirement: Every project is held to 80% coverage of the whole component

Every component's project SHALL be assigned a quality gate that requires the
component's OVERALL line coverage to be at least 80%, in addition to the
new-code conditions of the service's built-in gate. The gate SHALL be
created, conditioned and assigned by the same deliberate provisioning step
that creates the projects, idempotently, so that it is reproducible from the
repository and never set by hand.

The gate's verdict remains the service's own check and is NOT a check branch
protection requires — that is the existing requirement, unchanged. A
component under the threshold is visibly failing its gate on its dashboard
and on every pull request that touches it.

**A gate on new code alone lets a component sit at any coverage forever.** A
component at 27% and one at 79% passed identically; the number the tree is
asked to reach has to be a condition something evaluates.

#### Scenario: The provisioning step is run

- **WHEN** the provisioning step runs against the organisation
- **THEN** a gate exists carrying the built-in new-code conditions and an
  overall-coverage condition at 80%, it is the organisation's default, and
  every component project is assigned to it

#### Scenario: The provisioning step is run again

- **WHEN** the step runs a second time
- **THEN** no second gate and no duplicate condition is created

#### Scenario: A component is under the threshold

- **WHEN** a component's overall coverage is below 80%
- **THEN** its project's gate fails, the verdict is visible on the pull
  request, and the always-present check is unaffected

#### Scenario: A component reaches the threshold

- **WHEN** a push brings a component's overall coverage to 80% or more with
  its new code meeting the new-code conditions
- **THEN** its project's gate passes
