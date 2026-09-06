## MODIFIED Requirements

### Requirement: Tiers are defined by what gates a pull request
The pack SHALL be split into tiers with an explicit gating rule:

- **Pull requests** SHALL be gated by contract conformance alone — a minute,
  no cluster, no secret — reported through `ci-green`'s `needs:`, never as a
  separately required check. No pull request provisions a cluster.
- **The cluster smoke** — the smoke tier: a thin k3s run on the stub runtime,
  deterministic, no secret, bounded in wall-clock time; conformance is not
  restated in it, since the release already requires the commit's CI — SHALL
  gate a RELEASE and SHALL be runnable on demand against any branch.
- **A release** SHALL publish nothing until the cluster smoke passed on the
  tagged COMMIT: a tag can land on a commit CI proved days earlier against a
  different cluster. The smoke is keyed to the commit, never to the tag: a
  release SHALL look up the commit's existing smoke result before provisioning
  a cluster, SHALL reuse a passed one from any earlier run on that commit,
  SHALL wait — bounded — for one still running on that commit rather than
  race it, and SHALL run its own only where none passed. A commit tagged
  fourteen times is smoked once.
- **The full pack, including the real-runtime lane,** SHALL run nightly when
  the default branch moved since its last successful run — a night with no
  change is a night with nothing to learn and tokens to spend — and on manual
  dispatch, and SHALL NOT gate pull requests.
- **Pull requests from forks** SHALL run every tier their secret access allows
  and SHALL NOT be failed for tiers they cannot run. Secrets being unavailable
  to forks is the intended access boundary and SHALL NOT be worked around.

#### Scenario: A fork pull request is not failed for missing secrets
- **WHEN** a pull request originates from a fork and the API token is unavailable
- **THEN** the real-runtime tier is skipped and reported as skipped, and the pull request is not marked failed on that account

#### Scenario: A pull request runs no cluster
- **WHEN** a pull request changes a component or the chart
- **THEN** contract conformance runs and gates it through `ci-green`, and no cluster is provisioned

#### Scenario: A release is gated by the smoke on the tagged commit
- **WHEN** a `<component>-v<semver>` or `chart-v<semver>` tag is pushed and no run has smoked that commit
- **THEN** the cluster smoke runs on that commit, and the image or chart is published only if it passed

#### Scenario: A commit already smoked is not smoked again
- **WHEN** a tag is pushed on a commit whose smoke passed in an earlier release run or an on-demand run
- **THEN** no cluster is provisioned, the run records the commit as smoked, and the image or chart publishes

#### Scenario: A smoke in flight on the commit is waited for
- **WHEN** a tag is pushed while another run's smoke on the same commit is still running
- **THEN** the release waits for that verdict, publishes on a pass, and runs its own smoke on a failure

#### Scenario: A re-run after a flake smokes again
- **WHEN** a release run whose smoke failed is re-run
- **THEN** the lookup finds no passed smoke and the cluster smoke runs

#### Scenario: An unchanged night runs nothing
- **WHEN** the nightly full run finds the default branch at the commit its last successful run tested
- **THEN** it skips, and a dispatched run in the same state still runs

#### Scenario: A token-consuming tier never gates a pull request
- **WHEN** the real-runtime tier fails
- **THEN** no pull request is blocked by that failure alone
