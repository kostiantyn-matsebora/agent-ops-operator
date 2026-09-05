## Why

`.github/workflows/e2e.yml` runs `go test -tags e2e -v ./test/e2e/` for both
tiers and uploads diagnostics only on failure. Reading a result — did the
smoke tier's tests pass, which lane in the full pack failed — means opening
the job log and scrolling a `-v` dump. The full tier runs nightly across every
lane, including the real-runtime one, so its log is the larger of the two and
the least suited to reading in full; the smoke tier is short and gates a
release, so its detail is cheap and worth having at a glance. Neither tier
currently writes anything to the run's own summary page.

## What Changes

- The "Run the pack" step in `.github/workflows/e2e.yml` runs `go test` with
  `-json` (kept `-v` for full output text) instead of plain `-v`, teeing the
  raw test2json event stream to a file under `$E2E_ARTIFACT_DIR` while still
  streaming human-readable output to the live job log.
- A new script, `.github/scripts/e2e-report.py`, parses that event stream and
  renders a markdown report at one of two levels: `summary` (pass/fail/skip
  counts, total elapsed, and the names of any failed or skipped tests) or
  `full` (every test's name, status and elapsed time, plus the captured
  output of any failure).
- A new step, run `if: always()` after the pack, appends the report to
  `$GITHUB_STEP_SUMMARY` — `summary` level for the full tier, `full` level for
  the smoke tier.
- `.github/tests/e2e-report.test.sh` covers both levels and the case where the
  event stream holds no parseable test events (a build failure before any
  test runs).

## Capabilities

### New Capabilities

(none — see Impact: this is a CI-tooling change with no spec-level product
behavior)

### Modified Capabilities

(none)

## Impact

- `.github/workflows/e2e.yml` — the shared `pack` job gains one changed step
  and one new step; no change to what the two tiers install, test or gate.
- New `.github/scripts/e2e-report.py` and `.github/tests/e2e-report.test.sh`;
  `.github/tests/run.sh` picks the latter up automatically via its `*.test.sh`
  glob.
- `docs/testing.md` — the page that owns the tier model gets one addition:
  each tier's CI run now surfaces a report on the Actions run summary, and at
  what level. No other reference doc and no adopter-site page describes CI
  workflow internals, so nothing else needs updating.
- No CRD, controller, chart, or product-facing behavior changes. Nothing here
  is decided by a Kubernetes cluster.
