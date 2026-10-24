## Why

Two Playwright suites exist for the console and neither runs anywhere but a
workstation.

`platform/console/ui/e2e/` needs no cluster and no secret, yet a console
pull request can break it silently.

`platform/console/ui/e2e-live/` needs a real cluster, and its 21 specs
already caught a real defect (`docs/testing.md`). It is dispatched by
nobody, so a regression it would catch ships until someone happens to run it
by hand.

## What Changes

- Add a `console-e2e` job to `ci.yml` that runs `test:e2e` (the mocked-API
  Playwright smoke) against the built console bundle, gated on the same
  console-path filter the UI unit-test job already uses, reporting through
  `ci-green`.
- Extend the full system tier (`system.yml`'s `tier: full`, fired nightly by
  `system-full.yml`) with a step that runs `e2e-live` against the cluster and
  console the pack already stood up for that run, after the pack's own
  assertions pass.
- `e2e-live` never gates a pull request, matching the real-runtime lane's
  existing posture: it runs nightly when `master` moved, and on manual
  dispatch.
- Update `docs/testing.md`'s `e2e-live` section, which currently states "it
  is NOT CI-gated, and no workflow dispatches it."

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `continuous-integration`: a console pull request now runs the mocked
  browser e2e tier as a gating check, not only the jsdom-based UI suite.
- `end-to-end-testing`: the full nightly tier now includes a real-browser
  pass against the console, driven by the existing `e2e-live` suite, inside
  the same cluster run.

## Impact

- **Reference docs**: `docs/testing.md` (the tier table's "Console browser
  e2e" row and its "NOT CI-gated" claim), `CONTRIBUTING.md` (the CI section
  describing what a pull request and the nightly full tier each run).
- **Adopter site**: none — this is internal verification process, not a
  product capability or an install/upgrade step the landing page, guides, or
  installation docs describe.
- **Code**: `.github/workflows/ci.yml` (new job plus its path filter in
  `discover`), `.github/workflows/system.yml` (a step in the `full`-tier job
  that drives `e2e-live` against the pack's own cluster), `platform/console/ui/package.json`
  (if a dedicated CI script is useful beside `test:e2e`).
