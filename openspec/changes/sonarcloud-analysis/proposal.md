## Why

**Fifteen components are built, vetted, tested and scanned for vulnerabilities on
every pull request, and nothing measures the code itself.** No coverage number
exists anywhere, no duplication, no maintainability finding, no security
hotspot — `go vet` and the type checker are the whole of static analysis. A
reviewer reading a pull request has the review workflow's findings and their own
eyes, and a stranger evaluating the project has the CI badge.

SonarCloud is free for a public repository and speaks every language here (Go,
TypeScript, JavaScript), and its GitHub integration posts the verdict on the
pull request. The repository has been public since `public-repository` archived,
so the one precondition is met. Now, because the component set just stabilised
under `repository-layout` — one directory, one image, one name derived by
`.github/components.sh` — and that derivation is what makes "all components" a
list a program produces rather than one somebody types.

## What Changes

- **One SonarCloud project per component, under SonarCloud's monorepo mode.**
  The project key and display name are derived from the component name
  `components.sh` publishes (`agentops-<component>`), so the tree, the image
  registry and the analysis dashboard agree on what exists. Fifteen projects
  today; a sixteenth appears when a directory does, and the workflow never
  lists them.
- **The analysis is the last step of the job that tests the component** —
  `operator`, each `modules` leg, each `node-runtimes` leg — through one
  composite action, so it follows the changed-only filter those jobs already
  follow and reads the coverage profile from where the tests left it. A
  scanner that fails to submit fails that job, which reports through
  `ci-green` like every other gate; the QUALITY GATE verdict is SonarCloud's
  own check on the pull request and is NOT required by branch protection. See
  `design.md` D5: measure before gating, the same order the image scan took.
- **No test runs twice and nothing is uploaded.** The operator's envtest suite
  is minutes, and a job that re-ran it for a coverage number would be the job
  somebody switches off.
- **CI-based analysis only.** SonarCloud's Automatic Analysis is turned off for
  every project: it is incompatible with CI-submitted analysis, reads no
  coverage, and does not understand a monorepo.
- **A fork's pull request is analysed by nothing**, shown as a SKIPPED job. The
  scanner needs `SONAR_TOKEN`, and repository secrets are withheld from fork
  workflows — the same rule, the same visible shape, as the review workflow.
- **Nothing outside a component is analysed.** The chart, `.github/scripts/`
  and `docs/` are not components and get no project. A non-goal, stated so it
  is not read as an oversight — `design.md`.
- **No README badge.** Fifteen projects have fifteen badges and no honest
  single one; the README is at budget. `CONTRIBUTING.md` links the
  organisation's dashboard instead.

## Capabilities

### New Capabilities
- `code-quality-analysis`: what is analysed and at what granularity, when,
  what a pull request sees, how coverage reaches the analysis, what blocks the
  merge and what only reports, and what a fork's pull request gets.

### Modified Capabilities

None. `continuous-integration`'s always-present-check requirement already
describes how a new job reports through the gate; adding one is a line in its
`needs:`, which is what that requirement exists for.

## Impact

**Code and workflows:**
- `.github/workflows/ci.yml` — coverage-emitting test invocations and the
  `sonar-scan` step in `operator`, `modules` and `node-runtimes`;
  `.github/actions/sonar-scan`; `.github/scripts/sonar-provision.sh`.
- `.github/components.sh` — unchanged. The job derives everything from
  `images`; if a field is missing it is added THERE, never typed in the
  workflow.
- SonarCloud, outside the tree: an organisation, fifteen projects in monorepo
  mode, Automatic Analysis off on each, the GitHub app installed on this
  repository, and two repository secrets, `SONAR_TOKEN` and `SONAR_ORG`.
  UI-only steps are
  recorded as verdicts in `tasks.md`, never as values.
- `platform/console/ui/package.json` — a coverage reporter dev dependency for
  vitest, if one is not already transitively present.

**Documentation made untrue — reference half:**
- `CONTRIBUTING.md` — the "Build and test" section gains a "Code analysis"
  subsection beside "The image scan", and the table under "What reports
  through it" gains the analysis step's row. It currently says the image
  scan is the only analysis a pull request gets.
- `docs/security.md` — the paragraph naming the two Trivy scans as what
  reports to the security tab; SonarCloud's security hotspots are a third
  reporter, on its own site, and the page must say where they are and that
  they are not a gate.
- `.claude/rules/build-test.md` — how a coverage profile is produced locally
  with the same flags CI uses, so a local number matches the dashboard's.
- `docs/CHANGELOG.md` — no entry. Nothing an installed release does changes.

**Documentation made untrue — adopter site:**
- None. The landing page, `introduction.md`, `getting-started.md`,
  `installation.md`, the integration pages and the guides describe what an
  ADOPTER installs and operates, and this change alters how the project is
  DEVELOPED. Each page was checked for a sentence about CI or code quality;
  the site makes no claim about either. Stated so the documentation task
  ticks it deliberately rather than skipping it.
