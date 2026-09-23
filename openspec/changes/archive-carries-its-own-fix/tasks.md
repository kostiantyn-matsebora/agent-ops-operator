## 1. The fix-station carry accepts the archive label

- [x] 1.1 `carry-grant.py` computes an effective `grant_label`: `run_label` if present, else `archive_label` for `--station fix` only. Every later reference in `main()` (placement lookup, permission re-check, comments, log lines) reads `grant_label`, not `run_label`, so the same verification path serves either grant. Verify: the three new cases in `carry-grant.test.sh` below
- [x] 1.2 The module docstring names the fallback and why it exists

## 2. Unit tests

- [x] 2.1 `.github/tests/carry-grant.test.sh`: three new cases, covering `conveyor:archive` alone carrying `conveyor:fix` onto the archive pull request, `conveyor:run` still winning when both labels are present, and an archive-label placer who has since lost write access being refused the same way a run-label placer is
- [x] 2.2 `.github/tests/run.sh` passes in full
- [x] 2.3 `python3 .github/scripts/publication-guard.py` and `python3 .github/scripts/retired-vocabulary-guard.py` pass on this worktree

## 3. E2E tests

- [x] 3.1 Not applicable: nothing here is decided by a cluster. The change is one script's grant check, and `docs/testing.md` places that in the script suite.

## 4. Documentation

### 4.1 Reference docs

- [x] 4.1.1 `.claude/rules/worktree-delivery.md`'s `conveyor:archive` row already states this behaviour — confirmed no edit needed (`grep -n "loop drives that pull request to mergeable"`)

### 4.2 Adopter site

- [x] 4.2.1 `docs/diagrams/conveyor-lifecycle-implementation.mmd`'s `CARRY` node named only `conveyor:run` as the re-checked grant — updated to also name `conveyor:archive` on the issue. No other page under `docs/` mentions the fix-station carry (`grep -rn "conveyor:archive\|carry-grant" docs/`)
