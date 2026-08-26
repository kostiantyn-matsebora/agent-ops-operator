## Why

**Fourteen images are published and none of them is ever scanned.** CI builds
every image on every pull request and asserts the Dockerfile works; nothing asks
what is inside the result. `.github/` contains no scanner of any kind.

The images already carry an SBOM and max-mode provenance, so the contents are
described and attested. Nothing reads that description against a vulnerability
database — the repository can state precisely what shipped, and not whether any
of it is known-vulnerable.

**The pattern to follow already exists**, in this author's
`deployment-dashboard` repository: a composite action `.github/actions/trivy-scan`
that runs `aquasecurity/trivy-action` twice against one locally-loaded image —
once to emit SARIF for the security tab without failing anything, once as a gate
that fails on CRITICAL or HIGH with `ignore-unfixed: true`. This change ports
that pattern rather than designing a second one.

Two properties of it are the reason it works, and both are kept:

- **`ignore-unfixed: true`.** A gate that cannot be made green gets bypassed, and
  then it enforces nothing — the same reasoning this repository already applies
  to a hook that blocks work it does not understand. An unfixable upstream CVE is
  information, not a task.
- **The SARIF upload runs `if: always()`**, so the security tab is current even
  on the run where the gate failed.

## What Changes

- **A composite action `.github/actions/trivy-scan`**, ported from
  `deployment-dashboard`: SARIF run at `exit-code: 0`, upload at `if: always()`,
  then a table-format gate run at `exit-code: 1`. Both runs
  `severity: CRITICAL,HIGH` and `ignore-unfixed: true`; the second is cheap
  because the first warmed Trivy's database.
- **`ci.yml`'s existing `images` matrix job calls it.** The build step gains
  `load: true` and a `:scan` tag so there is an image in the daemon to scan. It
  stays single-platform and unpublished, so no artifact escapes a pull request.
- **The SARIF category is per component.** `deployment-dashboard` passes a
  distinct `category` per service because results otherwise overwrite one
  another in the security tab. Here that becomes
  `trivy-${{ matrix.image.component }}` across a fourteen-way matrix, where
  getting it wrong means thirteen components' findings silently replaced by the
  fourteenth's.
- **`security-events: write` is granted on the scanning job only.** `ci.yml`
  declares `contents: read` at workflow level, and that stays.
- **A scheduled scan of the PUBLISHED images.** Beyond the ported pattern, and
  separable — see `design.md` D4. It is the only mechanism that catches a CVE
  disclosed after an image was built, which is a case no build-triggered scan can
  reach — and, because CI builds only the components a pull request touched,
  the only mechanism that scans an untouched component's base image at all. It
  reports and never fails.
- **Nothing is added to the release path.** See `design.md` D3.

## Capabilities

### New Capabilities
- `image-vulnerability-scanning`: what is scanned, when, which findings block and
  which are only reported, how results reach the security tab without one
  component's overwriting another's, and what an exception is.

### Modified Capabilities
- `security-posture-page`: the supply-chain statement on `docs/security.md`
  says what the published images carry; scanning joins it, stated with the same
  bound (fixable HIGH/CRITICAL gate, unfixable findings reported nowhere).

<!-- `continuous-integration` is deliberately NOT modified. Scanning is its own
     capability — what blocks, what is reported, what an exception is — and the
     CI spec keeps describing the build the scan is attached to. -->

## Impact

**CI and workflows**

- `.github/actions/trivy-scan/action.yml` — new, ported.
- `.github/workflows/ci.yml` — the `images` job gains `load: true`, a `:scan`
  tag, a job-level `permissions:` block, and the scan step.
- `.github/workflows/` — a new scheduled workflow for the published images.
- `.trivyignore` — created only if the baseline in task 1 shows it is needed, and
  every entry carries a reason and an expiry.

**Ordering**

- None left. `sdlc-setup` and `publish-security-page` both archived on
  2026-08-24, so `continuous-integration`, `release-publishing` and
  `security-posture-page` are published specs this change reads against.
- **CI builds only what a pull request changed** (`continuous-integration`), so
  a pull request scans only the components it touched. That is what makes the
  scheduled scan a coverage mechanism rather than a nicety.

**Documentation — the reference docs**

- `CONTRIBUTING.md` — **required.** It owns the build and test commands and how a
  change is proposed. A contributor whose pull request goes red needs to know
  what the gate checks, that unfixable findings do not block, and how an
  exception is declared.
- `SECURITY.md` — its scope section already separates an upstream vulnerability
  from one the chart's defaults make reachable. What this project now detects
  about its own images belongs beside that.
- `docs/CHANGELOG.md` — **no entry.** Nothing an adopter installs or upgrades
  changes. Confirmed deliberately, not skipped.

**Documentation — the adopter site**

- `docs/security.md` — **required.** Its supply-chain paragraph states SBOM and
  provenance and the absence of signing; what the images are scanned for, and
  what is knowingly not gated, is stated beside it. No other site page describes
  CI.

**Not affected**

- `build-image.yml` and `release.yml` — unchanged, per D3.
- `.github/components.sh` — unchanged. The scan runs over the image set it
  already derives, so a new component is scanned without editing anything.
- Existing action versions in `ci.yml` — not bumped. `deployment-dashboard` runs
  newer majors of `checkout`, `setup-buildx` and `build-push`; porting the
  scanning pattern is not a reason to move this repository's other pins.
