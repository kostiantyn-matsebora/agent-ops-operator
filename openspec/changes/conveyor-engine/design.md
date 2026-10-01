## Context

See `proposal.md` — Why, for the motivation.

`.github/conveyor-model/workflows.desired.yaml` already states six workflows
(`conveyor.propose`, `conveyor.implement`, `conveyor.fix`, `conveyor.finalize`,
`loop`, and the orchestrating `conveyor.run`) in a settled shape.

`states:` is a mapping keyed by state id, each state owns its own
`transitions:` list (`event`, `to`, `guard`, `owned_by`), guards are bare
`snake_case` predicates combined with `AND`/`OR`/`NOT` and take no explicit
argument, and a state may `invokes:` another workflow to run as its own
internal sub-machine.

`conveyor.py` and every table it held are deleted from this branch already.
Nothing in this design references its shape, its function names, or its fact
dataclasses.

The conveyor already runs inside GitHub Actions, triggered by `issues:
labeled`, `workflow_run` completions, `issue_comment`, and `workflow_dispatch`,
and a `conveyor:implement`/`conveyor:run` label already starts a remote Claude
Code session through the mechanism `remote-session.md` and the
`remote-change-sessions` spec describe. This design does not change how a
session starts.

## Goals / Non-Goals

**Goals:**

- Load `workflows.desired.yaml` once and run every workflow it declares
  through one generic interpreter.
- Store every workflow's current state as a GitHub label on its declared
  subject, exactly as the conveyor has always stored state.
- Recognize real GitHub events as triggers, matched against each workflow's
  own declared vocabulary.
- Evaluate every named guard against live GitHub state through a real
  predicate implementation, fail-closed on an unreadable fact.
- Replace every real caller of the deleted `conveyor.py` with a call into
  this engine.

**Non-Goals:**

- Implementing any `owned_by` action's real side effect. Every one is a
  logging stub in this change.
- Changing the remote-session start mechanism, the GitHub Actions trigger
  surface (`issues: labeled`, `workflow_run`, `issue_comment`,
  `workflow_dispatch`), or anything `remote-change-sessions` already
  specifies.
- A UI, dashboard, or any new way for a person to inspect the line beyond
  reading its labels, which is unchanged.
- Migrating `workflows.desired.yaml`'s own shape. The engine runs whatever
  shape the YAML already has. Evolving that shape is a separate change.

## Decisions

### 1. The engine is a single Python package under `.github/scripts/conveyor_engine/`

A package, not a script. It has four independently testable layers (loader,
label mapping, trigger matcher, guard registry), each worth its own test
file.

**Alternative considered: one flat script.** Rejected. A flat script big
enough to hold a YAML loader, a trigger matcher and a guard registry
recreates exactly the one-file-does-everything shape this change exists to
retire.

### 2. The guard registry is a plain name-to-function dictionary, resolved at call time

Every bare predicate name a workflow's guard string can contain
(`has_access`, `is_session_at_work`, `all_prs_merged`, and so on) maps to one
Python function.

That function takes the workflow's resolved subject (an issue or pull
request handle) and returns `bool`. A guard string is parsed into a small
boolean expression over these names, joined by `AND`/`OR`/`NOT`, and
evaluated by looking each name up in the registry.

**Alternative considered: compile each guard string to a Python `eval`
expression.** Rejected. Event payloads and issue bodies are untrusted
elsewhere in this project, and a registry lookup refuses an unknown name
outright, where `eval` would accept any syntactically valid Python.

### 3. An event is matched by a direct lookup against the subject's current state, never inferred

The engine receives a real event already normalized by the caller: the
subject's number, the event's own name as a string, and whatever payload
fields a guard predicate needs.

It looks up, for that subject's current state, a transition whose declared
`event` string matches the payload's event name exactly.

**Alternative considered: a generic event bus matching on event type alone,
dispatching to every subscribed workflow.** Rejected. Most of this project's
own recorded incidents involve an event firing more broadly than intended.

A direct, per-call lookup keeps the matching rule visible at the one call
site rather than inside a dispatch table nobody reads twice.

### 4. State lives only as a label, read and written by the engine itself

Reading the current state means reading the live label set on the subject.
Writing a new state means adding the new label and removing every other
label in that workflow's own vocabulary, in one call.

No separate store, file, or second label family holds the state. This is the
same property the conveyor has always had — the change is who computes the
next state, not where the result is kept.

### 4a. The label mapping is its own file, read once, separate from the workflow declaration

`.github/conveyor-model/labels.yaml` names every real label a state or a
`label_placed` trigger uses, which prefix it belongs to, and that prefix's
propagation rule. `workflows.desired.yaml` never names a real label string
directly beyond what a state's or trigger's own id already is.

The engine loads this file once alongside the workflow declarations.

Every label read or write goes through it rather than a literal string
written at the call site.

**Alternative considered: fold the label mapping into `workflows.desired.yaml`
itself.** Rejected. Labels are the implementation of the workflow
abstraction, not part of it — the same split `labels.yaml`'s own header
states.

Mixing them would mean an edit to which real string carries a state is also
an edit to the abstract workflow file, for no abstract reason.

### 4b. A prefix's propagation rule is applied by the state writer, not by each caller

When the engine writes a label whose prefix is declared `bidirectional`, it
also writes the matching label on the subject's related pull request(s) (if
written on an issue) or on the related issue (if written on a pull request).

Both writes happen in the same call.

A caller never computes or requests propagation itself. The propagation
rule lives in one place, so a workflow that later moves between prefixes, or
a prefix whose rule changes, needs no caller rewritten.

### 5. A failed state write never fails the calling job

A write that cannot complete (the API call fails, the subject cannot be
read) logs the failure and returns, rather than raising past the caller.

The next real trigger on that subject re-derives the state from whatever
label is actually there, so a dropped write is self-correcting rather than
wedging the line.

### 6. `owned_by` stubs log one structured line per call

A stub records the action's name, the workflow, the subject, and the facts
available at the call, as a GitHub Actions annotation (`::notice::`) so it
is visible in the run's own log without a second reporting mechanism.

## Risks / Trade-offs

- **A guard predicate's own correctness is now the single point every
  transition depends on, with no table test enumerating every (state, event)
  pair the way `conveyor.py`'s own test suite did.** Mitigation: the workflow
  YAML's own completeness (every state's transitions are already declared)
  plus a test asserting every guard name a workflow references resolves to
  a registered predicate, run against the live YAML in CI.
- **Stubbed actions mean this change cannot, by itself, replace the
  production conveyor.** A session merging this change still needs
  `owned_by`'s real side effects implemented in a later change before any
  cutover. Mitigation: explicit in the proposal's Non-Goals and this design's
  Non-Goals, and the delta spec only changes the DECISION mechanism, not the
  grant and bound behavior `conveyor-lifecycle` already publishes.
- **A fail-closed guard that is wrong in the closed direction silently
  blocks real work.** Mitigation: the chosen failure direction (block, never
  proceed) is stated once per predicate in its own implementation and
  covered by a test asserting it on a simulated read failure, rather than
  left to be rediscovered per caller.

## Migration Plan

1. Build the engine and its guard registry against `workflows.desired.yaml`,
   with every `owned_by` action stubbed, with no real caller rewired yet.
2. Rewire one real caller (`remote-implement.py`, the simplest: one event,
   one decision) onto the engine, keeping every other caller on hold.
3. Rewire the remaining callers (`carry.py`, `dispatch-gate.py`,
   `land-dispatch.py`, `failed-checks.py`, `autofix-guard.py`,
   `recover-loop-state.py`, `refresh-loop-state.py`) one at a time, each its
   own task with its own test, so a regression in one caller's rewiring is
   caught before the next is attempted.
4. No rollback step beyond reverting the change: every `owned_by` action is a
   stub, so no production side effect depends on the engine being correct
   yet, and nothing this change does is irreversible on its own.
