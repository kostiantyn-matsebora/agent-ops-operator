## 1. The lookup

- [ ] 1.1 Write `.github/scripts/smoke-evidence.py`: `--repo`, `--sha`,
      `--wait-minutes` (default 20), `--poll-seconds` (default 30, for
      tests); reads the commit's check runs through `gh api` with
      pagination, selects names ending in `e2e / smoke`, and prints
      `smoked=true|false` to stdout plus a one-line reason to stderr,
      classifying per the design's table and waiting on an in-flight one.
      Verify with `python3 .github/scripts/smoke-evidence.py --help` and the
      test in 3.1, run from THIS WORKTREE.

## 2. The workflow

- [ ] 2.1 In `.github/workflows/release.yml`, add `smoke_is_green` after
      `ci_is_green` (needs both `determine` and `ci_is_green`, `actions:
      read` and `contents: read`, restores nothing from the tag — the script
      is read from the checkout, which is the tagged commit itself) with
      output `smoked`; condition `smoke` on `smoked != 'true'`; gate `image`
      and `chart` with the design's expression beside their kind condition;
      rewrite the smoke comment to say the smoke is keyed to the commit and
      why. Verify with `actionlint` if available and by reading the
      conditions back against the design.
- [ ] 2.2 Verify the workflow against the platform, since a `needs:` on a
      skipped job is the one thing no linter decides: push a throwaway tag
      on a commit that already has a passed smoke and confirm the run
      publishes with `smoke` skipped and `smoke_is_green` reporting found;
      then delete the tag and the artifact it produced, or use a
      `-rc` pre-release version the chart never pins. Record what was seen.

## 3. Unit tests

- [ ] 3.1 Add `.github/tests/smoke-evidence.test.sh` against the stubbed
      `gh`: a passed smoke is found; none means run; a failed one means run;
      in-progress then success waits and reports smoked; in-progress past
      the bound means run; a failing `gh` means run. Verify with
      `.github/tests/run.sh` from the worktree, every test green.

## 4. E2E tests

- [ ] 4.1 Not applicable — nothing here is decided by a cluster. The smoke's
      content is unchanged; what changes is whether the release provisions
      one, which the platform decides (task 2.2) and the script suite
      exercises.

## 5. Documentation

### Reference docs

- [ ] 5.1 `docs/testing.md` — the release row of the workflow table and the
      tier model's release sentence say the smoke runs once per COMMIT, is
      reused across the tags of a release and waited for when in flight.
      `docs/CHANGELOG.md` — an Unreleased entry, contributor-facing.
- [ ] 5.2 `.claude/rules/gotchas.md` — the measurement: fourteen tags on one
      commit ran fifteen smokes on 2026-09-06, both failures were load, the
      lookup is why; and the concurrency-group trap (the platform cancels the
      third run in a group), so nobody reaches for it next time. Verify by
      reading both back.

### The adopter site

- [ ] 5.3 Confirm no site page describes the release workflow's internals
      (`grep -rn -i 'smoke' docs/*.md docs/guides docs/integrations` returns
      only `docs/testing.md`), and that `CONTRIBUTING.md`'s one sentence on
      the smoke gate stays true. Record the result.
