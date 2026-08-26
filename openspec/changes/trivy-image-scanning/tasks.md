## 1. Measure before gating

D-Migration: enabling the gate first means the first red build belongs to
whoever opened the next unrelated pull request. Record COUNTS PER COMPONENT,
never the findings themselves — a findings list is a local artifact.

- [ ] 1.1 Build each image locally and scan it with the same settings the gate
  will use (`severity CRITICAL,HIGH`, `ignore-unfixed: true`). Record the count
  per component
- [ ] 1.2 Record the counts WITHOUT `ignore-unfixed` as well, for the three base
  tiers only — distroless, `debian:bookworm-slim` (egress-proxy, runtime-ollama),
  `node:22-bookworm-slim` (runtime-claude). This is the number D5 trades away,
  and it should be known rather than assumed
- [ ] 1.3 Decide from 1.1 whether any fixable finding must be fixed before the
  gate is enabled, and whether a seeded `.trivyignore` is needed at all. Prefer
  fixing; an exception is the fallback
- [ ] 1.4 Fix what 1.3 says to fix — a base image bump, a Go dependency bump, an
  npm bump — and verify the count drops by re-running 1.1 for that component

## 2. The composite action

Port `deployment-dashboard`'s `.github/actions/trivy-scan/action.yml`. Match it
step for step; D1 says this is a port, not a redesign.

- [ ] 2.1 Create `.github/actions/trivy-scan/action.yml` with inputs `image-ref`
  and `category`, both required, and the `category` description stating WHY it
  must be unique — that results uploaded under one category replace what was
  there
- [ ] 2.2 Step 1: `aquasecurity/trivy-action@v0.36.0`, `scan-type: image`,
  `format: sarif`, `output: trivy-results.sarif`, `exit-code: '0'`,
  `ignore-unfixed: true`, `severity: CRITICAL,HIGH`
- [ ] 2.3 Step 2: `github/codeql-action/upload-sarif@v4` at `if: always()`, with
  `category` from the input. Verify the `always()` is present — without it the
  security tab describes the last run that passed
- [ ] 2.4 Step 3: the gate — same action and settings, `format: table`,
  `exit-code: '1'`. Table format so the finding is readable in the log without
  leaving the run
- [ ] 2.5 Carry the reference's comments explaining why there are two runs and
  why the second is cheap. A reader who thinks it is duplication deletes one

## 3. Wire it into `ci.yml`

- [ ] 3.1 Add `load: true` and `tags:` to the `images` job's
  `docker/build-push-action` step so there is an image in the daemon.
  `push: false` and `platforms: linux/amd64` are unchanged — `load:` cannot
  express a multi-platform result, so the existing single-platform build is what
  makes this work
- [ ] 3.2 Verify no artifact escapes: confirm the job still pushes nothing, so
  the existing CI requirement holds
- [ ] 3.3 Add a job-level `permissions:` block to `images` with
  `contents: read` and `security-events: write`. Leave the workflow-level
  `contents: read` alone, and verify no other job gained a permission
- [ ] 3.4 Call the action with `category: trivy-${{ matrix.image.component }}`.
  **Settle the fork case (D6):** confirm what `upload-sarif` does on a
  pull request from a fork, where the token is read-only, and guard the upload so
  an external contribution cannot go red for a permission it cannot hold. The
  GATE must still run for a fork — that is the half that protects the merge
- [ ] 3.5 Cache Trivy's vulnerability database across the matrix. Up to
  fourteen legs pulling it is slow and rate-limitable, and the gate now carries a network
  dependency the build did not have
- [ ] 3.6 Enable the gate LAST, after task 1 is closed out
- [ ] 3.7 Verify per-component isolation on a real run: every component's
  findings readable separately in the security tab, none replaced by another's.
  D2 names this as the highest-consequence detail, and it fails GREEN
- [ ] 3.8 Verify `fail-fast: false` still holds, so one component's failure does
  not hide the other thirteen
- [ ] 3.9 Verify the scan follows the changed-only filter: a pull request
  touching one component scans that one and builds nothing else. The scan adds
  no matrix leg the build did not already have

## 4. The scheduled scan

Additive and separable — D4. Its own workflow, not a second trigger on `ci.yml`.

- [ ] 4.1 New workflow on a `schedule:`, scanning the PUBLISHED images by
  reference rather than building anything
- [ ] 4.2 Derive the image list from `.github/components.sh`, so a new component
  is covered with nothing edited
- [ ] 4.3 Report only — no `exit-code: 1` anywhere in it. Verify a run with
  findings still ends green
- [ ] 4.4 Use a category distinct from CI's, so a scheduled result never replaces
  a pull request's or the reverse
- [ ] 4.5 State which architecture is scanned. Published images are multi-arch
  and a scan covers one — claiming coverage of both would be the silent kind of
  wrong
- [ ] 4.6 Verify it runs at all: trigger it manually once via `workflow_dispatch`
  before trusting the schedule. A schedule that never fires and a scan that finds
  nothing look identical

## 5. Exceptions

- [ ] 5.1 Create `.trivyignore` only if task 1.3 called for it. An empty file
  committed "for later" is an invitation
- [ ] 5.2 Every entry carries the reason and the expiry date, per the spec
- [ ] 5.3 Verify an expired entry stops suppressing its finding, rather than
  being renewed silently — establish this by testing it, not by reading the
  documentation

## 6. Close out

- [ ] 6.1 A pull request with a deliberately vulnerable dependency FAILS the
  gate, naming the component. A gate that has never failed has not been tested
- [ ] 6.2 That same run still uploaded its SARIF — this is what `if: always()`
  buys and it is invisible until the gate fails
- [ ] 6.3 A clean pull request passes, and CI's wall-clock time is still
  acceptable on a rebuild-everything change — fourteen scans in the matrix
- [ ] 6.4 `python3 .github/scripts/publication-guard.py` passes. Record the
  verdict only
- [ ] 6.5 `openspec validate trivy-image-scanning --strict`

## 7. Documentation

Both halves, listed separately because they are skipped independently.

### 7.1 The reference docs

- [ ] 7.1.1 `CONTRIBUTING.md` — what the gate checks, that a finding with no
  available fix does not block, how an exception is declared, and that the
  scheduled scan reports rather than gates. A contributor meeting a red scan
  needs this before they need anything else
- [ ] 7.1.2 `SECURITY.md` — its scope section separates an upstream vulnerability
  from one the chart's defaults make reachable. State what this project now
  detects about its own images, and that unfixable upstream findings are
  knowingly not gated
- [ ] 7.1.3 `docs/CHANGELOG.md` — confirm NO entry is owed. Nothing an adopter
  installs or upgrades changed. Deliberate, not skipped
- [ ] 7.1.4 No CRD field, chart value or api doc comment changed, so
  `docs-generate.py` is not required. Confirm rather than assume

### 7.2 The adopter site

- [ ] 7.2.1 `docs/security.md` — the supply-chain paragraph ("Every published
  image carries an SBOM and max-mode provenance…") states beside it that every
  image is scanned on the pull request that builds it and on a schedule
  afterwards, what blocks (a fixable HIGH/CRITICAL) and what is knowingly not
  gated (unfixable findings). Same register as the signing sentence: what holds,
  and no further
- [ ] 7.2.2 The `security-posture-page` MODIFIED delta in this change's
  `specs/` says the same thing, so the published spec and the page agree.
  Created with `/opsx:continue`, not by hand
- [ ] 7.2.3 Confirm no other site page describes CI
