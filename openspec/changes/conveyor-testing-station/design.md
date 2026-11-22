## Context

See proposal.md for the motivation.

- `conveyor.py` is the one machine deciding the line. Its station states are `none, implement, fix, merge, archive, stalled, done`. Its loop states are on the pull request.
- Coverage already runs in CI per toolchain (`go test -coverpkg`, vitest lcov, `node --test` lcov, coverage.py) with a floor script, `coverage-floor.sh`. Nothing reads a diff, and the system and e2e tiers collect none.
- The fixing round is model-free around one model session. A scoped deploy key applies the patch, and the model never pushes.
- The implement and archive stations are SESSIONS started by `remote-implement.py` from a routine file (`.github/routines/`). The testing station is the third.
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

### D3. The session runs the tiers and collects coverage itself

The session runs each tier in its own environment, then `coverage-diff.py` over the output.

| Tier | The session runs | Coverage from |
|---|---|---|
| unit | each touched module's own suite | Go profile, vitest and `node --test` lcov, coverage.py |
| system | `go test -tags system` with `SYSTEM_COVER=1`, on a k3d cluster it spawns | the operator built with `-cover`, counters collected from the node |
| e2e | the Playwright suites against that cluster | V8 coverage of the console UI source |

- The instrumented operator writes its counters only on exit. A Helm post-renderer (`test/system/covrender`) points `GOCOVERDIR` at a node directory, and the harness stops the manager gracefully and copies the data off. No chart value is added.
- A run that wrote no data fails the tier. It never reads as zero changed lines.
- The session's environment must provide Docker, k3d, Helm, kubectl and a Chromium. That is an assumption to verify, and a missing tool stops the station with a named reason.
- Alternative: CI runs the tiers and the session reads artifacts. Rejected, since the session cannot then loop on its own result.

### D4. The tester is a separate agent with a narrower view and a narrower reach

- It is a project agent definition whose model is set to Sonnet 5. It must differ from the session's model, and `testing-station.py models` fails closed when they are equal or either is unset.
- It runs with its own context, so it sees the change's specs, design, the diff's public signatures and the uncovered line list, and never the session's reasoning.
- It writes test files only. After it returns, `testing-station.py check-tree` refuses a change to any path outside the test path list, and the session reverts it.
- The session may not edit a file the tester authored, found by a commit trailer. It disputes it instead, and the tester judges the dispute in the next round.
- Alternative: one model writes code and tests. Rejected by the requirement, since it grades its own work.

### D5. The loop is the session's, within its own budget

One session runs rounds. A round runs the tiers, derives the work, acts, and records itself.

| Result | Work | By |
|---|---|---|
| a tier red | the production code | the session |
| a tier under its floor | tests for uncovered changed lines | the tester agent |
| a tier collection failure | the pack or its instrumentation | the session |
| all green and over floors | none, station passes | |

- A round is recorded by a marker comment on the issue. The budget is those markers, counted apart from the fixing loop's, 5 by default.
- `conveyor:keep-going` grants another 5 and is consumed only when it takes effect.
- A round in which no model ran spends nothing. A round that changed nothing ends the station early.
- A red that does not reproduce is re-run, and a real-model failure is re-run once.
- The session pushes its own branch, as the implement session does. On passing, the station moves to `fix` and a dispatch of the carry starts the fixing loop on the pushed head.

### D5a. Whether the real model matters is a judgement with a floor

`testing.agent_facing_paths` lists the code a model's output flows through: the runtimes, the dispatch, the pod builder, the chat core, the MCP compiler, the prompts and the output-format template. A diff touching one of them requires real-model testing. The session may require it for other code and records why. It may not waive it for a listed path. Where it is required, `CLAUDE_CODE_OAUTH_TOKEN` must be set in the environment or the station stops.

### D6. The report is one issue comment with a hidden marker

A script renders it from the round's numbers and edits it in place through the issue comment API. It holds paths and line numbers only, so the publication guard and the no-quoted-code rule hold.

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
- The merge is done by `merge-sync.py`, a program. No model resolves a conflict.
- Alternative: let the fixer resolve all conflicts. Rejected, a model silently choosing a side of a hand-written conflict is the exact risk the requirement is meant to avoid.

## Risks / Trade-offs

- [The system tier's instrumented run costs more than the plain one] → the station runs the system tier only when the diff has system-domain lines, and the smoke pack is enough where it covers them.
- [A real model costs tokens and is nondeterministic] → it runs only where the judgement or the path floor requires it, asserts outcomes and never wording, and re-runs a failure once.
- [The session's environment may lack Docker or a browser] → the station stops with the missing tool named, and reads no tier as passed.
- [A credential in a session's environment] → it is read only as an environment variable by the tests, and never printed or stored.
- [A floor is unreachable for code the tier cannot reach] → the budget ends the station as `stalled` with the report naming the lines, and a person decides.
- [Playwright coverage maps imprecisely to source] → use source maps, and treat a file with no mapping as a collection failure.
- [A second model raises cost and latency] → the tester runs only on a coverage shortfall.
- [Tests written to cover lines and not to check behaviour] → the tester is given specs and scenarios, not only line numbers.
- [Re-entering testing after every base merge loops] → a merge that changes no tested line re-runs the gate once, not a full round, and the budget bounds the rest.

## Migration Plan

1. Land the station machine and its table test with the station unreachable.
2. Land the coverage scripts, the instrumented system run and the report.
3. Land the routine file, the tester agent and the step that fires the session.
4. Enable it by the implement session's hand-over.

Rollback is removing the hand-over, so `implement` goes to `fix` as before.
