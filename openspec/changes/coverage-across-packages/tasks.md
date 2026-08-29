## 1. Coverage across packages (design D1)

- [ ] 1.1 Record the BEFORE: every component's coverage from `api/measures/component?metricKeys=coverage`, as a table of numbers in this file's task 1.1 (no identifier, no url — the org is a secret). Verify: fourteen rows, one per component `components.sh images` lists.
- [ ] 1.2 `.github/workflows/ci.yml`: the `operator` and `modules` test steps run `go test -count=1 -coverpkg=./... -coverprofile=coverage.out ./...`; the comment above each says why the flag is there. Verify: `grep -c 'coverpkg=./...' .github/workflows/ci.yml` prints 2.
- [ ] 1.3 In the worktree's build container, run the manager's suite with the new flags and read `go tool cover -func=coverage.out | tail -1` before and after `-coverpkg`, recording both totals in this task. Verify: the after number is higher, and packages absent from the old profile appear in the new one.

## 2. The gate, provisioned (design D2)

- [ ] 2.1 `.github/scripts/sonar-provision.sh`: a second stage — find or create the gate `agentops` (`api/qualitygates/list`/`create`), copy every condition of the built-in `Sonar way` from its `show` response, add `coverage LT 80`, update rather than duplicate an existing condition on the same metric, `set_as_default`, and `select` every component project from the same `components.sh` list. A 403 fails naming the permission the token lacks (*Administer Quality Gates*); the header lists it. Verify: a dry read of the script shows each step keyed by lookup, and `sh -n` passes.
- [ ] 2.2 Run the script against the organisation with the user token, then read back `api/qualitygates/show?name=agentops` and one project's `api/qualitygates/get_by_project`. Verify: seven conditions, default, every project assigned; a second run creates nothing new.

## 3. Unit tests

- [ ] 3.1 New `.github/tests/sonar-provision.test.sh` with `curl` stubbed from fixtures: an org with no gate creates one and every condition; an org with the gate and six conditions adds only the coverage condition; a gate already complete and default makes no create call; a 403 fails naming the permission. Wired into `run.sh`. Verify: `.github/tests/run.sh` passes.
- [ ] 3.2 Run the suite and both guards from the worktree: `.github/tests/run.sh`, `python3 .github/scripts/publication-guard.py`, `python3 .github/scripts/retired-vocabulary-guard.py`. Verify: all three exit 0.

## 4. E2E tests

- [ ] 4.1 Not applicable: nothing here is decided by a cluster — a CI flag and a provisioning script against the analysis service. The live proof is the first CI run of this branch (the manager's analysis reports the corrected number) and task 2.2's read-back. Verify: the pull request's SonarCloud checks show the corrected coverage and the `agentops` gate's verdict.

## 5. Documentation

### 5.1 Reference docs

- [ ] 5.1.1 `.claude/rules/build-test.md`, "Coverage, with the flags CI uses": the Go row's command carries `-coverpkg=./...`, and one bullet says what the flag changes and what the 27% was. Verify: the table and the bullet are there.
- [ ] 5.1.2 Record the AFTER per component beside task 1.1's table, from the first master analysis after merge. Verify: fourteen rows, and the manager's row moved.
- [ ] 5.1.3 The delta spec is archived into `openspec/specs/code-quality-analysis/spec.md` by `/opsx:archive` on the branch; `openspec validate --all` passes. Verify: the command exits 0.

### 5.2 Adopter site

- [ ] 5.2.1 `CONTRIBUTING.md`, "Code analysis": what the gate requires (80% of the whole component plus the new-code conditions), that it is provisioned by the script and still not required by branch protection, and that a red gate on a component under 80% is expected until its coverage change lands. No page under `docs/` describes the analysis, so the site carries nothing to update — stated here so the absence is a claim rather than an omission. Verify: the paragraph names the threshold and the script, and `wc -l README.md` is unchanged.
