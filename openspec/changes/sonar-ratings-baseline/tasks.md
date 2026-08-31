## 1. The backlog, enumerated (design D1, D3)

- [ ] 1.1 New `.github/scripts/sonar-findings-baseline.py`: for every component
  `components.sh images` lists (plus `scripts`, same project-key pattern
  `sonar-issues.py` uses), `GET issues/search` with `resolved=false`,
  `impactSeverities=BLOCKER,HIGH` and no `pullRequest` param — the branch-wide
  backlog, not one pull request's. Writes counts per component per
  `softwareQuality` to a JSON file; prints the same as a table. Verify: run
  against one component known to carry issues; a 0-result response with
  `impactSeverities` set is treated as a possible taxonomy mismatch and the
  script also tries the legacy `severities=BLOCKER,CRITICAL` filter,
  reporting BOTH counts when they disagree rather than silently trusting the
  new one.
- [ ] 1.2 **OPEN — needs the user's token, and is theirs to run.** Run
  1.1's script against the organisation. Record the BEFORE counts here, as a
  table of numbers only (no identifier, no url, no finding text — the org is
  a secret, and a finding's message is the same class of thing task 1.1 in
  `coverage-across-packages` already established the pattern for). Verify:
  every component `components.sh images` lists appears as a row, zero counts
  included.

## 2. Every Blocker and High finding, fixed (design D2, D3)

- [ ] 2.1 For every component task 1.2's table lists with a nonzero count,
  fix every enumerated Blocker/High finding in that component's own code —
  the fix each rule calls for, never a suppression, an exclusion, or a
  threshold change on the gate. Re-run 1.1's script after each component's
  fixes land. Verify: the component's count in the script's output reaches
  zero for both reliability- and security-impacting findings, and
  maintainability's Blocker/High count reaches zero or is justified in this
  task with the specific rule and why it does not apply, per finding.

  **THIS TASK IS A PLACEHOLDER FOR A PER-COMPONENT BREAKDOWN**, expanded once
  task 1.2's table names which components need it — `/opsx:update` or the
  apply session that reads the enumerated list splits this into one task per
  affected component, sized to what that component actually carries. Design
  D3 states why a concrete list is not invented here.
- [ ] 2.2 Record the AFTER counts beside task 1.2's table, from a re-run of
  1.1's script once every component in 2.1 reaches zero. Verify: every row
  from 1.2 reads zero for Blocker and High severity.

## 3. The gate, extended (design D1, D2)

- [ ] 3.1 `.github/scripts/sonar-provision.sh`'s gate stage: extend the
  `wanted` list with `reliability_rating GT 2`, `security_rating GT 2` and
  `sqale_rating GT 2` (A=1 … E=5, so `GT 2` fails worse than B), same
  update-not-duplicate handling the coverage condition already has. Verify:
  `sh -n` passes; the three metrics are literal strings beside `coverage`,
  not derived, matching how `coverage LT 80` is written today.
- [ ] 3.2 **OPEN — needs the user's token, and is theirs to run, and comes
  AFTER task 2.2 reads zero.** Run the extended provisioning script against
  the organisation. Read back `api/qualitygates/show?name=agentops`: seven
  conditions become ten. Verify: ten conditions, every component project
  still assigned, and a second run creates nothing new.

## 4. Unit tests

- [ ] 4.1 New `.github/tests/sonar-findings-baseline.test.sh` with `curl`
  stubbed from fixtures: an org with issues on both taxonomies reports both
  counts and flags the mismatch; an org with only Clean Code impacts reports
  cleanly; the component list is read from a captured `components.sh` output,
  exactly as `sonar-issues.test.sh` fixtures it. Wired into `run.sh`. Verify:
  `.github/tests/run.sh` passes.
- [ ] 4.2 Extend `.github/tests/sonar-provision.test.sh`: an org with the
  `agentops` gate and seven conditions (coverage plus the six new-code ones)
  gains exactly the three rating conditions on a `--gate` run; a gate that
  already carries all ten makes no `create_condition` or `update_condition`
  call. Verify: `.github/tests/run.sh` passes, mutation-checked the same way
  the coverage condition's test was — replacing `update_condition` with
  `create_condition` fails only the no-duplicate assertion.
- [ ] 4.3 Every module whose code changed under task 2 passes its own suite:
  `go test ./...` (or the module's own toolchain) in each touched component,
  from the `golang:1.25` build container per `build-test.md`. Verify: every
  touched module's suite exits 0.
- [ ] 4.4 Run the suite and both guards from the worktree:
  `.github/tests/run.sh`, `python3 .github/scripts/publication-guard.py`,
  `python3 .github/scripts/retired-vocabulary-guard.py`. Verify: all three
  exit 0.

## 5. E2E tests

- [ ] 5.1 Not applicable: nothing here is decided by a cluster — a
  branch-wide analysis read, code fixes judged by the analysis service, and a
  gate provisioning script. The live proof is the first CI run of this
  branch (every touched component's analysis) and task 3.2's read-back.
  Verify: the pull request's SonarCloud checks show every touched component
  clean of Blocker/High findings, and the `agentops` gate's ten conditions.

## 6. Documentation

### 6.1 Reference docs

- [ ] 6.1.1 The delta spec is archived into
  `openspec/specs/code-quality-analysis/spec.md` by `/opsx:archive` on the
  branch; `openspec validate --all` passes. Verify: the command exits 0.
- [ ] 6.1.2 If task 2's fix sweep surfaces a technique worth keeping —
  a class of finding this codebase produces repeatedly, or a rule worth
  naming as a house convention — record it in `.claude/rules/gotchas.md` or
  the relevant topic file. Verify: named here if anything qualified, or this
  task states none did.

### 6.2 Adopter site

- [ ] 6.2.1 `CONTRIBUTING.md`, "Code analysis": the gate now additionally
  requires at least a B overall reliability, security and maintainability
  rating per component, provisioned the same way the coverage condition is.
  No page under `docs/` describes the analysis (confirmed by
  `coverage-across-packages`' own check), so the site carries nothing to
  update — stated here so the absence is a claim rather than an omission.
  Verify: the paragraph names the three ratings and the threshold, and
  `wc -l README.md` is unchanged.
