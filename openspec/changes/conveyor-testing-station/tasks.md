## 1. Station machine

- [ ] 1.1 Add `testing` to `STATION_STATES` and the events `testing:start`, `testing:passed`, `testing:capped` in `.github/scripts/conveyor.py`, and verify the totality test still defines every pair
- [ ] 1.2 Route `implement` → `testing` on the pull request opening, `testing` → `fix` on passed, `testing` → `stalled` on capped, and verify `test_every_state_is_reachable_from_none` passes
- [ ] 1.3 Add the testing budget as a counter apart from `max_rounds`, with a distinct marker, and verify the cap and `keep-going` extension in `conveyor.test.py`
- [ ] 1.4 Update `.github/conveyor-model/labels.yaml`, `workflows.desired.yaml` and the Mermaid diagrams, and verify `render_diagram.py` regenerates them without a diff

## 2. Coverage measurement

- [ ] 2.1 Write the diff intersection script over a Go profile, lcov and coverage.py XML, and verify it on fixtures with known covered and uncovered changed lines
- [ ] 2.2 Add per-tier floors as script inputs (90, 70, 50) with the `not applicable` rule, and verify a tier with no domain lines holds no floor
- [ ] 2.3 Build the operator with `-cover` for the system run, mount `GOCOVERDIR`, stop the pod gracefully and collect, and verify a system run writes data
- [ ] 2.4 Fail the tier when a run produced no coverage data, and verify with a fixture of an empty directory
- [ ] 2.5 Collect Playwright V8 coverage for `platform/console/ui/src` through source maps into lcov, and verify one e2e spec yields mapped lines
- [ ] 2.6 Emit each tier's coverage as a CI artifact in `ci.yml`, and verify the artifacts appear on a pull request run

## 3. The testing station

- [ ] 3.1 Add the station workflow that runs the tiers and derives the work list per round, and verify with a stubbed `gh` that each result maps to its work item
- [ ] 3.2 Add the tester session with its model configured as Sonnet 5, which must differ from the implementer's, and verify the job fails closed when they are equal
- [ ] 3.3 Give the tester specs, design, public signatures and uncovered lines and no transcript, and verify the prompt holds none of the implementer's output
- [ ] 3.4 Refuse a tester patch touching a path outside the test path list, and verify with a patch touching a production file
- [ ] 3.5 Refuse a fixer patch editing a tester-authored test, and verify the dispute path is taken instead
- [ ] 3.6 End the station on pass, cap or a round that changed nothing, and verify each ending sets its station event
- [ ] 3.7 Render the coverage comment and edit it in place by marker, and verify two rounds leave one comment holding paths and line numbers and no source text
- [ ] 3.8 Run the publication guard over a rendered report, and verify the verdict is clean

## 4. Mergeable pull request

- [ ] 4.1 Read `mergeStateStatus` in the loop's facts and make `mergeable` require clean, and verify with a table of every merge state
- [ ] 4.2 Treat a failed build check as work, and verify `failed-checks.py` returns it for a red build job
- [ ] 4.3 Merge a behind head's base in a model-free job and re-enter the testing gate, and verify on a throwaway repository
- [ ] 4.4 Resolve a trivial conflict (clean merge, or only generated files) and push, and verify on a throwaway repository with a conflicted CRD
- [ ] 4.5 For a non-trivial conflict set the loop `waiting`, comment the conflicted files, push nothing, and verify a person's comment resumes the loop
- [ ] 4.6 Verify the fixing loop cannot end `mergeable` while the testing station has not passed on the current head

## 5. Unit tests

- [ ] 5.1 Extend `.github/tests/conveyor.test.py` for the new states, events, budget, grant extension and merge-state decisions
- [ ] 5.2 Add shell tests for the diff intersection, the report renderer, the path refusals and the conflict classifier, and run `.github/tests/run.sh`
- [ ] 5.3 Run `python3 .github/scripts/publication-guard.py` and `retired-vocabulary-guard.py` and record the verdicts

## 6. System tests

- [ ] 6.1 Dispatch `gh workflow run system-smoke.yml --ref change/conveyor-testing-station` and verify the instrumented operator yields coverage data
- [ ] 6.2 Run the station once on a throwaway pull request carrying a known coverage shortfall, and verify it writes the report, loops, and ends `station:fix`
- [ ] 6.3 Run the console change through `e2e-live` and `npm run test:e2e` to verify browser coverage collects
- [ ] 6.4 Record any step a remote session cannot perform as not performed

## 7. Documentation

### 7.1 Reference docs

- [ ] 7.1.1 Update `docs/testing.md` with the diff floors, the instrumented system run and the browser coverage
- [ ] 7.1.2 Update `docs/diagrams/conveyor-lifecycle.mmd` and `conveyor-lifecycle-implementation.mmd` and regenerate any SVG from its source
- [ ] 7.1.3 Update `CONTRIBUTING.md` with the testing station and the mergeability rule
- [ ] 7.1.4 Update `.claude/rules/worktree-delivery.md` and `gotchas.md` for the new station and its lessons
- [ ] 7.1.5 Add a `docs/CHANGELOG.md` entry, and run `python3 .github/scripts/docs-generate.py --check`
- [ ] 7.1.6 Run `python3 .claude/scripts/rules_compliance.py` over the touched markdown

### 7.2 Adopter site

- [ ] 7.2.1 Check the landing page, Getting started, Installation and the guides for a page describing the delivery workflow, update it, or tick a line stating none does
