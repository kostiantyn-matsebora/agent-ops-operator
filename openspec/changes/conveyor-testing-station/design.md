## Context

See proposal.md for the motivation.

- `conveyor.py` is the one machine deciding the line. Its station states are `none, implement, fix, merge, archive, stalled, done`. Its loop states are on the pull request.
- Coverage already runs in CI per toolchain (`go test -coverpkg`, vitest lcov, `node --test` lcov, coverage.py) with a floor script, `coverage-floor.sh`. Nothing reads a diff, and the system and e2e tiers collect none.
- The fixing round is model-free around one model session. A scoped deploy key applies the patch, and the model never pushes.
- `.github/conveyor-model/labels.yaml` uses a longer station vocabulary than `conveyor.py`. This change follows `conveyor.py`, the live one.

## Goals / Non-Goals

**Goals:**

- A testing station the line cannot skip, with per-tier diff floors.
- Tests authored without the implementer's bias.
- A bounded loop, a coverage report on the issue, and a pull request that is really mergeable.

**Non-Goals:**

- A repository-wide coverage floor.
- Replacing the review loop. The testing station runs before it.
- Mutation testing.

## Decisions

### D1. The testing station is a state of the station machine, not a second machine

`testing` joins `STATION_STATES`. New events are `testing:start`, `testing:passed` and `testing:capped`. The table test keeps walking every state against every event.

- Alternative: a loop label on the pull request, beside `running`. Rejected, since the requirement is that `station:testing` is traceable on the issue.
- `implement` goes to `testing` when the pull request opens. `testing` goes to `fix` on passing, and to `stalled` on cap.

### D2. Coverage is the diff's, per tier, and a tier outside the diff holds no floor

A script intersects each tier's coverage output with the added lines of `git diff origin/<base>...HEAD`. The percentage is covered changed executable lines over changed executable lines.

| Tier | Source of coverage | Domain |
|---|---|---|
| unit | Go cover profile, vitest and node lcov, coverage.py | all changed code |
| system | the operator built with `go build -cover`, data from `GOCOVERDIR` | changed Go in `platform/manager` and what the pack runs |
| e2e | Playwright V8 coverage, converted to lcov | changed lines under `platform/console/ui/src` |

- Floors are inputs to the script (90, 70, 50), not constants in it.
- Alternative: whole-repository coverage. Rejected, it would block every change until the legacy gap is closed.
- Alternative: a floor for a tier with no domain lines. Rejected, it would force a meaningless test.

### D3. System coverage needs an instrumented operator and a clean shutdown

An instrumented binary writes its counters only on exit. The system run builds the operator image with `-cover`, mounts a `GOCOVERDIR` volume, and stops the pod gracefully before the data is read. Missing data after a run fails the tier. It does not read as zero changed lines (spec: instrumented operator).

### D4. The tester is a separate model session with a narrower view and a narrower patch

- Configuration names the tester's model, set to Sonnet 5. It must differ from the implementer's model, and the job fails closed when they are equal.
- The tester gets the change's specs, design, the diff's public signatures and the uncovered line list. It gets no transcript and no implementation reasoning.
- Its patch is applied by the model-free job, as the fixer's is. The job refuses any path outside test paths. Test paths are named in one list the script reads.
- The fixer's patch is refused where it edits a file the tester authored. The fixer may dispute it. The tester judges the dispute in the next round.
- Alternative: one model writes code and tests. Rejected by the requirement, since it grades its own work.

### D5. The loop is one round function, with its own counter

A round runs the tiers, derives the work, runs the tester for coverage work and the fixer for failing tests, applies patches, then ends.

| Result | Work | Session |
|---|---|---|
| a tier red | the production code | fixer |
| a tier under its floor | tests for uncovered changed lines | tester |
| a tier collection failure | the pack or its instrumentation | fixer |
| all green and over floors | none | none, station passes |

- Counted apart from `max_rounds`, default 5, by the same marker mechanism with a distinct marker. `conveyor:keep-going` grants 5 more to whichever budget is capped.
- A round with no model run spends nothing. A round that changes nothing ends the station, as in the fixing loop.
- A red that does not reproduce is re-run, as the fixing step already does.

### D6. The report is one issue comment with a hidden marker

A model-free script renders it from the round's numbers and edits it in place through the issue comment API. It holds paths and line numbers only, so the publication guard and the no-quoted-code rule hold.

### D7. Mergeable is a fact read from GitHub, not inferred

`conveyor.py` gains a decision over `mergeStateStatus`. The loop ends `mergeable` only on clean.

| Merge state | Action |
|---|---|
| clean | `mergeable` if checks, threads and testing agree |
| behind | merge the base in, push, re-enter the testing gate on the new head |
| conflicting, trivial | resolve, push, re-enter the testing gate |
| conflicting, non-trivial | loop `waiting`, comment naming files, nothing pushed |
| failed required check, including build | work item for the fixer |

- Trivial means the base merges with no conflict marker, or every conflicted file is one a repository script regenerates (CRDs, generated docs, lockfiles).
- The merge is done by a model-free job with the deploy key. No model resolves a conflict.
- Alternative: let the fixer resolve all conflicts. Rejected, a model silently choosing a side of a hand-written conflict is the exact risk the requirement is meant to avoid.

## Risks / Trade-offs

- [The system tier's instrumented run costs more than the plain one] → the station runs the system tier only when the diff has system-domain lines, and the smoke pack is enough where it covers them.
- [A floor is unreachable for code the tier cannot reach] → the budget ends the station as `stalled` with the report naming the lines, and a person decides.
- [Playwright coverage maps imprecisely to source] → use source maps, and treat a file with no mapping as a collection failure.
- [A second model raises cost and latency] → the tester runs only on a coverage shortfall.
- [Tests written to cover lines and not to check behaviour] → the tester is given specs and scenarios, not only line numbers.
- [Re-entering testing after every base merge loops] → a merge that changes no tested line re-runs the gate once, not a full round, and the budget bounds the rest.

## Migration Plan

1. Land the station machine and its table test with the station unreachable.
2. Land the coverage scripts and CI emission.
3. Land the station workflow and the report.
4. Enable it by the implement session's hand-over.

Rollback is removing the hand-over, so `implement` goes to `fix` as before.
