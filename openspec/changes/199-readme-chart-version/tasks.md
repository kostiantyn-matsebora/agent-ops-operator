## 1. README

- [x] 1.1 In `README.md`'s "Try it" section, add `--version 13.4.0` to the
  `helm install agent-ops oci://ghcr.io/kostiantyn-matsebora/charts/agent-ops-operator`
  command, matching `chart/Chart.yaml`'s current `version:`. Verify: the line
  reads `--version 13.4.0` and `wc -l README.md` grows by at most one line
  (this is a one-line edit inside an existing code block, not a new section).

  Done. `wc -l README.md` unchanged (204 lines) — the flag was added to the
  existing `helm install` line, not a new one.

## 2. Unit tests

- [x] 2.1 Run `python3 .github/scripts/docs-generate.py --check` in this
  working copy's tree (the session's clone with `change/199-readme-chart-version`
  checked out, not `master`). Verify: exits 0 and reports `README.md`'s new
  `--version 13.4.0` as up to date against `chart/Chart.yaml` — this is the
  existing `check_versions()` machinery `design.md` relies on, exercised for
  real against this change rather than assumed.

  Ran on this branch, after the README edit: `49 generated file(s) up to
  date`, exit 0.
- [x] 2.2 Run `bash .github/tests/docs-generate.test.sh` in this working
  copy's tree. Verify: exits 0 (`--check` reports up to date, and neither
  read-only mode touches `docs/`) — this is the test suite's own real-checkout
  exercise of `check_versions()`, and it is what actually covers this
  change's edit; no new test file is added since the existing suite already
  runs the real generator against this exact checkout, README included.

  Ran on this branch: 4 passed, exit 0.
- [x] 2.3 Run `.github/tests/run.sh` (the routine's own step 5). Verify:
  `docs-generate.test.sh` (this change's own coverage) passes; any other
  failure is pre-existing and unrelated.

  `cloud-bootstrap.test.sh` reports 3 of 10 failed — reproduced identically
  on `origin/master` with this change's `README.md` edit stashed (same three
  cases, same "workstation marker" assertions), so it predates and is
  unrelated to this change: this sandbox's own ambient environment leaks
  into the test's "unset `CLAUDE_CODE_REMOTE`" subshell. Every other suite in
  the run passed, `docs-generate.test.sh` (task 2.2) included.

## 3. E2E tests

- [x] 3.1 Not applicable: this change edits one line of static markdown in
  `README.md`. Nothing here is decided by a cluster — no CRD field, RBAC
  rule, pod lifecycle, informer or context-continuity behavior is touched.

## 4. Documentation

### 4.1 Reference docs

- [x] 4.1.1 `docs/concepts.md` and `docs/contracts.md` describe CRD fields,
  semantics and contracts. Verify: neither page mentions README's install
  command or a chart version for it, so neither is made untrue by this
  change — confirmed by `git grep -n "README" docs/concepts.md
  docs/contracts.md` returning nothing relevant.

  Confirmed: no matches in either file.

### 4.2 Adopter site

- [x] 4.2.1 The landing page, `introduction.md`, `getting-started.md`,
  `installation.md` and `docs/guides/*` each describe their own install
  commands, not README's. Verify: `git grep -rn -- "--version" docs/*.md
  docs/*/*.md` shows every one of them already pins the chart's version
  (`13.4.0`) via the existing mechanism `check_versions()` covers — so this
  change introduces no gap on the site side, only closes the one README had.

  Confirmed: only `docs/installation.md` prints `--version`, already pinned
  at `13.4.0` (the two `--version <version>` lines are the generic upgrade
  placeholder, not a number). No other adopter page names a chart version.
