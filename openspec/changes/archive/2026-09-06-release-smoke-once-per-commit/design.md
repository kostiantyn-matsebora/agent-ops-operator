## Context

See `proposal.md` — Why. What shapes the approach:

- **`ci_is_green` is the pattern, already in the file.** A tag push runs
  `release.yml` and nothing else, so evidence about the commit has to be
  looked up; that job polls the commit's `ci.yml` runs, waits while one is
  still running, and fails when none passed. The smoke needs the same
  question answered about a different piece of evidence.
- **The evidence is a check run, not a workflow run.** The smoke is a
  reusable workflow called by a job named `smoke`, so every smoke — from
  `release.yml` or from `e2e-smoke.yml` — appears on the commit as a check run
  named `smoke / e2e / smoke`. The commit's check runs are the one place all
  of them meet, whatever workflow produced them.
- **The pack builds every image from the commit under an `:e2e` tag and
  overrides the chart's pins**, so a smoke proves the COMMIT and nothing about
  the tag that triggered it. Fourteen tags on one commit smoke the same
  thing fourteen times. This is what makes reuse sound.
- **Tags are pushed three at a time, by rule.** Runs on one commit arrive
  together, so the lookup will routinely find a smoke in progress rather than
  passed.
- **A skipped job's dependants are skipped too**, unless their condition says
  otherwise. Gating `image` and `chart` on a smoke that may legitimately be
  skipped needs an explicit condition, not `needs:` alone.

## Goals / Non-Goals

**Goals:**

- One smoke per commit in the ordinary case, whatever the number of tags.
- Every published artifact still sits on a commit with a passed smoke.
- The decision lives in a script with a test.

**Non-Goals:**

- Changing what the smoke runs, or the on-demand and nightly workflows.
- Making the smoke a required check on pull requests. Its gating rule is
  unchanged.
- Perfect deduplication under a simultaneous start. Two runs that both find
  nothing before either has started a cluster will both smoke. The cost is
  two, not fourteen, and closing that window needs a lock this platform does
  not offer without cancelling runs.

## Decisions

**A `smoke_is_green` job answers "smoked" / "run it", and `smoke` runs only
on the second.** Placed after `ci_is_green` — a commit CI never passed is not
worth smoking, exactly as today — with one output, `smoked`, and one script
behind it.

- *Alternative: a `concurrency` group keyed by the commit.* Rejected. The
  platform keeps one running and one PENDING run per group and CANCELS the
  rest, so fourteen tags would become one smoke, one wait and twelve
  cancelled releases. A cancelled release is a tag that published nothing,
  silently — the exact failure `build-test.md` warns about.
- *Alternative: smoke only on the chart tag.* Rejected. An image tag would
  then publish with no cluster run behind it, which is the case the
  workflow's comment exists to prevent.

**The script reads the commit's check runs and classifies them.**
`.github/scripts/smoke-evidence.py --repo <owner/repo> --sha <sha>
[--wait-minutes N]`, over `gh api repos/<r>/commits/<sha>/check-runs`
(paginated), selecting check runs whose name ends with `e2e / smoke`:

| It finds | It answers |
|---|---|
| one with `conclusion: success` | `smoked=true`, exit 0 |
| none, or only failed ones | `smoked=false`, exit 0 — run one |
| one `queued` or `in_progress` and no success | wait: re-read every thirty seconds up to the bound, then classify again |
| the bound passed with one still running | `smoked=false` — run our own rather than wait forever |
| `gh` unreachable or the API erring | `smoked=false` — the safe answer is to run the smoke, never to publish on missing evidence |

- *Why "ends with `e2e / smoke`".* The check run's name is
  `<caller job> / <called workflow> / <called job>`: `smoke / e2e / smoke`
  from both `release.yml` and `e2e-smoke.yml` today. Matching the tail keeps
  a renamed caller job from silently switching reuse off.
- *Why the wait bound is twenty minutes.* A smoke takes about ten on a quiet
  runner; the bound covers one slow run and does not double a release that
  would have taken ten minutes anyway.
- *Why a failed smoke does not block.* A failure on the commit from another
  run is most often the flake this change exists to reduce; this run's own
  smoke is the re-test. The image is still not published unless a smoke
  passed — this run's.

**The publish jobs gate on the commit being smoked.** `image` and `chart`
carry `if: !cancelled() && needs.ci_is_green.result == 'success' &&
(needs.smoke.result == 'success' || (needs.smoke.result == 'skipped' &&
needs.smoke_is_green.outputs.smoked == 'true'))` beside their existing
artifact-kind condition. `!cancelled()` is what lets a job run behind a
skipped dependency; the rest states the two acceptable histories and no
other — a smoke that FAILED in this run never publishes, whatever the lookup
said before it ran.

**The script is exercised in the suite against the stubbed `gh`.** The
existing stub serves canned API answers; the test feeds it check-run lists
for each row of the table and a sequence for the wait, with the poll interval
shortened by a flag the workflow never passes.

## Risks / Trade-offs

- **Two runs start before either smoke exists and both smoke.** → Accepted;
  see Non-Goals. The wait covers the common case, where the three tags of a
  batch land seconds apart and the first smoke is queued before the third
  lookup runs.
- **An on-demand smoke on a branch becomes evidence for a later release of
  the same commit.** → Correct by construction: same commit, same images,
  same chart. If the pack changes what a smoke means, the check-run name is
  where a future change would draw the line.
- **A commit smoked before the workflow gained the lookup.** → Its check run
  has the same name and counts. Nothing migrates.
- **The lookup itself fails.** → Falls to "run it", the pre-change
  behaviour; a broken lookup costs a smoke, never a silent publish.

## Migration Plan

None. The next tag push takes the new path; no state, no secret, no setting.
