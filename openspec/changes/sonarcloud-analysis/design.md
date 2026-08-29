## Context

See `proposal.md` — Why. What shapes the approach:

- **The component list is derived.** `.github/components.sh images` returns
  `{component, context, dockerfile}` per directory, and `ci.yml`'s `discover`
  job filters it to what a diff touched. Every matrix reads that output; none
  lists a component.
- **Three test toolchains, three coverage formats.** Go emits a
  `coverprofile`; vitest emits lcov through `@vitest/coverage-v8`; `node --test`
  emits lcov natively on Node 22 (`--experimental-test-coverage` with
  `--test-reporter=lcov`). SonarCloud reads all three
  (`sonar.go.coverage.reportPaths`, `sonar.javascript.lcov.reportPaths`,
  `sonar.typescript.lcov.reportPaths`).
- **One component holds two languages.** `platform/console/` is a Go module
  embedding a TypeScript application; `components.sh` sees one directory and
  publishes one image. The analysis follows the same rule.
- **Secrets are withheld from fork workflows.** The review workflow already
  skips visibly for that reason, and `ci-green` treats SKIPPED as passed.
- **The image scan set the migration order.** Measure, fix what is fixable,
  enable the gate last — and record counts, never findings.

## Goals / Non-Goals

**Goals:**
- Every component analysed on every pull request that touched it and on every
  push to `master`, under a project named for the image it publishes.
- Coverage from the tests CI already runs, with no test run twice.
- A pull request sees the verdict where it sees the review: on the pull request.
- Adding a component adds a project with no workflow edit.

**Non-Goals:**
- **Gating the merge on the quality gate.** Not in this change; D5.
- **Analysing the chart, `.github/scripts/`, `docs/` or the openspec tree.**
  None is a component. A project for the chart's Go templates or the Python
  scripts would be a project with no image behind it, and the naming rule that
  makes the list derivable would have its first exception.
- **Replacing the review workflow.** SonarCloud reports rule findings; the
  review reads intent against this repository's rules. Different questions.
- **Screenshot, e2e or Playwright coverage.** Those suites need a browser and
  run deliberately, not on every pull request; their coverage is not a number
  CI can produce.

## Decisions

### D1. One project per component, in SonarCloud monorepo mode

A single project over the whole repository was the alternative, and it is what
SonarCloud's default setup produces. Rejected:

- One project has ONE quality gate and ONE new-code baseline across fifteen
  modules that release independently. A coverage drop in `signals/ha` would
  fail a pull request to `platform/console`.
- One project has one language configuration. Fifteen `go.mod` roots and one
  `package.json` under one `sonar.sources` need every exclusion typed once for
  all and revised for each.
- The pull request check would read `agent-ops-operator` and say nothing about
  which component failed; every other gate here names its component
  (`images (<component>)`, `modules (<path>)`).

Monorepo mode is SonarCloud's own answer: several projects bound to one GitHub
repository, each decorating the pull request under its own name. **The key and
the name are derived from `components.sh`:**

| | Value |
|---|---|
| project key | `<org>_agent-ops-operator_<component>` |
| project name | `agentops-<component>` — the image name |
| `sonar.projectBaseDir` | the component's `context` |

`<org>` is a repository variable, never typed in the workflow.

### D2. The analysis is a STEP of the job that tests the component, not a job

The first version was a `sonar` matrix job over `discover.outputs.images`,
fed by coverage artifacts the test jobs uploaded. It worked, and it was
deleted for what it needed: an upload in four jobs, a download with a
pattern, a script transforming a path into an artifact name that both sides
had to call, a second full checkout per component — and a failure mode that
fails GREEN (a name mismatch submits an analysis with coverage at zero).

The job that built and tested a component already has the checkout, the
toolchain and the profile on disk. `.github/actions/sonar-scan` is called as
that job's last step:

| Job | Component | Coverage on disk |
|---|---|---|
| `operator` | `manager` | `coverage.out` |
| `modules (<path>)` | derived from the path through `components.sh images` | `coverage.out`; for the console also `ui/coverage/lcov.info`, produced in the same leg |
| `node-runtimes (<runtime>)` | `runtime-<runtime>` | `coverage.lcov` |

So the changed-only filter is inherited rather than re-derived, a skipped test
job means a skipped analysis (nothing to measure), and a failed submission
fails the job that would have been green — which is the gate.

**`console-ui` stays a plain test-and-build job.** The console is ONE
component and one project; its UI coverage is produced in the `modules` leg
for `platform/console`, beside the Go profile, so one analysis carries both.
The UI suite therefore runs twice per pull request (seconds); the alternative
was two analyses of one project per commit, the second overwriting the first.

### D3. Coverage is read from where the tests wrote it

Each test invocation gains its coverage flag and nothing else changes:

| Toolchain | Invocation | Writes |
|---|---|---|
| Go | `go test -count=1 -coverprofile=coverage.out ./...` | `coverage.out` |
| vitest | `vitest run --coverage --coverage.reporter=lcov` | `ui/coverage/lcov.info`, then `SF:` re-anchored to `ui/` |
| `node --test` | `--experimental-test-coverage --test-reporter=lcov --test-reporter-destination=coverage.lcov`, with `spec` kept for the log | `coverage.lcov` |

The action names every report path a component can have; a path that is not
there is a scanner INFO line, so a component without tests is analysed with no
coverage figure — the true statement.

### D4. Properties are passed as arguments, not fifteen `sonar-project.properties` files

Fifteen byte-identical files was the state nine Dockerfiles were in before
`go-module.Dockerfile`, and the tell was an edit that had to be scripted across
all of them. The job passes `-Dsonar.projectKey`, `-Dsonar.projectBaseDir` and
the exclusions on the command line, derived from the matrix entry.

**The exclusions are one set, stated once:**

| Excluded | Because |
|---|---|
| `**/zz_generated.deepcopy.go` | controller-gen output |
| `**/node_modules/**`, `**/dist/**` | vendored and built |
| `**/*_test.go`, `**/*.test.{ts,tsx,js}` | declared as TESTS (`sonar.test.inclusions`), not excluded — Sonar analyses tests for test-specific rules |
| `platform/console/ui/{e2e,screenshots,demo}/**` | Playwright harnesses, run deliberately |

A component that needs an exception is not a reason to add a properties file;
it is a reason to name the exception in the one place with a comment.

### D5. The scanner's failure is a gate; the quality gate's verdict is not — yet

Two different failures, and only one reports through `ci-green`:

| Failure | Reports as | Blocks the merge |
|---|---|---|
| the scanner did not run, or could not submit | the component's test job red, through `ci-green` | yes — a gate that silently did nothing is the failure `ci-green` exists to catch |
| the quality gate failed on the submitted analysis | SonarCloud's own check on the pull request | **no** |

The default "Sonar way" gate requires ≥ 80 % coverage on new code and zero new
issues. Nobody has measured this tree against either. Enabling the verdict as a
required check first means the first red build belongs to whoever opens the
next unrelated pull request — exactly the image scan's D-Migration, and the same
answer: measure on the first runs, decide from counts, and enable a gate in a
later change that names what it enforces. `sonar.qualitygate.wait` stays
false so the job's exit code means "submitted".

**Enabling it later is a one-line change** — `sonar.qualitygate.wait=true`
makes the step fail with the gate, and the job already reports through `ci-green`.

### D6. CI-based analysis, with Automatic Analysis off per project

SonarCloud offers to analyse the repository itself, with no workflow. It is off
for every project because it is exclusive with CI-submitted analysis (both on
is a hard error at submission), reads no coverage report, and has no monorepo
mode. It is a per-project setting in the UI; the task records that it was done.

### D7. A fork's pull request skips the step, visibly

`if: github.event_name != 'pull_request' || github.event.pull_request.head.repo.fork == false`
on the step, so a fork's pull request runs the tests and shows the analysis
step as skipped. The secret is absent there and the scanner would fail on a
401 that reads as a broken workflow. The review workflow's rule, one step over.

`pull_request_target` would have handed the secret to fork code and was not
considered further.

### D8. The GitHub app, not the token, decorates the pull request

The scanner submits with `SONAR_TOKEN`; the pull request comment and check come
from the SonarCloud GitHub app installed on the repository. Two grants, and the
job holds only the first. `GITHUB_TOKEN` stays `contents: read` on this job —
the scanner needs no GitHub permission, and the app's are its own.

`fetch-depth: 0` on the checkout, because new-code detection and blame need
the history; the `discover` job carries the same comment for the same reason.

### D9. The job refuses a project that does not exist, and never provisions one

Found on the first run, not planned. The scanner auto-provisions any key it
cannot find when the token may create projects — and a project created that
way is bound to NO repository: no pull request decoration, no monorepo
membership, while looking exactly like one that was set up. Fifteen of those
were created by the first run, deleted, and created again by the next push
before the check existed.

So the job asserts the project exists before scanning and fails naming the
component otherwise. Creation is a deliberate step, `.github/scripts/sonar-provision.sh`,
which posts the monorepo wizard's own request — the public
`api/projects/create` makes the same unbound project the scanner does.
`is_bound_to_monorepo` is how the result was verified: `true` on all fifteen.

## Risks / Trade-offs

- **[Fifteen projects to create by hand]** → SonarCloud's web API
  (`api/projects/create`) takes org, key and name; a loop over
  `components.sh images` creates them in one go, and a new component is one
  more call — noted in `CONTRIBUTING.md` beside the tag-and-package steps a
  new component already owes. Monorepo binding and Automatic Analysis are UI,
  once each.
- **[A coverage path is wrong]** → it fails GREEN: the analysis submits with
  coverage at zero. Task 3 reads the scanner's own coverage-sensor lines on a
  real run, per toolchain, rather than trusting a green job.
- **[A component with no tests reports 0 % and looks broken]**
  `gateways/telegram`, `signals/telegram` and `platform/context-sync` may have
  no `_test.go`. Accepted: 0 % is the fact, and the gate is not required.
- **[The first master push analyses only what the diff touched]** → the
  change edits `ci.yml`, which is one of the four rebuild-everything paths,
  so the merge itself analyses all fifteen and lands every baseline.
- **[Coverage instrumentation slows the test jobs]** → `-coverprofile` on
  Go is a few percent; vitest's v8 provider is native. The operator job is
  dominated by envtest provisioning either way.
- **[`fetch-depth: 0` on every test job]** → the repository is small; the
  `discover` job already pays it once per run.
- **[SonarCloud outage fails every pull request]** → the step fails the test
  job, so an outage is red. Accepted over the alternative (`continue-on-error`),
  which would make a scanner that never runs indistinguishable from one that
  did — the image scan's `trivy-db` is best-effort because a MISS still scans;
  here a miss submits nothing.

## Migration Plan

1. Create the organisation binding, the fifteen projects, monorepo mode and
   Automatic Analysis off (tasks 1). Nothing in CI changes yet.
2. Land the workflow. Its own merge analyses everything and sets baselines.
3. Read the first dashboards; record counts per component in the task, never
   findings.
4. Gating the quality gate is a SEPARATE change, if the counts warrant it.

Rollback: delete the `sonar` job and the coverage steps; the projects can stay
or be deleted, and nothing in a release is affected either way.

## Open Questions

- Whether SonarCloud's Go analyser reads a `coverprofile` produced with
  `-covermode=atomic` identically to the default `set`. Settled by the first
  run; changes nothing in the specs or the tasks.
