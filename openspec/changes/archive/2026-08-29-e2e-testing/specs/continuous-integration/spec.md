## ADDED Requirements

### Requirement: Contract conformance reports through the always-present check, and the cluster jobs live elsewhere
Contract conformance — every adapter's built binary against a fake manager —
SHALL be a job of the `ci` workflow and SHALL be listed in `ci-green`'s
`needs:`. It SHALL NOT be a separately required status check: branch
protection names one check, and adding a gate is a line in that job's
`needs:`. The `ci` workflow SHALL provision no cluster: the k3s smoke belongs
to the release workflow and to on-demand runs, both calling `e2e.yml`.

The job SHALL run only when the diff touches what a running install is built
from — a component group, the chart, the test doubles under `test/`, or the
e2e workflow itself — and a job that did not run reports through `ci-green` as
skipped, exactly as every other path-filtered job does.

The boundary is stated on every side: `continuous-integration` owns per-module
build, vet and unit test, the envtest suite, chart lint and render, the image
builds and scans, the site, the guards, and the job that RUNS the conformance
suite on a pull request; `contract-conformance-suite` owns what that suite
asserts; `end-to-end-testing` owns the cluster-based jobs only. No capability
restates another's jobs, so "what CI runs" has exactly one definition per tier.

#### Scenario: Conformance gates through ci-green
- **WHEN** a pull request changes a component, the chart or the test doubles
- **THEN** the `conformance` job runs, and `ci-green` fails if it failed

#### Scenario: A docs-only pull request runs nothing of it
- **WHEN** a pull request changes only pages under `docs/`
- **THEN** the `conformance` job is skipped and `ci-green` treats it as skipped, not failed

#### Scenario: A per-module job is not duplicated in the e2e workflow
- **WHEN** a change adds a per-module build, vet or lint step
- **THEN** it is specified and wired under `continuous-integration`, and `e2e.yml` carries no copy of it
