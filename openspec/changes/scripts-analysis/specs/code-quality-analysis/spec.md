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
- **THEN** the job fails naming the unit and the provisioning script, and
  creates nothing
