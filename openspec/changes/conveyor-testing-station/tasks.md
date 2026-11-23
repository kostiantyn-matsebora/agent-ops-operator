## 1. Station machine

- [x] 1.1 Add `testing` to `STATION_STATES` and the events `testing:start`, `testing:restart`, `testing:resume`, `testing:passed`, `testing:capped` in `.github/scripts/conveyor.py`, and verify the totality test still defines every pair
- [x] 1.2 Route `implement` → `testing` on the pull request opening, `testing` → `fix` on passed, `testing` → `stalled` on capped, and verify `test_every_state_is_reachable_from_none` passes
- [x] 1.3 Add the testing budget as a counter apart from `max_rounds`, with its own markers, and verify the cap and `keep-going` extension in `conveyor.test.py`
- [x] 1.4 `.github/conveyor-model/` is the separate engine proposal (#293), read by nothing here, so it is left as it is. The lifecycle diagrams are task 7.1.2

## 2. Coverage measurement

- [x] 2.1 Write the diff intersection script over a Go profile, lcov and coverage.py XML, and verify it on fixtures with known covered and uncovered changed lines
- [x] 2.2 Add per-tier floors from the vocabulary (90, 70, 50) with the `not applicable` rule, and verify a tier with no domain lines holds no floor
- [x] 2.3 Build the operator with `-cover` for the system run, point `GOCOVERDIR` at a node directory through a Helm post-renderer, stop the manager gracefully and collect, and verify the post-renderer's unit tests pass
- [x] 2.4 Fail the tier when a run produced no coverage data, and verify with a fixture of an empty directory
- [x] 2.5 Collect browser coverage for `platform/console/ui/src` into lcov behind `E2E_COVERAGE=1` from an istanbul-instrumented build of the mocked suite, `npm run test:e2e:coverage`, and verify the specs yield mapped lines (154 files)
- [x] 2.6 Run the instrumented system pack once and verify it writes `system-coverage.out` (a container runtime and k3d are needed. It ran against Rancher Desktop on moby with Helm 4 and wrote an 8595-line profile)

## 3. The testing session

- [x] 3.1 Write `.github/routines/test-change.md`: the payload, the rounds, the tiers and their commands, the real-model judgement, the tester, the report and the endings. Verify a test reads every script the file names and that each exists
- [x] 3.2 Add the tester agent definition with its model set to Sonnet 5 and a tool list limited to reading, test-writing and running tests, and verify the opsx-commands test or a new test reads its `model`
- [x] 3.3 Fire the session from the implement hand-over: `begin` in the `open` job, then `remote-implement.py` with a testing station, and verify with the stubbed endpoint that one fire starts and a second is skipped
- [x] 3.4 Resume a stalled station: the sweep dispatches a job that consumes `conveyor:keep-going` and fires with a restart, and verify the label is consumed only when it takes effect
- [x] 3.5 Check the tester's work against the test path list and the session's against the tester-authored files, and verify with a patch touching a production file
- [x] 3.6 Fail closed when the tester's model equals the session's or either is unset, and verify
- [x] 3.7 Render the coverage comment and edit it in place by marker, and verify two rounds leave one comment holding paths and line numbers and no source text
- [x] 3.8 Add `testing.agent_facing_paths` and a decision naming the diff's agent-facing files, and verify it on a prompt change and a label-only chart change
- [x] 3.9 Stop the station with the reason named when real-model testing is required and the credential variable is unset, and verify no tier reads as passed and the value is never printed
- [x] 3.10 On passing, dispatch the carry so the fixing loop starts on the pushed head, and verify the dispatch input

## 4. Mergeable pull request

- [x] 4.1 Read `mergeStateStatus` in the loop's facts and make `mergeable` require clean, and verify with a table of every merge state
- [x] 4.2 Treat a failed build check as work, and verify `failed-checks.py` returns it for a red build job
- [x] 4.3 Merge a behind head's base in a program and send the station back to testing, and verify on a throwaway repository
- [x] 4.4 Resolve a trivial conflict (clean merge, or only generated files) and push, and verify on a throwaway repository with a conflicted CRD
- [x] 4.5 For a non-trivial conflict set the loop `waiting`, comment the conflicted files, push nothing, and verify a person's comment resumes the loop
- [x] 4.6 Verify the fixing loop cannot end `mergeable` while the testing station has not passed on the current head

## 5. Unit tests

- [x] 5.1 Extend `.github/tests/conveyor.test.py` for the new states, events, budget, grant extension and merge-state decisions
- [x] 5.2 Add tests for the diff intersection, the report renderer, the path refusals, the conflict classifier and the adapter, and run `.github/tests/run.sh`
- [x] 5.3 Run `python3 .github/scripts/publication-guard.py` and `retired-vocabulary-guard.py` and record the verdicts

## 6. System tests

- [x] 6.1 Run the instrumented smoke pack in an environment with Docker and verify the operator's coverage comes back
- [ ] 6.2 Run the station once on a throwaway pull request carrying a known coverage shortfall, and verify it writes the report, loops, and ends `station:fix`
- [x] 6.3 Run the real-runtime system lane with the credential set, and verify both real-runtime tests pass and a stub run is not accepted for an agent-facing change
- [ ] 6.4 Run the console change through `npm run test:e2e` and `e2e-live` and verify browser coverage collects
- [ ] 6.5 Run each step above that a remote session cannot run in an environment that can, and tick it only when it ran. A step that was not run stays unticked and the pull request says why

## 7. Documentation

### 7.1 Reference docs

- [x] 7.1.1 Update `docs/testing.md` with the diff floors, the session as runner, the instrumented system run, browser coverage and the real-model rule
- [x] 7.1.2 Update `docs/diagrams/conveyor-lifecycle.mmd` and `conveyor-lifecycle-implementation.mmd` and regenerate any SVG from its source
- [x] 7.1.3 Update `CONTRIBUTING.md` with the testing station and the mergeability rule
- [x] 7.1.4 Update `.claude/rules/worktree-delivery.md`, `remote-session.md` and `gotchas.md` for the new station and its lessons
- [x] 7.1.5 Add a `docs/CHANGELOG.md` entry, and run `python3 .github/scripts/docs-generate.py --check`
- [x] 7.1.6 Run `python3 .claude/scripts/rules_compliance.py` over the touched markdown

### 7.2 Adopter site

- [x] 7.2.1 Check the landing page, Getting started, Installation and the guides for a page describing the delivery workflow, update it, or tick a line stating none does
