## ADDED Requirements

### Requirement: The end-to-end tiers report through the always-present check, and own only the cluster-based jobs
The pull-request tier of the end-to-end pack — contract conformance and the
k3s smoke on the stub runtime — SHALL be a job of the `ci` workflow, reached
by CALLING `e2e.yml` as a reusable workflow, and SHALL be listed in
`ci-green`'s `needs:`. It SHALL NOT be a separately required status check:
branch protection names one check, and adding a gate is a line in that job's
`needs:`.

The tier SHALL run only when the diff touches what a running install is built
from — a component group, the chart, the test doubles under `test/`, or the
e2e workflow itself — and a job that did not run reports through `ci-green` as
skipped, exactly as every other path-filtered job does.

The boundary with the `end-to-end-testing` capability is stated on both sides:
`continuous-integration` owns per-module build, vet and unit test, the envtest
suite, chart lint and render, the image builds and scans, the site and the
guards; `end-to-end-testing` owns the cluster-based jobs and the conformance
suite. Neither restates the other's jobs, so "what CI runs" has exactly one
definition per tier.

#### Scenario: The pull-request tier gates through ci-green
- **WHEN** a pull request changes a component, the chart or the test doubles
- **THEN** the `e2e` job runs the pr tier, and `ci-green` fails if it failed

#### Scenario: A docs-only pull request runs no cluster
- **WHEN** a pull request changes only pages under `docs/`
- **THEN** the `e2e` job is skipped and `ci-green` treats it as skipped, not failed

#### Scenario: A per-module job is not duplicated in the e2e workflow
- **WHEN** a change adds a per-module build, vet or lint step
- **THEN** it is specified and wired under `continuous-integration`, and `e2e.yml` carries no copy of it
