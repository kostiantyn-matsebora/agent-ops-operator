## Why

The weekly scan of the published images reports and never fails, so on
2026-08-27 it ran green over 101 fixable CRITICAL/HIGH findings — every Go
image carried 22 standard-library CVEs — and nothing but a visit to the
security tab would have said so. A run whose status cannot go red is not a
signal, and the README cannot carry a badge for it honestly.

## What Changes

- The scheduled `image-scan.yml` runs the trivy-scan action's gate as well as
  its reporting run: SARIF is uploaded first (`if: always()`), then the gate
  fails the job on a fixable CRITICAL or HIGH finding. The workflow's status
  then means "every published image scans clean today".
- The pull-request gate is unchanged, and so is the threshold: one scanner, one
  severity set, one `.trivyignore`.
- `README.md` gains three badges beside the existing three: the chart version
  (from the newest `chart-v*` tag), the `ci` workflow status, and the
  `image-scan` workflow status labelled as the published-image scan.
- The sentences saying the scheduled scan "reports and never gates" are
  rewritten wherever they appear: the workflow header, the action's `gate`
  input, `CONTRIBUTING.md`, `docs/security.md`.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `image-vulnerability-scanning`: the requirement "Published images are
  re-scanned on a schedule" changes from SHALL NOT gate to SHALL fail the
  scheduled run on a fixable CRITICAL/HIGH finding, after reporting.

## Impact

- `.github/workflows/image-scan.yml` — drops `gate: "false"`, header rewritten.
- `.github/actions/trivy-scan/action.yml` — the `gate` input's description no
  longer names the scheduled scan as the caller that passes false (no caller
  does now, and the input stays for a fork's pull request and for any future
  caller).
- `README.md` — three badge lines; the 215-line budget holds.
- Reference docs made untrue: `CONTRIBUTING.md` ("blocks nothing"),
  `openspec/specs/image-vulnerability-scanning/spec.md` (via the delta).
- Adopter site made untrue: `docs/security.md` ("Only the pull-request scan
  blocks"). The landing page, introduction, getting started, installation and
  the guides do not describe the scan and need nothing.
- No chart, CRD or image changes, so no `docs-generate.py` run is owed.
