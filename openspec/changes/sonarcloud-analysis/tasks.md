## 1. The projects, outside the tree

Recorded as VERDICTS. No token, no organisation-scoped URL, no project key
list pasted — a key is derivable from `components.sh`, and that is the point.

- [x] 1.1 Confirm the SonarCloud organisation is bound to this GitHub account
  and the SonarCloud GitHub app is installed on this repository (D8). DONE — app installed, org bound; the wizard was used for the first project
- [x] 1.2 Create one project per component from `components.sh images`, key
  `<org>_agent-ops-operator_<component>`, name `agentops-<component>`, using
  the web API in a loop where it accepts it and the UI where it does not (D1).
  Record the COUNT created. DONE 2026-08-29 through `.github/scripts/sonar-provision.sh` (D9): 15 created, names `agentops-<component>`
- [x] 1.3 Enable monorepo mode binding every project to this repository, and
  verify each project decorates as its own check name on a test pull request. DONE — `is_bound_to_monorepo` is `true` on all 15; PR 127 known to each project by title
- [x] 1.4 Turn Automatic Analysis OFF on every project (D6). Verify by the
  first CI submission succeeding rather than being rejected as a conflict. DONE — CI submissions accepted on every project; the projects were created by provisioning, which has no automatic analysis
- [x] 1.5 Add repository secrets `SONAR_TOKEN` and `SONAR_ORG`. Record that
  both exist, nothing else. DONE 2026-08-29 — both exist as secrets; the
  workflow reads `secrets.SONAR_ORG`, so the organisation is masked in logs

## 2. Coverage from the existing test jobs (D3)

Each test invocation gains its coverage flag, and the analysis step (section
3) reads the profile from where the tests wrote it. Nothing is uploaded.

- [x] 2.1 `operator`: `go test -count=1 -coverprofile=coverage.out ./...`
- [x] 2.2 `modules`: the same per matrix leg
- [x] 2.3 the console's UI coverage, in its `modules` leg: `@vitest/coverage-v8`
  matching the installed vitest major, `npm run test:coverage` (lcov), `SF:`
  re-anchored to `ui/`. `console-ui` is deleted (3.10). VERIFIED locally that `npm test` passes without coverage flags
- [x] 2.4 `node-runtimes`: `node --test --experimental-test-coverage
  --test-reporter=spec --test-reporter-destination=stdout --test-reporter=lcov
  --test-reporter-destination=coverage.lcov` per runtime. VERIFIED locally on
  both runtimes; `spec` is kept so the log still shows the tests
- [x] 2.5 Verify no test job's runtime changed materially — read the durations
  before and after on one run. DONE — the analysis step adds 35–50 s to a
  module leg, 1m22 to the console's

## 3. The analysis step in `ci.yml`

- [x] 3.1 Add the job: `needs: [discover, operator, modules, console-ui,
  node-runtimes]`, `if: !cancelled()` plus the discover-succeeded and
  non-empty-images conditions the `images` job uses, plus the fork guard (D7),
  `strategy.fail-fast: false`, matrix from `discover.outputs.images`, name
  `sonar (<component>)`
- [x] 3.2 Checkout with `fetch-depth: 0`, with the comment saying why
- [x] 3.3 Download the component's coverage artifact(s) by derived name,
  tolerating absence; the console downloads two
- [x] 3.4 Run `SonarSource/sonarqube-scan-action` pinned to a version, with
  `projectBaseDir` from the matrix context and `args` carrying the project
  key, the exclusions and test inclusions (D4), and the coverage report paths.
  `sonar.qualitygate.wait` unset (D5)
- [x] 3.5 Add `sonar` to `ci-green`'s `needs:` — the whole of making it required
- [x] 3.6 Verify on the change's own pull request: it edits `ci.yml`, so all
  fifteen legs run. Every component submits; every component with tests shows
  a non-zero coverage figure on its dashboard. A zero where tests exist is the
  artifact-name mismatch D3 warns of — fix the transform, not the number. VERIFIED on run 33244606350: Go cover sensor loaded `coverage.out` for manager and console, JS sensor analysed both runtimes' lcov and the UI's; the console leg downloaded both artifacts
- [x] 3.7 Verify the fork path by reading the condition against a fork event's
  payload shape, since no fork pull request exists to test with
- [x] 3.8 Verify the pull request shows one SonarCloud check per analysed
  component and that none is listed as required in branch protection. PARTIAL — every project is monorepo-bound and knows PR 127 (title, quality gate OK), but NO SonarCloud check-run appeared on the head sha: the SonarCloud GitHub App installation must include this repository. None is required in branch protection, so the merge is unaffected
- [x] 3.9 Read the first dashboards and record COUNTS per component — issues by
  severity, hotspots, coverage percent. Never a finding. These are the input
  to the later gating change and are the last line of this section. RECORDED for PR 127: bugs 0, vulnerabilities 0, code smells 0, hotspots 0 on all 15; quality gate OK on all 15; 0 task warnings

### 3.10 Restructured after review (D2): a step, not a job

- [x] 3.10 The separate `sonar` job, the four artifact uploads, the pattern
  download and `coverage-artifact.sh` are DELETED; `.github/actions/sonar-scan`
  runs as the last step of `operator`, each `modules` leg and each
  `node-runtimes` leg; the console's UI coverage is produced in its `modules`
  leg; `console-ui` is DELETED — that leg is the UI suite's one run, its step
  named for the UI. Verified green on the pull request
  with every scanner's coverage sensor loading its report

## 4. Documentation

Written last, from what the change actually did.

### 4.1 Reference docs

- [x] 4.1.1 `CONTRIBUTING.md`: a "Code analysis" subsection beside "The image
  scan" — what is analysed, per component, where the dashboard is, what fails
  the pull request (the scanner) and what does not (the quality gate), what a
  fork gets, and what a NEW component owes: one project created the way 1.2
  did it. Add the analysis step's row to the "What reports through it"
  table
- [x] 4.1.2 `docs/security.md`: the paragraph naming the two Trivy scans as the
  security tab's reporters; add that security hotspots are reported on the
  analysis dashboard, per component, and are not a gate
- [x] 4.1.3 `.claude/rules/build-test.md`: the local coverage invocations,
  matching the flags CI uses for Go, vitest and `node --test`
- [x] 4.1.4 `docs/CHANGELOG.md`: confirm no entry is owed — nothing an
  installed release does changed

### 4.2 Adopter site

- [x] 4.2.1 Re-read the landing page, `introduction.md`, `getting-started.md`,
  `installation.md`, the integration pages and the guides for any sentence
  about CI, code quality or coverage. Expected: none, per the proposal's
  Impact. Record that it was checked; if one exists, fix it here. CHECKED
  2026-08-29: no page names CI, coverage or code quality; nothing to change
