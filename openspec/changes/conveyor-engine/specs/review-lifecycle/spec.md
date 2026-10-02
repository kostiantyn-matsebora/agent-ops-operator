## Purpose

The code-review pipeline's own lifecycle on its pull request: whether a
review is running, clean, has found issues, was skipped, or failed.

Kept separate from the station and loop decisions `conveyor-lifecycle`
owns, and never coupled to a pull request's own checks.

## ADDED Requirements

### Requirement: The review's lifecycle is tracked on its own pull request, never on a tracking issue

The `review` workflow SHALL declare `subject: pull_request`, and every state
it writes SHALL be a label on that pull request.

No part of this workflow SHALL read or write a station or loop label, and no
part of `conveyor-lifecycle`'s station or loop workflows SHALL read or write
a `scan:*` label.

#### Scenario: A pull request opens

- **WHEN** a pull request is opened, synchronized, or marked ready for review
- **THEN** the `review` workflow starts tracking it, and no station or loop
  label on any related issue is read or written

#### Scenario: A station or loop transition fires

- **WHEN** a `conveyor.*` or `loop` transition fires on an issue or pull
  request
- **THEN** no `scan:*` label changes as a result

### Requirement: A push starts a review cycle, and every outcome returns to running

The `review` workflow SHALL move to `scan:running` whenever the pull request
is opened, synchronized, or marked ready for review, from its initial state
and from every one of its outcome states (`scan:clean`, `scan:found_issues`,
`scan:skipped`, `scan:failed`).

#### Scenario: The first push

- **WHEN** a pull request opens for the first time
- **THEN** the workflow moves from its initial state to `scan:running`

#### Scenario: A later push

- **WHEN** a pull request already carrying an outcome state receives a new
  commit
- **THEN** the workflow moves to `scan:running` again, replacing whichever
  outcome label was there

### Requirement: A completed run's outcome is read from the run that happened, never re-derived

The `review` workflow SHALL decide a completed run's outcome — clean, found
issues, skipped, or failed — from facts the caller reads off that run: its
own conclusion, whether the pull request carries open review threads, and
whether the run's own skip conditions applied.

No guard in this workflow SHALL re-implement a skip condition the review
workflow's own queueing already decides.

#### Scenario: A run completes clean

- **WHEN** a review run concludes successfully and the pull request carries
  no open review thread
- **THEN** the workflow moves to `scan:clean`

#### Scenario: A run completes with findings

- **WHEN** a review run concludes successfully and the pull request carries
  at least one open review thread
- **THEN** the workflow moves to `scan:found_issues`

#### Scenario: A run is skipped

- **WHEN** a review run reports itself skipped — a draft, a fork, a
  dependabot actor, a failed hygiene guard, or an edited review workflow file
- **THEN** the workflow moves to `scan:skipped`

#### Scenario: A run fails

- **WHEN** a review run concludes without success and was not reported
  skipped
- **THEN** the workflow moves to `scan:failed`

### Requirement: A review outcome never fails a pull request's own checks

Reaching `scan:found_issues` or `scan:failed` SHALL NOT cause any required
check on the pull request to fail, and SHALL NOT be wired into `ci-green` or
any workflow this change's `conveyor.*` or `loop` workflows drive.

An open review thread already holds the merge through the platform's
required conversation resolution. This workflow's state exists to make that
lifecycle inspectable, not to gate it a second time.

#### Scenario: A review finds issues

- **WHEN** the workflow moves to `scan:found_issues`
- **THEN** no required check is made to fail because of it, and the merge is
  blocked only by the open threads themselves

#### Scenario: A review run fails

- **WHEN** the workflow moves to `scan:failed`
- **THEN** no required check outside the review's own run is made to fail
  because of it

### Requirement: An outcome worth acting on names a stub action

The transitions into `scan:found_issues` and `scan:failed` SHALL each name
an `owned_by` action. The transitions into `scan:clean` and `scan:skipped`
SHALL name none.

Every named action SHALL be a registered stub in this change, performing no
side effect beyond recording what it would have done.

#### Scenario: Findings land

- **WHEN** the workflow moves to `scan:found_issues`
- **THEN** its named action is recorded as called, and performs no side
  effect

#### Scenario: A clean run lands

- **WHEN** the workflow moves to `scan:clean`
- **THEN** no action is called
