## Why

The conveyor's rules lived in one hardcoded Python file (`conveyor.py`).
Every change to the line's behavior was a change to that file's tables and
functions.

`.github/conveyor-model/workflows.desired.yaml` already states the target
shape as data — states, triggers, guards. A `python-statemachine` spike
already proved a declarative YAML of this shape can be loaded and run.

`conveyor.py` has been deleted from this line of work entirely. Nothing here
is ported, wrapped, or referenced from it. The engine is built fresh, and the
YAML is its only source.

## What Changes

- **A new workflow engine loads `workflows.desired.yaml` and runs it for
  real.** No workflow's shape — states, transitions, which trigger moves which
  state, which guard gates which transition — is hardcoded anywhere in code.
  The YAML is the only source. Changing a workflow's shape is an edit to the
  YAML, never to the engine.
- **A state is a label on an issue or a pull request.** The engine reads the
  live label set as the current state and writes the new state as a label,
  the same mechanism the conveyor has always used, generalized to work from
  any workflow the YAML declares rather than two hardcoded tables.
- **A trigger is a real GitHub event or action**, recognized and dispatched to
  the matching workflow/state by name, matching each workflow's own event
  vocabulary (`conveyor:implement`, `loop:fix`, `all_prs_merged`, and so on)
  — no event name is invented at runtime that the YAML does not declare.
- **A new file, `.github/conveyor-model/labels.yaml`, is the only source for
  which real label carries which state or trigger, and how it propagates.**
  `workflows.desired.yaml` states the abstraction. `labels.yaml` states the
  implementation. Every trigger whose nature is `label_placed` names its real
  label and the prefix it belongs to (`conveyor:*` on the issue, `loop:*` on
  the pull request). Every state's id already is its real label
  (`station:*` on the issue, `round:*` on the pull request). A prefix's
  propagation rule — `bidirectional` or `none` — decides whether placing it
  on one subject writes the matching label on the other.
- **Every guard is a real predicate implementation**, evaluated against live
  GitHub state: `has_access`, `is_session_at_work`, `is_change_finished`,
  `all_pr_mergeable`, `all_prs_merged`, `pr_is_mergeable`, `has_open_prs`,
  `master_is_green`, `is_capped`, and every other bare predicate the YAML
  names. Each takes no explicit argument, evaluated implicitly against the
  workflow's own declared `subject` (an issue or a pull request).
- **`owned_by:` actions are stubs.** Every named action (`fire`,
  `carry_archive`, and so on) is a no-op that logs what it would have done.
  State storage, trigger recognition, and guard evaluation are real and
  load-bearing. The side effects a transition's owner performs are not, in
  this change.
- **The engine runs through the existing remote-session mechanism,
  unchanged.** `conveyor:implement` and `conveyor:run` placed on an issue
  already start a remote Claude session per the repository's own
  `remote-session.md` rule. This change wires the engine into that same
  path and invents no new one.
- **`conveyor.py` is deleted, with no replacement reusing its shape.** Every
  real caller that imported it (`carry.py`, `remote-implement.py`,
  `dispatch-gate.py`, `land-dispatch.py`, `failed-checks.py`,
  `autofix-guard.py`, `recover-loop-state.py`, `refresh-loop-state.py`) is
  rewired onto the new engine as part of this change. **BREAKING**: these
  scripts' internal shape changes. Their externally observed behavior does
  not, except where this proposal says so.

## Capabilities

### New Capabilities

- `conveyor-engine`: the generic engine — loading a workflow YAML, mapping a
  real GitHub event to a trigger, evaluating a named guard against live
  state, writing the resulting state as a label, and the registry of
  stubbed `owned_by:` actions.

### Modified Capabilities

- `conveyor-lifecycle`: the mechanism deciding every station and loop
  transition changes from a hardcoded Python table to the generic engine
  reading `workflows.desired.yaml`. The station and loop vocabulary itself
  grows to match the desired model (`ready`, `implementing`, `implemented`,
  `ready_to_merge`, `merge_failed`, `fixing`, `hotfix_created`, `finalizing`,
  `finalized`, and the rest), superseding the real, narrower vocabulary
  `conveyor.py` used. The grant and re-check rules this capability already
  publishes (a label is a grant only when a person placed it or a program
  carried one, re-checked at every transition) are preserved, now enforced
  by guard predicates rather than Python functions of the same name.

## Impact

- **Code**: `.github/scripts/conveyor.py` stays deleted. A new engine module
  under `.github/scripts/` (or a dedicated `.github/conveyor-engine/`
  directory — settled in design.md) reads `.github/conveyor-model/
  workflows.desired.yaml`. Every real caller listed above is rewritten
  against the new engine's interface. `.github/workflows/remote-implement.yml`
  and `.github/workflows/review-dispatch.yml` keep their trigger wiring
  (`issues: labeled`, `workflow_run`, `issue_comment`, `workflow_dispatch`)
  but call the new engine instead of the deleted file.
- **Reference docs made untrue**: none yet — `conveyor.py` and its mechanism
  were never documented in `docs/concepts.md` or `docs/contracts.md` (they are
  internal tooling, not part of the published product contract), and
  `docs/CHANGELOG.md` gets an entry once this change lands.
- **Adopter site**: none. The conveyor is this repository's own delivery
  tooling, not a capability an adopter installs or configures. No landing
  page, Introduction, Getting started, Installation page or guide under
  `docs/guides/` describes it or needs to change.
