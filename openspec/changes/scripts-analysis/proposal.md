## Why

The scripts under `.github/scripts/` decide things — they refuse commits,
resolve review threads, build the review's queue, open issues — and they are
the one body of code in this repository that CI tests without measuring: the
suite runs as a step inside `docs-task`, produces no coverage, and no
SonarCloud project sees a line of it. Every image-publishing component is
analysed; the code that judges the pull requests is not.

## What Changes

- **`.github/scripts` gets its own CI job, `scripts`.** It runs
  `.github/tests/run.sh`, produces Python coverage for the scripts — the tests
  drive them as subprocesses from bash, so coverage is collected at
  interpreter start-up in every process the suite spawns, then combined — and
  analyses the unit on SonarCloud as the job's last step, exactly as
  `operator`, `modules` and `node-runtimes` do. It reports through `ci-green`.
- **`docs-task` keeps only the documentation gate.** The script suite moves
  out of it; the step proving the hook and the check agree stays part of the
  suite (`run.sh` calls `docs-task-guard-test.sh`).
- **The analysis action learns to analyse a unit that is not an image.**
  `.github/actions/sonar-scan` takes an explicit `name` for a unit
  `components.sh` does not list, and overrides for what counts as sources and
  tests; its defaults gain the Python coverage report path. Every existing
  call site is unchanged.
- **The provisioning script provisions the extra project** `scripts`, under
  the same key and name pattern (`agentops-scripts`), beside the component
  list — so the "project must exist" assertion holds for it too.

## Capabilities

### New Capabilities

_None._

### Modified Capabilities

- `code-quality-analysis`: the workflow's own scripts are analysed as one
  unit, named explicitly rather than derived from the image list, with
  coverage from the script suite; the docs gate job no longer carries the
  suite.

## Impact

**Code:** `.github/workflows/ci.yml` (new `scripts` job, `discover` output
for it, `docs-task` shrinks, `ci-green` needs it); `.github/actions/sonar-scan`
(inputs `name`, `sources`, `tests`, `test-inclusions`; Python report path);
`.github/scripts/sonar-provision.sh` (the extra project); a coverage
configuration file under `.github/`; a test for the provisioning script's
extra entry under `.github/tests/`.

**In flight beside it:** `coverage-across-packages` edits the same two files
(`ci.yml`'s Go test steps, `sonar-provision.sh`'s gate stage). Different
hunks; whichever lands second rebases.

**Reference docs made untrue:** `openspec/specs/code-quality-analysis/spec.md`
(the delta folds in at archive); `.claude/rules/build-test.md`, "Coverage,
with the flags CI uses" (a Python row). `.claude/rules/change-tests.md`
names `.github/tests/run.sh` as the unit tier for a workflow script — still
true. `docs/CHANGELOG.md` is untouched: nothing ships in the chart or an
image.

**Adopter site:** nothing under `docs/` describes the analysis or the
`docs-task` job. `CONTRIBUTING.md` is the contributor's page and is updated
in two places: "Code analysis" (the scripts unit and how it is named) and the
`ci-green` table (the `scripts` row, and `docs-task`'s row no longer covering
the suite). `README.md` is untouched.
