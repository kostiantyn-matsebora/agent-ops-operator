## ADDED Requirements

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
  analysing, exactly as for a component (the MODIFIED requirement below) —
  never through the scanner's own provisioning, which binds to nothing

## MODIFIED Requirements

### Requirement: A component without a project fails its analysis job rather than creating one

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
