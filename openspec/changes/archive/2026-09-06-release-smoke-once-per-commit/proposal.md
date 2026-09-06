## Why

**The release workflow smokes the same commit once per TAG, and a release is
many tags on one commit.** On 2026-09-06 chart 13.4.0 shipped fourteen
component images from one commit: fourteen tags, fourteen identical
ten-minute cluster smokes in parallel on shared runners, then a fifteenth on
the chart tag, whose commit differed from theirs in pins and docs alone. Both
failures of that release came from the load and not from the code: a k3d
image import deadlocked under fourteen concurrent runs, and the console
lifecycle lane timed out after two minutes on a step that takes eight seconds
when the runners are quiet. Each was answered by a re-run that changed
nothing.

The workflow's own comment states the intent correctly — the smoke runs on the
exact commit being published, because a tag can land on a commit CI passed
days ago — and `ci is green for this commit` implements that intent for CI by
LOOKING UP the commit's existing run. The smoke has no such lookup, so the
intent "this commit was smoked" is implemented as "this tag ran a smoke".

## What Changes

- **The smoke is keyed to the commit.** Before the release workflow provisions
  a cluster it looks up the tagged commit's check runs for a successful smoke
  — from any earlier release run, or an on-demand smoke on the same commit —
  and, finding one, records the commit as smoked and skips the run.
- **A smoke already RUNNING on the same commit is waited for, not raced.**
  Tags are pushed three at a time by rule; the lookup finds another run's
  smoke in progress on the commit and waits for its verdict, bounded, before
  deciding. Two runs that both find nothing may still both smoke; that is a
  cost of two instead of fourteen, accepted.
- **The publish jobs gate on "smoked", not on "this run's smoke job passed".**
  An image or chart publishes when the commit's smoke passed, whether this run
  produced it or found it.
- **A failed or absent smoke still runs one**, so a re-run after a flake, a
  tag on a commit no run smoked, and a chart tag on a fresh commit all behave
  as today.
- **The lookup is a script with a test**, not workflow YAML: what decides
  whether a cluster is provisioned before publishing is a decision, and the
  script suite is where this repository's decisions are exercised.
- Not breaking. No artifact, tag form or published name changes.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `end-to-end-testing`: the requirement "Tiers are defined by what gates a
  pull request" changes its release clause from "the cluster smoke runs on
  that commit" to "the commit has a passed smoke — found or produced", with
  the wait for an in-flight one stated.

## Impact

**Code**

- `.github/workflows/release.yml` — a `smoke_is_green` job between
  `ci_is_green` and `smoke`, the `smoke` job conditioned on its answer, and
  `image` / `chart` gated on the commit being smoked by either path.
- `.github/scripts/smoke-evidence.py` — the lookup: the commit's check runs,
  the smoke check's name, success / in-progress / absent, a bounded wait.
- `.github/tests/smoke-evidence.test.sh` — against the stubbed `gh`: found,
  absent, in-progress-then-success, in-progress-then-failure, the wait bound.

**Reference docs**

- `docs/testing.md` — the release row of the workflow table, and the tier
  model's "gates a release" sentence, say once per commit.
- `.claude/rules/gotchas.md` — the measurement: fourteen tags, fifteen
  smokes, two flakes, what a lookup costs instead.
- `docs/CHANGELOG.md` — an Unreleased entry; contributor-facing only.

**The adopter site**

- Nothing on the site describes the release workflow's internals. `CONTRIBUTING.md`
  names the smoke as a release gate in one sentence; that sentence stays true.
  Checked.

**Not affected**

- `e2e.yml`, `e2e-smoke.yml`, `e2e-full.yml`, the pack itself, `ci.yml`.
