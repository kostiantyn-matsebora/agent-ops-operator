## ADDED Requirements

### Requirement: A console pull request runs the mocked-API browser e2e tier

CI SHALL run `platform/console/ui/e2e/` (the `test:e2e` Playwright suite,
a real browser against the production build served behind the suite's own
mocked API) on every pull request and every push to `master` that touches
the console, as a job reporting through `ci-green`.

This tier needs no cluster and no secret, so it SHALL use the same
console-path filter the UI unit-test job already uses, never the cluster
path filter `end-to-end-testing` owns.

#### Scenario: A console pull request runs the browser smoke

- **WHEN** a pull request touches `platform/console/ui/**`
- **THEN** the `console-e2e` job builds the console and runs `test:e2e`
  against it, and `ci-green` fails if that job failed

#### Scenario: A non-console pull request skips it

- **WHEN** a pull request touches no file under `platform/console/ui/`
- **THEN** the `console-e2e` job is skipped, and `ci-green` treats it as
  skipped, not failed

#### Scenario: The rendering defect this tier exists for is caught

- **WHEN** a change makes the console's shipped SVG fail to render, animate,
  or stay stable in a real browser, in a way jsdom cannot observe
- **THEN** the `console-e2e` job fails on that pull request
