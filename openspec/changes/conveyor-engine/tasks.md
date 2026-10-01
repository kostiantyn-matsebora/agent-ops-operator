## 1. The engine core — backend-developer

- [ ] 1.1 Create `.github/scripts/conveyor_engine/` as a Python package with a loader module that reads `.github/conveyor-model/workflows.desired.yaml` into the engine's own in-memory shape (workflows, states, transitions), and verify a unit test loads the real file and asserts every declared workflow name is present
- [ ] 1.2 Implement the trigger matcher: given a workflow name, a subject's current state label, and an event name, look up the one matching transition (or none), and verify a unit test covers a match, a non-match, and two transitions on the same event with different guards
- [ ] 1.3 Implement the guard registry and expression evaluator: a `dict[str, Callable]` of predicate name to function, a parser for `AND`/`OR`/`NOT` over bare names, and verify a unit test evaluates a multi-clause guard string against a registry of fake predicates for both outcomes
- [ ] 1.4 Create a label-mapping loader that reads `.github/conveyor-model/labels.yaml` into the engine's own in-memory shape (prefixes, triggers, states), and verify a unit test loads the real file and asserts every prefix's `kind`, `native_subject`, `propagation` and `set_manually_on` fields are present
- [ ] 1.5 Implement the state writer: given a subject, the workflow's own state vocabulary, and a next-state value, look up the new label's prefix in the label mapping, add the new label and remove every other label in that vocabulary in one API call, and — where the prefix's `propagation` is `bidirectional` — write the matching label on the subject's related pull request(s) or related issue in the same call, failing by logging rather than raising, and verify a unit test with a stubbed GitHub client asserts the label set after a successful write, after a propagated write, and after a simulated API failure
- [ ] 1.6 Implement the `owned_by` stub registry: every action name referenced anywhere in `workflows.desired.yaml` resolves to a function that logs a `::notice::` line naming the action, the workflow, the subject, and the facts it was called with, and performs no other effect, and verify a unit test asserts no outbound API call happens when a stubbed action runs
- [ ] 1.7 Wire the five pieces into one `evaluate(workflow_name, subject, event_name, facts)` entry point that reads the current state, matches the trigger, evaluates the guard, writes the new state (with propagation) if permitted, and calls the `owned_by` stub, and verify a unit test drives a full transition end to end against `conveyor.implement`'s real declared shape
- [ ] 1.8 Add a CI-run consistency check between `workflows.desired.yaml` and `labels.yaml`: every state and every event the workflow file declares has exactly one matching entry in the label mapping, and every entry's declared owning workflow(s) match the workflow file exactly, failing the build naming the mismatch

## 2. Guard predicates — backend-developer

- [ ] 2.1 Implement `has_access`, reading the real actor's collaborator permission from the GitHub API, failing closed (treated as no access) on an unreadable permission, and verify a unit test for granted, denied, and unreadable
- [ ] 2.2 Implement `is_session_at_work`, reading whether a pull request is already open from the subject's own branch, failing closed (treated as a session already at work) on an unreadable pull request list, and verify a unit test for each case
- [ ] 2.3 Implement `is_change_finished`, reading the bound change's tasks file and checking every task is ticked, failing closed (treated as not finished) on an unreadable file, and verify a unit test for finished, unfinished, and unreadable
- [ ] 2.4 Implement `has_open_prs`, `all_pr_mergeable`, `pr_is_mergeable`, `all_prs_merged`, and `master_is_green`, each reading the real pull request and check-run state needed, each failing closed toward "not yet" rather than "proceed," and verify a unit test per predicate covering its true, false, and unreadable cases
- [ ] 2.5 Implement `is_capped` and `reset`, reading the loop's own round count and cap against the same bound this repository's `conveyor-lifecycle` capability already publishes, and verify a unit test for under the cap, at the cap, and a granted reset
- [ ] 2.6 Implement `proposal_pr_is_mergeable` and `hotfix_pr_is_created`/`all_checks_ran`, reading the specific pull requests `conveyor.propose` and `conveyor.fix` name, and verify a unit test per predicate
- [ ] 2.7 Add a CI-run test asserting every bare predicate name referenced anywhere in `workflows.desired.yaml`'s guard strings resolves to a registered function, failing the build on a name with no implementation

## 3. Rewire `remote-implement.py` — backend-developer

- [ ] 3.1 Replace `remote-implement.py`'s call into the deleted `conveyor.py` with a call into `conveyor_engine.evaluate` for `conveyor.implement`'s `implement_fired` transition, keeping its existing GitHub-facing behavior (the POST to the cloud routine's fire endpoint, the marker comment) as the caller's own code outside the engine, and verify the script's existing test suite passes unchanged in assertion but rewired in implementation
- [ ] 3.2 Verify end to end against a stubbed GitHub client: a label event for an actor with access, and one for an actor without, each producing the state transition (or refusal) `conveyor.implement`'s declared guard predicts

## 4. Rewire the remaining real callers — backend-developer

- [ ] 4.1 Rewire `carry.py`'s fix-station and archive-station paths onto `conveyor_engine.evaluate`, reading `conveyor.fix` and `conveyor.finalize`'s declared transitions respectively, and verify its existing test suite passes rewired
- [ ] 4.2 Rewire `dispatch-gate.py` onto `loop`'s declared transitions, preserving its existing trigger-building logic (resolving the pull request, building a normalized event) as caller code outside the engine, and verify its existing test suite passes rewired
- [ ] 4.3 Rewire `land-dispatch.py`, `failed-checks.py`, `autofix-guard.py`, `recover-loop-state.py`, and `refresh-loop-state.py` onto the matching declared workflow's transitions each one drives, one at a time, and verify each script's existing test suite passes rewired before moving to the next
- [ ] 4.4 Remove every remaining import of or reference to the deleted `conveyor.py` from `.github/scripts/`, and verify `git grep conveyor\\.py` across `.github/` finds no hit outside this change's own history

## 5. Unit tests

- [ ] 5.1 Run `.github/tests/run.sh` (or the equivalent script-suite entry point this repository uses) and verify it passes with every rewired caller's own test file included
- [ ] 5.2 Run the conveyor engine's own test suite (tasks 1.1–1.8, 2.1–2.7) standalone and verify every test passes

## 6. E2E tests

- [ ] 6.1 Nothing here is decided by a cluster. The engine reads GitHub API state and writes GitHub labels through GitHub Actions. No kubelet, RBAC rule, informer, pod lifecycle, or context-continuity behavior is touched by this change.

## 7. Documentation

### Reference docs

- [ ] 7.1 Add an entry to `docs/CHANGELOG.md` naming the engine replacing `conveyor.py`, and verify the entry names every real caller rewired
- [ ] 7.2 Confirm `docs/concepts.md` and `docs/contracts.md` need no change (the conveyor is internal delivery tooling, not part of the published product contract), and record that confirmation in this task

### Adopter site

- [ ] 7.3 Confirm the landing page, Introduction, Getting started, Installation page, and every guide under `docs/guides/` need no change (none describes the conveyor), and record that confirmation in this task
