# code-quality-analysis Specification

## Purpose
Static analysis and test coverage for every component, reported per component
on the pull request that changed it, from the tests CI already runs.

## Requirements

### Requirement: Every component is analysed under a project named for it

CI SHALL submit a static analysis of each component that a pull request or a
push to `master` touched, and each component SHALL be analysed under its OWN
project whose name is the component's published image name. The component set
SHALL be derived from the same list that names the images, never enumerated in
the workflow, so a new component is analysed once it exists.

A component holding more than one language SHALL still be one project.

#### Scenario: A pull request touches one component

- **WHEN** a pull request changes files under exactly one component directory
- **THEN** that component is analysed under its project
- **AND** no other component is analysed

#### Scenario: A workflow-wide path changes

- **WHEN** a pull request changes a path that rebuilds every component
- **THEN** every component is analysed

#### Scenario: A component is added

- **WHEN** a directory that qualifies as a component appears in the tree
- **THEN** the next run that touches it analyses it, with no edit to the
  workflow's component list

#### Scenario: A component holds two languages

- **WHEN** the console, a Go module embedding a browser application, is analysed
- **THEN** both languages' findings and coverage appear under one project

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

### Requirement: Generated and vendored code is not analysed, and tests are analysed as tests

Generated files, vendored dependencies and build output SHALL be excluded from
analysis. Test files SHALL be declared as tests rather than excluded, so
test-specific rules apply and test code is not counted as source.

#### Scenario: Generated deepcopy

- **WHEN** the operator is analysed
- **THEN** no finding is reported against a controller-gen generated file

#### Scenario: A Go test file

- **WHEN** a component is analysed
- **THEN** its `_test.go` files are classified as tests, not as source lines
  missing coverage

### Requirement: A pull request from a fork is analysed by nothing, visibly

A pull request whose head is a fork SHALL skip the analysis step while its
tests still run, and the skip SHALL be visible as a skipped step rather than a
failed one. The credential
that submits an analysis SHALL never be exposed to code from a fork.

#### Scenario: A fork opens a pull request

- **WHEN** a pull request from a fork touches a component
- **THEN** that component's tests run and its analysis step is skipped
- **AND** the always-present check passes
- **AND** no analysis credential is available to the run

### Requirement: The analysis service performs no analysis of its own

Only CI-submitted analyses SHALL exist for a project. The service's own
repository-side analysis SHALL be disabled on every project, so one analysis
per commit exists and it is the one carrying coverage.

#### Scenario: A commit is pushed to master

- **WHEN** a push to `master` touches a component
- **THEN** exactly one analysis of that commit exists for that component, and
  it carries coverage

### Requirement: A missing project is created inside the monorepo binding, never by a submission

The analysis step SHALL verify that the component's project exists before
submitting. When it does not, the step SHALL create it through the same
deliberate provisioning path a person would use — inside the repository's
monorepo binding, so it is bound to the repository and decorates pull
requests — and SHALL verify it exists before submitting. A project SHALL
never be created by the submission itself: one created that way is bound to
no repository and decorates nothing, while being indistinguishable from one
set up deliberately. Provisioning SHALL accept only names the repository
owns — its components and its named non-image units — so a mistyped name
creates nothing.

#### Scenario: A new component has no project yet

- **WHEN** a component directory is added and no project exists for it
- **THEN** the job that tested it provisions the project inside the monorepo
  binding, says so, and analyses the component under it

#### Scenario: Provisioning did not produce the project

- **WHEN** the provisioning call returns and the project still does not exist
- **THEN** the job fails naming the component and the provisioning script,
  and nothing is submitted

#### Scenario: A project is created deliberately

- **WHEN** the provisioning step is run for the repository
- **THEN** every component has a project bound to the repository's monorepo,
  and running it again creates nothing new

### Requirement: A failed submission fails the pull request, and so does a failed quality gate

A scanner that did not run or could not submit its analysis SHALL fail the
job that tested the component, and that job SHALL report through the
always-present check. The quality gate's verdict on a submitted analysis SHALL
be a check the pull request cannot merge without, reported through the same
always-present check, so that a component's gate failing holds the merge
whether or not anyone reads the service.

The verdict was measured before it was required — the first change that
submitted analyses deferred the gate until the counts were known. It is
required now because it is the termination signal of the automatic fixing loop:
a loop that ended while the gate was red would end with the merge blocked by
the one check it never read.

#### Scenario: The scanner cannot submit

- **WHEN** the analysis cannot be submitted — a scanner error, an unreachable
  service, a rejected token
- **THEN** the job that tested the component fails, and the always-present
  check fails naming it

#### Scenario: The quality gate fails

- **WHEN** the submitted analysis fails the project's quality gate
- **THEN** the verdict is visible on the pull request
- **AND** the always-present check fails naming the component

#### Scenario: The quality gate passes on every analysed component

- **WHEN** every component the pull request touched passes its gate
- **THEN** the always-present check is unaffected by the analysis

### Requirement: The workflow's own scripts are analysed as one unit

The scripts the workflow depends on SHALL be analysed under their own project,
named by the same pattern as a component's but stated explicitly, because the
unit publishes no image and so is absent from the image list. The analysis
SHALL include coverage produced by the script suite's own run — including the
lines reached in processes the suite spawns, since the suite drives the
scripts as subprocesses — and no script test SHALL run a second time to obtain
it. The unit SHALL be analysed on a pull request that changes the scripts,
their tests, or the hooks the suite exercises, and on every path that
re-analyses every component. The job analysing it SHALL report through the
always-present check, and the documentation gate's job SHALL no longer run the
suite.

#### Scenario: A pull request changes a script

- **WHEN** a pull request changes a file under the scripts directory, the
  script tests, or the hooks the suite exercises
- **THEN** the script suite runs once, and the unit is analysed with the
  coverage that run produced

#### Scenario: A script is exercised only through a subprocess

- **WHEN** a script is run by a test only as a child process of the shell
  driving it
- **THEN** the lines that process executed are reported as covered

#### Scenario: A pull request touches no script

- **WHEN** a pull request changes nothing under the scripts, their tests, the
  hooks, or a path that re-analyses everything
- **THEN** the scripts job is skipped, and the always-present check treats it
  as passed

#### Scenario: The project is missing

- **WHEN** the unit's project does not exist on the analysis service
- **THEN** the job creates it inside the repository's monorepo binding before
  analysing — bound to the repository, so it decorates pull requests — and
  never leaves it to the scanner's own provisioning, which binds to nothing

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
