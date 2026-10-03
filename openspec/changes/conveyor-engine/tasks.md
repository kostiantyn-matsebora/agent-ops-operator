## 1. The engine core — backend-developer

- [x] 1.1 Create `tools/conveyor-engine/` as a standalone Python package (no import of `conveyor.py`, `conveyor_io.py`, or `load_script.py`, and no file under `.github/scripts/` or `.github/workflows/` touched) with a loader module that reads `.github/conveyor-model/workflows.desired.yaml` into the engine's own in-memory shape (workflows, states, transitions), and verify a unit test loads the real file and asserts every declared workflow name is present
- [x] 1.2 Implement the trigger matcher: given a workflow name, a subject's current state label, and an event name, look up the one matching transition (or none), and verify a unit test covers a match, a non-match, and two transitions on the same event with different guards
- [x] 1.3 Implement the guard registry and expression evaluator: a `dict[str, Callable]` of predicate name to function, a parser for `AND`/`OR`/`NOT` over bare names, and verify a unit test evaluates a multi-clause guard string against a registry of fake predicates for both outcomes
- [x] 1.4 Create a label-mapping loader that reads `.github/conveyor-model/labels.yaml` into the engine's own in-memory shape (prefixes, triggers, states), and verify a unit test loads the real file and asserts every prefix's `kind`, `native_subject`, `propagation` and `set_manually_on` fields are present
- [x] 1.5 Implement the state writer: given a subject, the workflow's own state vocabulary, and a next-state value, look up the new label's prefix in the label mapping, add the new label and remove every other label in that vocabulary in one API call, and — where the prefix's `propagation` is `bidirectional` — write the matching label on the subject's related pull request(s) or related issue in the same call, failing by logging rather than raising, and verify a unit test with a stubbed GitHub client asserts the label set after a successful write, after a propagated write, and after a simulated API failure
- [x] 1.6 Implement the `owned_by` stub registry: every action name referenced anywhere in `workflows.desired.yaml` resolves to a function that logs one structured line naming the action, the workflow, the subject, and the facts it was called with, and performs no other effect, and verify a unit test asserts no outbound API call happens when a stubbed action runs
- [x] 1.7 Wire the five pieces into one `evaluate(workflow_name, subject, event_name, facts)` entry point that reads the current state, matches the trigger, evaluates the guard, writes the new state (with propagation) if permitted, and calls the `owned_by` stub, and verify a unit test drives a full transition end to end against `conveyor.implement`'s real declared shape
- [x] 1.8 Add a test, run as part of the package's own suite, asserting consistency between `workflows.desired.yaml` and `labels.yaml`: every state and every event the workflow file declares has exactly one matching entry in the label mapping, and every entry's declared owning workflow(s) match the workflow file exactly, failing with the mismatch named

## 2. Guard predicates — backend-developer

- [ ] 2.1 Implement `has_access`, reading the real actor's collaborator permission from the GitHub API via a thin `gh` wrapper owned by this package (not `conveyor_io.py`), failing closed (treated as no access) on an unreadable permission, and verify a unit test for granted, denied, and unreadable
- [ ] 2.2 Implement `is_session_at_work`, reading whether a pull request is already open from the subject's own branch, failing closed (treated as a session already at work) on an unreadable pull request list, and verify a unit test for each case
- [ ] 2.3 Implement `is_change_finished`, reading the bound change's tasks file and checking every task is ticked, failing closed (treated as not finished) on an unreadable file, and verify a unit test for finished, unfinished, and unreadable
- [ ] 2.4 Implement `has_open_prs`, `all_prs_mergeable`, `pr_is_mergeable`, `all_prs_merged`, and `master_is_green`, each reading the real pull request and check-run state needed, each failing closed toward "not yet" rather than "proceed," and verify a unit test per predicate covering its true, false, and unreadable cases
- [ ] 2.5 Implement `is_capped` and `reset`, reading the loop's own round count and cap against the same bound this repository's `conveyor-lifecycle` capability already publishes, and verify a unit test for under the cap, at the cap, and a granted reset
- [ ] 2.6 Implement `proposal_pr_is_mergeable` and `hotfix_pr_is_created`/`all_checks_ran`, reading the specific pull requests `conveyor.propose` and `conveyor.fix` name, and verify a unit test per predicate
- [ ] 2.7 Implement `review_run_succeeded`, `has_open_review_threads`, and `review_run_skipped` for the `review` workflow, reading the pull request's own review run conclusion, its open review-thread count, and the run's own skip condition (never re-derived — read from the run that already happened), each failing closed toward "not yet landed" rather than "clean," and verify a unit test per predicate covering its true, false, and unreadable cases
- [ ] 2.8 Add a test asserting every bare predicate name referenced anywhere in `workflows.desired.yaml`'s guard strings resolves to a registered function, failing on a name with no implementation

## 3. Unit tests

- [ ] 3.1 Run the engine's own test suite (tasks 1.1–1.8, 2.1–2.8) standalone, with no network and no real GitHub call (every guard's I/O boundary stubbed in its test), and verify every test passes
- [ ] 3.2 Confirm `.github/tests/run.sh` needs no change and still passes unmodified: this package is not wired into it, since no file under `.github/` is touched or depends on `tools/conveyor-engine/`, and record that confirmation in this task

## 4. E2E tests

- [ ] 4.1 Nothing here is decided by a cluster. The engine reads GitHub API state and writes GitHub labels, invoked by nothing in production in this change. No kubelet, RBAC rule, informer, pod lifecycle, or context-continuity behavior is touched.

## 5. Documentation

### Reference docs

- [ ] 5.1 Confirm `docs/CHANGELOG.md` needs no entry (the package ships unused by production in this change, so there is no upgrade step or behavior change for an adopter), and record that confirmation in this task
- [ ] 5.2 Confirm `docs/concepts.md` and `docs/contracts.md` need no change (the conveyor is internal delivery tooling, not part of the published product contract), and record that confirmation in this task
- [ ] 5.3 Confirm `docs/security.md`'s fixing-loop push-credential section needs no change (it describes `conveyor:fix` and `autofix-guard.py` as they exist today, and this change touches neither), and record that confirmation in this task
- [ ] 5.4 Add a `.claude/rules/gotchas.md` or `structure.md` entry (whichever this repository's own convention points to for a new top-level tooling directory) naming `tools/conveyor-engine/` as the standalone, unwired home of the declarative conveyor engine, so the next reader does not assume it is live, and naming the follow-up change as where it gets wired in

### Adopter site

- [ ] 5.5 Confirm the landing page, Introduction, Getting started, Installation page, and every guide under `docs/guides/` need no change (none describes the conveyor), and record that confirmation in this task
