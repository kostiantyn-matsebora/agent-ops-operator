## 1. The lookup

- [x] 1.1 Write `.github/scripts/smoke-evidence.py`: `--repo`, `--sha`,
      `--wait-minutes` (default 20), `--poll-seconds` (default 30, for
      tests); reads the commit's check runs through `gh api` with
      pagination, selects names ending in `e2e / smoke`, and prints
      `smoked=true|false` to stdout plus a one-line reason to stderr,
      classifying per the design's table and waiting on an in-flight one.
      Verify with `python3 .github/scripts/smoke-evidence.py --help` and the
      test in 3.1, run from THIS WORKTREE.

## 2. The workflow

- [x] 2.1 In `.github/workflows/release.yml`, add `smoke_is_green` after
      `ci_is_green` (needs both `determine` and `ci_is_green`, `actions:
      read` and `contents: read`, restores nothing from the tag — the script
      is read from the checkout, which is the tagged commit itself) with
      output `smoked`; condition `smoke` on `smoked != 'true'`; gate `image`
      and `chart` with the design's expression beside their kind condition;
      rewrite the smoke comment to say the smoke is keyed to the commit and
      why. Verify with `actionlint` if available and by reading the
      conditions back against the design.
- [x] 2.2 Verify the workflow against the platform, since a `needs:` on a
      skipped job is the one thing no linter decides: push a throwaway tag
      on a commit that already has a passed smoke and confirm the run
      publishes with `smoke` skipped and `smoke_is_green` reporting found;
      then delete the tag and the artifact it produced, or use a
      `-rc` pre-release version the chart never pins. Record what was seen.

      **Recorded.** Verified live on commit `43061e7` (PR #185):
      `signal-cron-v0.0.2-rc1` — `smoke_is_green` found nothing, ran the real
      smoke (`smoke / e2e / smoke` passed), image published (run 34035160353).
      `signal-cron-v0.0.2-rc2`, same commit — `smoke_is_green` logged "a smoke
      already passed for 43061e7…", `smoke` job `completed/skipped`, `image
      (signal-cron) / publish` `completed/success` (run 34035689223). Both
      throwaway tags deleted after.

      **A real bug surfaced by this step, fixed in the same commit:**
      `gh api <path> -f k=v` with no `--method GET` sends the `-f` params as
      a request body on the check-runs route, which answers a bodied GET
      with 404 rather than the list — silently read by the script as "lookup
      failed, run one" (safe, but defeats the reuse). First verification
      attempt (tags `signal-cron-v0.0.1-rc1`/`-rc2` on commit `bd35215`)
      caught it: `smoke_is_green` logged the 404 and both tags ran their own
      smoke instead of reusing. Fixed with `--method GET`, a regression
      assertion added to the unit test (asserts the exact `gh` invocation),
      then re-verified clean on the fixed commit above. The four throwaway
      `agentops-signal-cron` package versions this produced
      (`0.0.1-rc2`, `0.0.2-rc1`, `0.0.2-rc2` published; `0.0.1-rc1` did not,
      `determine` failed it on the tag name before anything ran) are left in
      GHCR — this session's token lacks `packages` scope to delete them, and
      none is referenced by the chart or any real install.

## 3. Unit tests

- [x] 3.1 Add `.github/tests/smoke-evidence.test.sh` against the stubbed
      `gh`: a passed smoke is found; none means run; a failed one means run;
      in-progress then success waits and reports smoked; in-progress past
      the bound means run; a failing `gh` means run. Verify with
      `.github/tests/run.sh` from the worktree, every test green.

## 4. E2E tests

- [x] 4.1 Not applicable — nothing here is decided by a cluster. The smoke's
      content is unchanged; what changes is whether the release provisions
      one, which the platform decides (task 2.2) and the script suite
      exercises.

## 5. Documentation

### Reference docs

- [x] 5.1 `docs/testing.md` — the release row of the workflow table and the
      tier model's release sentence say the smoke runs once per COMMIT, is
      reused across the tags of a release and waited for when in flight.
      `docs/CHANGELOG.md` — an Unreleased entry, contributor-facing.
- [x] 5.2 `.claude/rules/gotchas.md` — the measurement: fourteen tags on one
      commit ran fifteen smokes on 2026-09-06, both failures were load, the
      lookup is why; and the concurrency-group trap (the platform cancels the
      third run in a group), so nobody reaches for it next time. Verify by
      reading both back.

### The adopter site

- [x] 5.3 Confirm no site page describes the release workflow's internals
      (`grep -rn -i 'smoke' docs/*.md docs/guides docs/integrations` returns
      only `docs/testing.md`), and that `CONTRIBUTING.md`'s one sentence on
      the smoke gate stays true. Record the result.

      **Recorded.** `grep -rn -i 'smoke' docs/*.md docs/guides
      docs/integrations` returns matches only in `docs/testing.md` (reference
      page) and `docs/CHANGELOG.md` (reference page, this change's own
      Unreleased entry) — no adopter site page. `CONTRIBUTING.md:206`, "The
      cluster smoke gates a release, on the tagged commit before anything is
      published," is unchanged and stays true: the smoke (found or produced)
      still gates the release on the tagged commit.
