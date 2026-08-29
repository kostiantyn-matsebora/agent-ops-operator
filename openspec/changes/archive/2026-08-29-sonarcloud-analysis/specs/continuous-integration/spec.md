## MODIFIED Requirements

### Requirement: The console UI is built and tested

CI SHALL install, test, and build the console's browser application on every
pull request and every push to `master` that touches the console, as part of
the console component's own build-and-test job, and the UI's tests SHALL run
exactly once per workflow run. A failing UI test SHALL be named by its own
step, so the failure reads as the UI's and not as the image's or the Go
module's.

#### Scenario: A UI test fails

- **WHEN** a pull request breaks a console UI test
- **THEN** CI fails and the failing step names the UI, not the image that
  embeds it

#### Scenario: A UI build breaks

- **WHEN** the browser application no longer builds
- **THEN** CI fails before any image is built

#### Scenario: The UI suite runs once

- **WHEN** a pull request touches the console
- **THEN** the UI test suite executes once in that workflow run, producing the
  coverage the analysis reads
