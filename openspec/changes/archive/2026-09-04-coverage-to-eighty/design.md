## Context

See `proposal.md` — Why. Sixteen SonarCloud projects, one Go/Node module
each (`build-test.md`'s module list), all below the `agentops` gate's 80%
overall-coverage condition. `sonar-ratings-baseline` (PR #145, delivered
2026-09-01) already brought every component to A/A/A on ratings; this change
is the coverage half that was explicitly deferred out of it.

## Goals / Non-Goals

**Goals:**
- Every component's project-level `coverage` measure ≥ 80%.
- Every added test exercises a real, previously-untested path.

**Non-Goals:**
- Raising coverage past 80% once a component clears it — unlike the ratings
  sweep, there is no reason to keep pushing a component that already meets
  the gate's own bar.
- Any refactor not required to make a line testable. A hard-to-test line is
  tested as it stands, or excluded with a stated reason — never restructured
  for its own sake.
- The `agentops-unenforced` (`scripts`) project's GATE status — it stays
  exempt from the gate per `sonar-ratings-baseline`. Its coverage is still
  raised, because it is shipped tooling read by contributors, just not
  gate-blocking.

## Decisions

**Order: worst starting-coverage first**, per the table in `proposal.md`.
Two reasons: the components furthest from 80% (`signal-cron` 26.6%,
`housekeeping` 39.4%) are also small in absolute lines, so they clear fastest
and the gate's red count drops quickly; and a large component close to the
line (`signal-k8s-events` 78.7%) is cheap regardless of when it is worked, so
sequencing it last costs nothing.

**One task section per component**, each independently committable and
independently checkable against its own SonarCloud project — mirroring how
`sonar-ratings-baseline` structured its per-component sections. A single
`go test`/`node --test` run per component is the section's own verification;
the PR-level `new_coverage` gate re-confirms it in CI exactly as it did for
ratings.

**Coverage is read from `get_component_measures` (main branch), never
`new_coverage`.** `new_coverage` is scoped to lines the PR itself touches; the
gate condition this change is closing is the OVERALL `coverage` measure,
which only updates on a main-branch analysis. A component's own dashboard
number is therefore the one this change's tasks record, and the PR's
`new_coverage` passing is a necessary but not sufficient signal — it can pass
on a PR that adds no coverage to old, still-uncovered code.

**A genuinely dead or unreachable line is deleted or excluded, not tested
around.** Writing a test whose only purpose is to execute an unreachable
branch produces a test that asserts nothing about behavior — pure gate
theater. `docs/testing.md`'s tier boundaries (what unit/envtest/e2e can each
decide) govern whether a line even belongs in a component's own suite before
either happens.

## Risks / Trade-offs

**[Risk] A mechanical push for coverage produces trivial or brittle tests**
(asserting implementation details, mocking away the behavior under test) →
Mitigation: every new test states in a comment what real gap it closes,
following the pattern `sonar-ratings-baseline` used for its own coverage
fixes (`errorpaths_test.go`, `gitexec_test.go`) — a failing manager, a real
subprocess, a real PATH lookup, never a mock standing in for the thing being
tested.

**[Risk] `manager`'s envtest suite is slow (~45s+ per run) and its coverage
gap is the largest in absolute lines (~617)** → Mitigation: worked in its own
task section with its own commits, so a slow suite does not block progress on
the other fifteen components; not treated as a reason to skip it.

**[Risk] Coverage tooling differs per toolchain** (`-coverpkg=./...` for Go,
`vitest`'s lcov for the console's UI half, `node --test`'s
`--experimental-test-coverage` for the two Node runtimes, which reports only
LOADED files rather than the whole package) → Mitigation: each component's
task section names its own toolchain's coverage command from
`build-test.md`, "Coverage, with the flags CI uses", rather than assuming Go's
command applies everywhere.

## Migration Plan

None — test-only, no runtime behavior, no deploy step. Each component's
section is verified against its own SonarCloud project once its PR merges to
`master` (the overall `coverage` measure only updates on a main-branch
analysis, per the Decisions section above).
