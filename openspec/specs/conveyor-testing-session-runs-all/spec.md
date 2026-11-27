# conveyor-testing-session-runs-all Specification

## Purpose
TBD - created by archiving change conveyor-testing-all-tiers-in-session. Update Purpose after archive.

## Requirements

### Requirement: Every tier runs in the session, and CI is never the system tier's runner

The testing station SHALL run the unit, system and e2e tiers inside the remote session and
read their results itself. It SHALL NOT dispatch a CI workflow to run a tier. Where the
session's environment cannot run a tier, the station SHALL stop and report what is missing.

#### Scenario: The system tier on a cloud session
- **WHEN** the station runs on a cloud session
- **THEN** the system pack runs in that session and writes its own coverage file
- **AND** no `system-full.yml` or `system-smoke.yml` run is started

#### Scenario: The sandbox cannot start pods
- **WHEN** a trivial pod does not start in the session's sandbox
- **THEN** the station stops and names the failure
- **AND** it does not dispatch the tier to CI

### Requirement: A round re-runs only what failed

After the first round, a round SHALL re-run only the tests that failed in the last run and
the tests added since, and SHALL merge their coverage into the first round's profile. A tier
SHALL be judged passed only on a full run of that tier, run once at the end.

#### Scenario: One failing system lane
- **WHEN** round 1 ends with one system lane red
- **THEN** round 2 runs that lane against the cluster that is still up
- **AND** it does not run the other lanes

#### Scenario: A fix breaks an unrelated test
- **WHEN** every re-run passes and the closing full run fails a test no round touched
- **THEN** the tier is not passed and that test is work for the next round

#### Scenario: Coverage from several runs
- **WHEN** coverage is measured from the first round's profile and a later re-run's profile
- **THEN** a block counts as covered when any profile covers it
