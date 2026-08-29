## 1. Coverage across packages (design D1)

- [ ] 1.1 Record the BEFORE: every component's coverage from `api/measures/component?metricKeys=coverage`, as a table of numbers in this file's task 1.1 (no identifier, no url — the org is a secret). Verify: **fifteen** rows, one per component `components.sh images` lists.

  **OPEN — the API read needs the user's token.** `SONAR_TOKEN` and
  `SONAR_ORG` exist as repository secrets and are write-only, so no session
  can read the dashboard's numbers without being handed one. What follows is
  the LOCAL measurement of the same runs CI feeds the analysis from, produced
  in the `golang:1.25` container on 2026-08-29, and it is not a substitute
  for the read-back: SonarCloud applies its own exclusions and counts a file
  absent from the profile as uncovered, so its per-component figure is at or
  below each of these.

  **THE VERIFY CLAUSE ABOVE SAID `fourteen`, AND WAS CORRECTED IN PLACE.**
  `components.sh images` lists fifteen, and the archived `sonarcloud-analysis`
  recorded fifteen projects created, so the plan was off by one. Keeping the
  wrong number visible "as the plan wrote it" was tried and reverted: two
  review rounds read it as an inconsistency, and a task file is an instruction
  to the next session before it is a record of what the last one found. The
  finding is kept here, in prose, where it cannot be mistaken for a count to
  meet.

  | Component | Toolchain | Before | With `-coverpkg` |
  |---|---|---|---|
  | `channel-telegram` | Go | 70.9% | 70.9% |
  | `console` | Go **and** vitest | 74.8% Go, 82.9% lcov (2088/2519 lines) | 74.8% Go |
  | `context-sync` | Go | 68.6% | 68.6% |
  | `egress-proxy` | Go | 62.3% | 62.3% |
  | `gateway-telegram` | Go | 56.2% | 56.2% |
  | `housekeeping` | Go | 41.7% | 41.7% |
  | `manager` | Go | **21.7%** | **70.7%** |
  | `runtime-claude` | `node --test` | 99.2% of LOADED files | — |
  | `runtime-copilot` | `node --test` | 99.6% of LOADED files | — |
  | `runtime-ollama` | Go | 68.5% | 68.5% |
  | `signal-alertmanager` | Go | 74.0% | 74.0% |
  | `signal-cron` | Go | 28.7% | 28.7% |
  | `signal-ha` | Go | 72.2% | 72.2% |
  | `signal-k8s-events` | Go | 78.1% | 78.1% |
  | `signal-telegram` | Go | 56.2% | 56.2% |

  **FIFTEEN ROWS, AND THE CONSOLE IS ONE OF THEM.** It is one component with
  two toolchains and ONE project, so its Go and vitest figures share a row
  rather than making a sixteenth: SonarCloud combines both reports into a
  single coverage number for `agentops-console`. Splitting it looked tidier
  and made the table disagree with its own verify clause.

  **THE FLAG MOVES EXACTLY ONE COMPONENT, WHICH IS THE DESIGN'S CLAIM
  MEASURED RATHER THAN ARGUED.** Every other Go module is a single-package
  suite, so `-coverpkg=./...` names the package it already recorded and the
  number is byte-identical. D1's "the flag costs them nothing and stops being
  a special case the manager alone carries" is the whole justification for
  putting it on both steps, and this table is it.

  **THE TWO NODE RUNTIMES' FIGURES ARE NOT COMPARABLE TO THE REST.**
  `node --test --experimental-test-coverage` reports only files the suite
  LOADED, so a module no test imports is absent rather than 0% — which is why
  both read over 99% while their projects will not. The lcov CI submits has
  the same shape, so the dashboard's number for these two is the one that
  matters and this row cannot stand in for it.

  **The gap the follow-up changes start from**, against the 80% condition:
  `signal-cron` (28.7%), `housekeeping` (41.7%), `gateway-telegram` and
  `signal-telegram` (56.2%), `egress-proxy` (62.3%), `runtime-ollama` (68.5%),
  `context-sync` (68.6%), `channel-telegram` (70.9%), `signal-ha` (72.2%),
  `signal-alertmanager` (74.0%), `console` (74.8% Go), `signal-k8s-events`
  (78.1%). The manager clears it on the corrected number.
- [x] 1.2 `.github/workflows/ci.yml`: the `operator` and `modules` test steps run `go test -count=1 -coverpkg=./... -coverprofile=coverage.out ./...`; the comment above each says why the flag is there. Verify: `grep -c 'coverpkg=./...' .github/workflows/ci.yml` prints 2.
- [x] 1.3 In the worktree's build container, run the manager's suite with the new flags and read `go tool cover -func=coverage.out | tail -1` before and after `-coverpkg`, recording both totals in this task. Verify: the after number is higher, and packages absent from the old profile appear in the new one.

  MEASURED 2026-08-29, `platform/manager`, envtest 1.31.0, `golang:1.25`
  container. `go tool cover -func | tail -1`: **21.7% before, 70.7% after.**
  Per package, statement-weighted, blocks deduplicated by highest count
  (`-coverpkg` writes one block set per test binary, so a naive sum
  double-counts):

  | Package | Statements | Before | After |
  |---|---|---|---|
  | `api/v1alpha1` | 1115 | 3.5% | 47.2% |
  | `cmd/manager` | 80 | 3.8% | 3.8% |
  | `internal/activity` | 98 | 88.8% | 89.8% |
  | `internal/addressing` | 9 | 77.8% | 77.8% |
  | `internal/chat` | 653 | 50.2% | 81.3% |
  | `internal/configschema` | 62 | 85.5% | 87.1% |
  | `internal/controller` | 1080 | 8.1% | 77.5% |
  | `internal/dispatch` | 106 | 84.9% | 88.7% |
  | `internal/httpapi` | 1234 | 1.5% | 74.6% |
  | `internal/ingest` | 48 | 83.3% | 83.3% |
  | `internal/mcpcompile` | 50 | 80.0% | 80.0% |
  | `internal/metrics` | 60 | 95.0% | 95.0% |
  | `internal/runtimepod` | 224 | 79.9% | 89.3% |
  | `internal/storagebreaker` | 31 | 67.7% | 93.5% |
  | **total** | **4850** | **21.7%** | **70.7%** |

  **The second half of the verify does NOT hold for this module, and the
  measurement is what says so.** The package SET is identical — fourteen
  packages either side. Every package of the manager carries tests of its
  own, so none was ever absent from the profile; what was absent was the
  ATTRIBUTION, and the three packages the envtest suite drives are where the
  whole move is (`internal/httpapi` 1.5% → 74.6%, `internal/controller` 8.1%
  → 77.5%, `internal/chat` 50.2% → 81.3%). The one package of the fifteen
  `go list ./...` reports that appears in neither profile is
  `internal/integration`, which holds no non-test file and therefore no
  statement to record. The claim is kept as written and answered rather than
  quietly reworded: a module whose packages all carry tests gains no rows,
  only true numbers.

  **Build time did not move measurably here** — 43.7 s before, 42.0 s after,
  both with a warm module and build cache — so the design's cost is left to
  the branch's first CI run, on a cold cache, as it says.

## 2. The gate, provisioned (design D2)

- [x] 2.1 `.github/scripts/sonar-provision.sh`: a second stage — find or create the gate `agentops` (`api/qualitygates/list`/`create`), copy every condition of the built-in `Sonar way` from its `show` response, add `coverage LT 80`, update rather than duplicate an existing condition on the same metric, `set_as_default`, and `select` every component project from the same `components.sh` list. A 403 fails naming the permission the token lacks (*Administer Quality Gates*); the header lists it. Verify: a dry read of the script shows each step keyed by lookup, and `sh -n` passes. DONE — `sh -n` passes; every write is preceded by its lookup (`list` for the gate, the gate's own `conditions[]` for each metric, `get_by_project` for each project), and `sq` reads the status with `-w` rather than `curl -sf` so a 403 can be told from anything else and name the permission.
- [ ] 2.2 **OPEN — needs the user's token, and is theirs to run.** Running the script writes to their SonarCloud organisation: it creates a gate, makes it the organisation DEFAULT and reassigns every project. That is an outward-facing change to a live service, so it is not something a session does on its own even with a credential in hand. Run the script against the organisation with the user token, then read back `api/qualitygates/show?name=agentops` and one project's `api/qualitygates/get_by_project`. Verify: seven conditions, default, every project assigned; a second run creates nothing new.

## 3. Unit tests

- [x] 3.1 New `.github/tests/sonar-provision.test.sh` with `curl` stubbed from fixtures: an org with no gate creates one and every condition; an org with the gate and six conditions adds only the coverage condition; a gate already complete and default makes no create call; a 403 fails naming the permission. Wired into `run.sh`. Verify: `.github/tests/run.sh` passes. DONE — 22 assertions, all passing. `run.sh` globs `*.test.sh`, so the file is wired by being there. MUTATION-CHECKED: replacing `update_condition` with `create_condition` fails "does NOT create a duplicate condition on that metric" and nothing else, so the drift case has teeth rather than passing by construction.
- [x] 3.2 Run the suite and both guards from the worktree: `.github/tests/run.sh`, `python3 .github/scripts/publication-guard.py`, `python3 .github/scripts/retired-vocabulary-guard.py`. Verify: all three exit 0. DONE 2026-08-29 — `.github/tests/run.sh` "every script test passed", `publication-guard.py` clean, `retired-vocabulary-guard.py` clean (108 files). All three exit 0.

## 4. E2E tests

- [x] 4.1 Not applicable: nothing here is decided by a cluster — a CI flag and a provisioning script against the analysis service. The live proof is the first CI run of this branch (the manager's analysis reports the corrected number) and task 2.2's read-back. Verify: the pull request's SonarCloud checks show the corrected coverage and the `agentops` gate's verdict. DONE — the section's claim stands: no cluster decides any of this. The remaining live proof is task 2.2's read-back and the branch's first SonarCloud checks.

## 5. Documentation

### 5.1 Reference docs

- [x] 5.1.1 `.claude/rules/build-test.md`, "Coverage, with the flags CI uses": the Go row's command carries `-coverpkg=./...`, and one bullet says what the flag changes and what the 27% was. Verify: the table and the bullet are there. DONE — the Go row carries the flag, and the bullet above the analysis one states what `-coverpkg` records, the 27% dashboard / 21.7% local number it replaces, that a package no test reaches now reads 0% rather than absent, and the build-time cost.
- [ ] 5.1.2 Record the AFTER per component beside task 1.1's table, from the first master analysis after merge. Verify: **fifteen** rows, and the manager's row moved.

  **The count is corrected here for the same reason it is in 1.1**, which
  measured it: `components.sh images` lists fifteen components, and the
  archived `sonarcloud-analysis` recorded fifteen projects created. Two
  clauses saying fourteen would have been one correction that only took in
  half the file.
- [ ] 5.1.3 The delta spec is archived into `openspec/specs/code-quality-analysis/spec.md` by `/opsx:archive` on the branch; `openspec validate --all` passes. Verify: the command exits 0.

### 5.2 Adopter site

- [x] 5.2.1 `CONTRIBUTING.md`, "Code analysis": what the gate requires (80% of the whole component plus the new-code conditions), that it is provisioned by the script and still not required by branch protection, and that a red gate on a component under 80% is expected until its coverage change lands. No page under `docs/` describes the analysis, so the site carries nothing to update — stated here so the absence is a claim rather than an omission. Verify: the paragraph names the threshold and the script, and `wc -l README.md` is unchanged. DONE — two paragraphs added under *what does not fail your pull request*: what the gate requires (the built-in new-code conditions plus overall coverage at 80%, provisioned by `sonar-provision.sh` and still not required by branch protection) and why a red gate under 80% is expected. The provisioning paragraph gains the second permission. `wc -l README.md` is 211, unchanged.

  **The absence under `docs/` is checked rather than assumed, and it is not
  total.** `docs/security.md`'s own "Code analysis" section describes hotspot
  REPORTING — "reported and not gated" — which this change leaves true, since
  branch protection still reads no verdict. No page under `docs/` mentions
  coverage or a quality gate at all (`grep -rniE 'sonar|quality gate|coverage'
  docs/` returns that one file, that one line), so the adopter site carries
  nothing this change made untrue.
