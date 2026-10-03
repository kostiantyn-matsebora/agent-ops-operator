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

**Scoped down from the original proposal, before implementation started.**
This change builds and proves the engine as a standalone package, isolated
from the production line.

- No file under `.github/scripts/` or `.github/workflows/` is touched.
- `conveyor.py` and `conveyor_io.py` are neither imported nor referenced.
- The only thing it reads from `.github/` is the two already-merged model
  files (`workflows.desired.yaml`, `labels.yaml`), as data.
- Nothing in production changes behavior as a result of this change
  landing.

Rewiring the real callers onto this engine, and wiring `claude-review.yml`
for the `review` workflow, are a follow-up change — proposed once this one
proves the engine correct on its own.

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
  `all_prs_mergeable`, `all_prs_merged`, `pr_is_mergeable`, `has_open_prs`,
  `master_is_green`, `is_capped`, and every other bare predicate the YAML
  names. Each takes no explicit argument, evaluated implicitly against the
  workflow's own declared `subject` (an issue or a pull request).
- **`owned_by:` actions are stubs.** Every named action (`fire`,
  `carry_archive`, and so on) is a no-op that logs what it would have done.
  State storage, trigger recognition, and guard evaluation are real and
  load-bearing. The side effects a transition's owner performs are not, in
  this change.
- **The `review` workflow's declared shape stands in `workflows.desired.yaml`
  and `labels.yaml` already** (merged ahead of this change) — `subject:
  pull_request`, trigger prefix `review:*`, state prefix `scan:*`. This
  change wires nothing to call it. The engine runs it the same as every
  other declared workflow, once something invokes `evaluate()` for it —
  which is the follow-up change's job.
- **`conveyor.py` stays exactly as it is, untouched.** No real caller is
  rewired in this change. The engine is proven against the real
  `workflows.desired.yaml` and `labels.yaml` through its own tests, with no
  production code calling it yet.

## Capabilities

### New Capabilities

- `conveyor-engine`: the generic engine — loading a workflow YAML, mapping a
  real GitHub event to a trigger, evaluating a named guard against live
  state, writing the resulting state as a label, and the registry of
  stubbed `owned_by:` actions. Ships as a standalone package nothing in
  production calls yet.

No other capability is added or modified by this change. `review-lifecycle`
and the `conveyor-lifecycle` modification described in the original
proposal both depend on real callers invoking the engine, which this change
does not do — they move to the follow-up change, where they will be true.

## Impact

- **Code**: `.github/scripts/conveyor.py` is untouched. A new, standalone
  package at `tools/conveyor-engine/` (settled in design.md) reads
  `.github/conveyor-model/workflows.desired.yaml` and `labels.yaml` as data.
  Nothing under `.github/scripts/` or `.github/workflows/` changes.
- **Docs to re-check**: none yet. `docs/security.md`'s fixing-loop section
  describes `conveyor:fix` and `autofix-guard.py` as they exist today, and
  this change does not touch either — it is re-read once the follow-up
  change rewires them.
- **Reference docs made untrue**: none — `conveyor.py` and its mechanism
  were never documented in `docs/concepts.md` or `docs/contracts.md` (they
  are internal tooling, not part of the published product contract).
  `docs/CHANGELOG.md` needs no entry: nothing an adopter runs changes.
- **Adopter site**: none. The conveyor is this repository's own delivery
  tooling, not a capability an adopter installs or configures.
