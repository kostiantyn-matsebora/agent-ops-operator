## 1. Coverage across packages (design D1)

- [ ] 1.1 Record the BEFORE: every component's coverage from `api/measures/component?metricKeys=coverage`, as a table of numbers in this file's task 1.1 (no identifier, no url — the org is a secret). Verify: fourteen rows, one per component `components.sh images` lists.
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
- [ ] 2.2 Run the script against the organisation with the user token, then read back `api/qualitygates/show?name=agentops` and one project's `api/qualitygates/get_by_project`. Verify: seven conditions, default, every project assigned; a second run creates nothing new.

## 3. Unit tests

- [x] 3.1 New `.github/tests/sonar-provision.test.sh` with `curl` stubbed from fixtures: an org with no gate creates one and every condition; an org with the gate and six conditions adds only the coverage condition; a gate already complete and default makes no create call; a 403 fails naming the permission. Wired into `run.sh`. Verify: `.github/tests/run.sh` passes. DONE — 22 assertions, all passing. `run.sh` globs `*.test.sh`, so the file is wired by being there. MUTATION-CHECKED: replacing `update_condition` with `create_condition` fails "does NOT create a duplicate condition on that metric" and nothing else, so the drift case has teeth rather than passing by construction.
- [x] 3.2 Run the suite and both guards from the worktree: `.github/tests/run.sh`, `python3 .github/scripts/publication-guard.py`, `python3 .github/scripts/retired-vocabulary-guard.py`. Verify: all three exit 0. DONE 2026-08-29 — `.github/tests/run.sh` "every script test passed", `publication-guard.py` clean, `retired-vocabulary-guard.py` clean (108 files). All three exit 0.

## 4. E2E tests

- [x] 4.1 Not applicable: nothing here is decided by a cluster — a CI flag and a provisioning script against the analysis service. The live proof is the first CI run of this branch (the manager's analysis reports the corrected number) and task 2.2's read-back. Verify: the pull request's SonarCloud checks show the corrected coverage and the `agentops` gate's verdict. DONE — the section's claim stands: no cluster decides any of this. The remaining live proof is task 2.2's read-back and the branch's first SonarCloud checks.

## 5. Documentation

### 5.1 Reference docs

- [x] 5.1.1 `.claude/rules/build-test.md`, "Coverage, with the flags CI uses": the Go row's command carries `-coverpkg=./...`, and one bullet says what the flag changes and what the 27% was. Verify: the table and the bullet are there. DONE — the Go row carries the flag, and the bullet above the analysis one states what `-coverpkg` records, the 27% dashboard / 21.7% local number it replaces, that a package no test reaches now reads 0% rather than absent, and the build-time cost.
- [ ] 5.1.2 Record the AFTER per component beside task 1.1's table, from the first master analysis after merge. Verify: fourteen rows, and the manager's row moved.
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
