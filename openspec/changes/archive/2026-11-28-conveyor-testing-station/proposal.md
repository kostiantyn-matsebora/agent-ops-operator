## Why

The conveyor implements a change and then fixes review remarks. It never asks whether
the new behaviour is tested, and it ends a loop as `mergeable` on review state alone.

A change can reach the archive station with no test for the code it added, with a merge
conflict, or behind its base.

## What Changes

- A new station, `station:testing`, between `implement` and the fixing loop.
- The station checks coverage of the CHANGED lines against a floor per tier.
  Unit holds 90%, system 70%, e2e 50%.
- The station is a SESSION, not a CI job. It runs the unit, system and e2e tiers itself,
  in its own environment, and measures the coverage there.
- Tests that depend on the agent's behaviour run against a REAL model, with the
  credential read from an environment variable. The session judges, per change,
  whether the model's behaviour affects the functionality. A stub run never stands in
  for a real one where it does.
- A separate model writes the missing tests, never the model that wrote the code.
- The session loops: run the tiers, fix or write per the result, run again. It has
  its own budget of 5 rounds, extended by `conveyor:keep-going`.
- One comment on the tracking issue reports the numbers, edited in place each round.
- The fixing loop ends `mergeable` only when the pull request can merge: no failed
  check, no open thread, no unaddressed analysis issue, no conflict with its base,
  and the testing station passed on the current head.
- A trivial conflict is resolved by the line. A non-trivial one stops the loop and
  asks a person.

## Capabilities

### New Capabilities

- `conveyor-testing`: the testing station. Diff coverage floors per tier, test
  authorship by a separate model, the bounded test-and-fix loop, the issue report.

### Modified Capabilities

- `conveyor-lifecycle`: a `testing` station value, the station machine's new
  events, and `mergeable` requiring a clean merge state.

## Impact

- `.github/scripts/conveyor.py` and `conveyor.test.py`: station states, events, decisions.
- New scripts for the diff intersection, the verdict and the report.
- A routine file for the testing session, a tester agent definition, and the workflow step that fires the session.
- `.github/conveyor-model/` labels and workflow model.
- `platform/manager/test/system/` runs against an instrumented operator binary.
- `platform/console/ui/e2e*` collects V8 coverage.
- Reference docs made untrue: `docs/testing.md` (tiers, floors, instrumented runs), `docs/diagrams/conveyor-lifecycle*.mmd`, `CONTRIBUTING.md`, `docs/CHANGELOG.md`.
- Adopter site: the pages describing the delivery workflow. This is a repository-internal change, so a statement that no adopter page names the conveyor is recorded.
