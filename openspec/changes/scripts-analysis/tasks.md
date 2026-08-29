## 1. The action analyses a named unit (design D1, D2)

- [x] 1.1 `.github/actions/sonar-scan/action.yml`: inputs `name` (optional — when set, the components.sh lookup is skipped and the comment says why a non-image unit is the exception), `sources` (default `.`), `tests` (default `.`), `test-inclusions` (default the current three globs); the args use them; `-Dsonar.python.coverage.reportPaths=coverage.xml` joins the defaults. Verify: `git diff` shows every existing call site's behaviour unchanged (defaults equal the old literals) and the action parses (`yq` or `python3 -c 'import yaml'`).

## 2. Coverage for subprocess-driven scripts (design D3)

- [x] 2.1 `.github/.coveragerc` — `[run] parallel = true`, `source = scripts`, `[xml] output = coverage.xml`, header comment saying it is read by `COVERAGE_PROCESS_START` in every process the suite spawns. Verify: the file exists and `python3 -m coverage --rcfile=.github/.coveragerc debug config` (in a venv with coverage) prints `source = scripts`.
- [x] 2.2 Locally, in a venv: install coverage, write the `.pth` hook, export `COVERAGE_PROCESS_START` and an absolute `COVERAGE_FILE`, run `.github/tests/run.sh`, `coverage combine`, `coverage xml`, and read `coverage report`. Verify: the report lists `scripts/*.py` files with non-zero coverage and `coverage.xml` exists; record the total as a number in this task. Recorded: 91% — 1026 statements, 95 missed, over the twelve scripts the suite drives (a `.pth` is honoured only in a site dir, so locally it was `sitecustomize.py` on `PYTHONPATH`; same hook, same result).

## 3. The job and its wiring (design D4, D5, D6)

- [x] 3.1 `.github/workflows/ci.yml`, `discover`: output `scripts`, true when the diff touches `.github/scripts/`, `.github/tests/` or `.claude/hooks/`, or when everything is built. Verify: the output is listed under `outputs:` and set in the same `run:` as the others.
- [x] 3.2 `.github/workflows/ci.yml`: new job `scripts` (needs `discover`, conditional on the output): checkout with full history, `actions/setup-python`, `pip install coverage`, the `.pth` hook, the suite under `COVERAGE_PROCESS_START`/`COVERAGE_FILE`, `coverage combine` + `coverage xml` in `.github`, then `./.github/actions/sonar-scan` with `dir: .github`, `name: scripts`, `sources: scripts`, `tests: scripts,tests`, `test-inclusions: '**/*.test.sh,**/*-test.sh'`, under the same fork guard as the other analysis steps. Header comment says why the unit is named rather than derived. Verify: the job is in `ci-green`'s `needs:` and `docs-task` no longer runs `run.sh`.
- [x] 3.3 `.github/scripts/sonar-provision.sh`: the `projects` array is the component list plus the `scripts` entry, same key and name pattern. Verify: `sh -n` passes and a dry `jq` of the body (curl stubbed) shows fifteen projects, the last keyed `_scripts`.
- [ ] 3.4 Run `sonar-provision.sh` once against the organisation with the user token. Verify: it prints the `scripts` key and the project exists (`api/projects/search`) — record "exists", never the key.

## 4. Unit tests

- [x] 4.1 `.github/tests/sonar-provision.test.sh` (new, or extended if `coverage-across-packages` landed first): with `curl` stubbed to echo its `--data`, the body holds every `components.sh images` component plus one `scripts` entry, and no other. Wired into `run.sh` by the glob. Verify: `.github/tests/run.sh` passes.
- [x] 4.2 From the worktree: `.github/tests/run.sh`, `python3 .github/scripts/publication-guard.py`, `python3 .github/scripts/retired-vocabulary-guard.py`. Verify: all three exit 0.

## 5. E2E tests

- [x] 5.1 Not applicable: nothing here is decided by a cluster — a workflow job, a composite action and a provisioning script. The live proof is the branch's first CI run: the `scripts` job runs the suite, submits the analysis, and `ci-green` lists it. Verify: the pull request's checks show `scripts` green and a SonarCloud check for `agentops-scripts` with a coverage figure. Partly verified on #138: the suite and the combined coverage report ran green on the runner; the analysis step failed on the project assertion exactly as designed, naming `sonar-provision.sh` — green once task 3.4 is done.

## 6. Documentation

### 6.1 Reference docs

- [x] 6.1.1 `.claude/rules/build-test.md`, "Coverage, with the flags CI uses": a Python row (the `.github/scripts` unit — the venv, the `.pth` hook, the two env vars, `combine` and `xml`) and one bullet on why start-up hooking rather than `coverage run`. Verify: the table has four rows.
- [ ] 6.1.2 The delta spec is archived into `openspec/specs/code-quality-analysis/spec.md` by `/opsx:archive` on the branch; `openspec validate --all` passes. Verify: the command exits 0.

### 6.2 Adopter site

- [x] 6.2.1 `CONTRIBUTING.md`, "Code analysis": the scripts unit is analysed under `agentops-scripts`, named explicitly because it publishes no image, and `sonar-provision.sh` provisions it with the rest; the `ci-green` table gains a `scripts` row and `docs-task`'s row says only the documentation gate. No page under `docs/` describes CI, so the site carries nothing to update — stated here so the absence is a claim rather than an omission. Verify: both places name `scripts`, and `wc -l README.md` is unchanged.
