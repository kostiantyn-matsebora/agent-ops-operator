## Context

See proposal.md — Why. What shapes the approach:

- **`sonar-scan` names its project by asking `components.sh`**, and refuses a
  directory it does not list. `.github/scripts` is not a component and must
  not become one: `components.sh` takes the union of `go.mod` and `Dockerfile`
  directories, and listing the scripts there would publish an image.
- **The suite is bash driving Python.** `.github/tests/*.test.sh` run each
  script as a child process; `coverage run <script>` would measure nothing,
  because the process under measurement is the shell.
- **The action's property set is fixed** — `sources=.`, `tests=.`, test
  inclusions for Go/TS/JS — and its coverage paths name Go and lcov shapes.
  Sonar's Python analyser reads Cobertura XML from
  `sonar.python.coverage.reportPaths`.
- **`docs-task` runs unconditionally** and ran the suite on every pull
  request; the component jobs run only for what `discover` says changed.

## Goals / Non-Goals

**Goals:**

- One job per unit, the analysis as its last step, coverage from the tests
  that ran — the shape every other analysed job has.
- No call site of `sonar-scan` changes; a non-component is the exception and
  says so at its own call.

**Non-Goals:**

- Analysing the workflow YAML, the composite actions or the hooks as source.
  The unit is the scripts; the rest of `.github/` is exercised, not measured.
- Shell coverage. SonarCloud has no shell analyser; the `.sh` scripts are
  analysed for nothing and the suite is what verifies them.

## Decisions

**D1 — the unit is named `scripts`, explicitly.** Project key
`<org>_agent-ops-operator_scripts`, name `agentops-scripts` — the component
pattern, so the dashboard reads as one list. The name is an INPUT to the
action, and the action skips the `components.sh` lookup only when it is given.
Alternative rejected: teaching `components.sh` a "non-image unit" class — a
second kind of thing in the one program whose whole contract is "what
publishes".

**D2 — the analysis base directory is `.github`, sources `scripts`, tests
`scripts,tests`, test inclusions `**/*.test.sh,**/*-test.sh`.** Base `.github`
so the tests are inside the analysed tree; sources narrowed to the scripts so
the workflows and actions are not counted as uncovered code. The two
directories overlap on `docs-task-guard-test.sh`, which the inclusions
classify as a test and the scanner then drops from sources, the same way the
default `sources=.`/`tests=.` pair already works.

**D3 — coverage at interpreter start-up, combined afterwards.** `coverage.py`
installed on the runner; a `.pth` file in its site-packages calling
`coverage.process_startup()`; `COVERAGE_PROCESS_START` naming a committed
`.github/.coveragerc` (`parallel`, `source = scripts`); `COVERAGE_FILE` set
absolute so every process, whatever its working directory, writes beside the
others. After the suite: `coverage combine` then `coverage xml`. This is
coverage.py's own documented subprocess mechanism. Alternatives: wrapping
`python3` on `PATH` with a shim — the same thing, hand-rolled; rewriting the
tests to import the scripts — a rewrite of the suite to measure it.

**D4 — a `scripts` output on `discover`.** True when the diff touches
`.github/scripts/`, `.github/tests/` or `.claude/hooks/`, and on the
everything paths (`ci.yml` is one already). The job is conditional on it.
This narrows the suite from "every pull request" to "pull requests that could
change its verdict", which is what the component jobs do and what a coverage
baseline per push to master wants.

**D5 — `docs-task` loses the suite and nothing else.** The guard step is the
job's purpose; the suite was there because it had no home.

**D6 — provisioning appends one entry, and CI calls it for a missing
project.** `sonar-provision.sh` takes names as a filter over its own list —
the components plus `scripts` — and the analysis step, finding no project,
runs it with the job's token and checks again before scanning. The
published requirement "CI never creates a project" is MODIFIED to "never by
a submission": what it guarded against was the scanner's auto-provisioning,
which binds to nothing, and the monorepo call from CI is the same deliberate
path a person takes. Alternative rejected: a person running the script once
per new unit — the first run of every new component failed on it, and a
failure whose fix is "somebody with a token" is a step somebody forgets.

## Risks / Trade-offs

- [The `.pth` hook measures every Python process on the runner for the
  suite's duration, including the guards a test invokes] → `source = scripts`
  confines what is recorded; anything else is dropped at combine.
- [`coverage-across-packages` edits `sonar-provision.sh` and `ci.yml` too] →
  different hunks; the second to land rebases. Stated in the proposal.
- [A fork's pull request has no token] → the analysis step carries the same
  fork guard as every other; the suite still runs.
- [The first run finds no project] → the step provisions it inside the
  binding and re-checks; only a provisioning that returned without producing
  the project fails, naming `sonar-provision.sh` for a run by hand.
