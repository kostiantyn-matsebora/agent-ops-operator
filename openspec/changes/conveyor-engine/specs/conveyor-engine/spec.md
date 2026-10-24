## Purpose

The generic machine that runs every conveyor workflow from its own
declaration: loading a workflow's states and transitions, recognizing a real
event as a trigger, evaluating a named guard against live state, and writing
the resulting state where the line's state already lives.

## ADDED Requirements

### Requirement: A workflow's shape comes from its own declaration, never from code

The engine SHALL load every workflow it runs from a declared definition
naming its subject, its states, and each state's own transitions.

No workflow's states, transitions, or the mapping from a trigger and a guard
to a next state SHALL be written as code specific to that workflow.

Changing which trigger moves a workflow to which state, or which guard gates
a transition, SHALL be an edit to that workflow's declaration alone.

#### Scenario: A workflow's transition changes

- **WHEN** a workflow's declaration is edited to send an existing trigger to
  a different next state
- **THEN** the engine moves the line to the new state on that trigger, with
  no change to the engine itself

#### Scenario: A new workflow is declared

- **WHEN** a new workflow is added to the declaration the engine loads
- **THEN** the engine runs it the same way it runs every other declared
  workflow, with no workflow-specific code added

### Requirement: A state is the label already on the subject the workflow declares

The engine SHALL read a workflow's current state from the label set on the
issue or pull request the workflow declares as its subject, and SHALL write a
state transition the same way: as a label on that subject.

No state SHALL be held anywhere other than that label.

#### Scenario: The engine reads the current state

- **WHEN** the engine evaluates a workflow for an issue or pull request
  carrying one of that workflow's state labels
- **THEN** it treats that label as the workflow's current state

#### Scenario: The engine writes a new state

- **WHEN** a transition moves a workflow to a new state
- **THEN** the subject's label changes to the new state, and no other
  record of the state is written

### Requirement: A trigger is a real event, matched to a workflow's own vocabulary

The engine SHALL recognize a real GitHub event or action and match it to the
event name a workflow's current state declares a transition for. An event
with no matching declared transition for the current state SHALL move
nothing.

The engine SHALL NOT invent, infer, or synthesize an event name a workflow's
declaration does not itself name.

#### Scenario: A recognized event matches a declared transition

- **WHEN** a real event arrives whose name matches a transition declared on
  the workflow's current state
- **THEN** the engine evaluates that transition's guard

#### Scenario: An event has no matching transition

- **WHEN** a real event arrives whose name matches no transition on the
  workflow's current state
- **THEN** the engine leaves the state as it is

### Requirement: A guard is a named predicate, evaluated against live state

The engine SHALL evaluate a transition's guard by looking up its name in a
registry of real predicate implementations and calling it with no explicit
argument, reading whatever live GitHub state that predicate needs to decide
true or false.

A transition with no guard SHALL be treated as always permitted once its
event is recognized.

A guard combining several named predicates with `AND`, `OR`, or `NOT` SHALL
evaluate each by the same lookup, combined by that logic.

An event recognized for a transition whose guard is not satisfied SHALL move
nothing, even where another transition from the same state and the same
event, gated by a different guard, is satisfied.

Guards on transitions sharing one state and one event SHALL be mutually
exclusive. Where more than one evaluates true, the engine SHALL move nothing
and log an error naming the transitions, rather than pick one.

#### Scenario: A satisfied guard permits its transition

- **WHEN** a recognized event's transition carries a guard that evaluates true
- **THEN** the state moves to that transition's declared next state

#### Scenario: An unsatisfied guard blocks its transition

- **WHEN** a recognized event's transition carries a guard that evaluates
  false
- **THEN** the state does not move to that transition's next state

#### Scenario: Two guards on one event both evaluate true

- **WHEN** a recognized event matches two transitions from the current state
  and both of their guards evaluate true
- **THEN** the engine moves nothing and logs an error naming both transitions

#### Scenario: Two transitions on the same event, different guards

- **WHEN** a recognized event matches two transitions from the current state,
  each with a different guard
- **THEN** the state moves to whichever transition's guard evaluates true,
  and to neither if both evaluate false

### Requirement: A guard predicate fails closed

A guard predicate that cannot read the GitHub state it needs SHALL return
whichever of true or false treats the subject as already at work, already
granted, or already carrying whatever fact a wrong answer would otherwise
start unattended action on.

#### Scenario: A guard's underlying read fails

- **WHEN** a guard predicate's call to read GitHub state fails or returns
  nothing readable
- **THEN** the predicate returns the answer that does not start new
  unattended work

### Requirement: An action is a registered stub

Every `owned_by` action a transition names SHALL exist in a registry of named
functions. Calling one SHALL record what it would have done and SHALL NOT
perform a GitHub write, a label placement beyond the engine's own state
write, or any other side effect.

A transition's `owned_by` action, if named, SHALL be called after its
transition's state has been written, never before.

#### Scenario: A transition names an action

- **WHEN** a transition whose guard is satisfied names an `owned_by` action
- **THEN** the state moves, the action's name and the facts it was called
  with are recorded, and nothing beyond the state label is written

#### Scenario: A transition names no action

- **WHEN** a transition whose guard is satisfied names no `owned_by` action
- **THEN** the state moves and nothing is recorded beyond it

### Requirement: An invoked sub-workflow runs its own machine independently

A state declaring `invokes` on another workflow SHALL treat that workflow as
running its own machine, on its own subject, while the declaring state is
current.

The invoking state's own transitions SHALL fire on events naming the invoked
workflow's state entries, not on the invoked workflow's internal transitions.

#### Scenario: An invoking state's transition fires

- **WHEN** an invoked workflow reaches a state an invoking state's own
  transition names as its trigger
- **THEN** the invoking workflow evaluates that transition's guard against
  the invoking workflow's own subject

#### Scenario: The invoked workflow's internal transitions are invisible to the parent

- **WHEN** the invoked workflow moves between its own internal states
- **THEN** the invoking workflow's state does not change until the invoked
  workflow reaches a state the invoking transition names

### Requirement: A label's real name and propagation come from a declared mapping, never from code

The engine SHALL read, for every state and every `label_placed` trigger, its
real label string and its prefix from a declared mapping separate from the
workflow declaration itself.

No label string, and no rule for which subject a label is native to or
propagates to, SHALL be written as code.

#### Scenario: A label's propagation rule changes

- **WHEN** a prefix's declared propagation rule is edited
- **THEN** the engine propagates (or stops propagating) a label of that
  prefix between an issue and its pull requests accordingly, with no change
  to the engine itself

### Requirement: A prefix's declared propagation rule governs cross-subject label writes

A prefix declared `bidirectional` SHALL cause the engine to write the
matching label on an issue's related pull request when that label is placed
on the issue, and on the issue when that label is placed on any of its
related pull requests.

A prefix declared `none` SHALL cause the engine to write no label on any
other subject when that label is placed.

A prefix's declared `set_manually_on` list SHALL NOT restrict propagation
generated by the engine itself.

It states only where a person is expected to place the label directly. A
prefix mechanically wired for propagation propagates whether or not a person
ever places it on the subject the list excludes.

#### Scenario: A bidirectional trigger is placed on the pull request

- **WHEN** a label of a prefix declared `bidirectional` is placed on a pull
  request related to an issue
- **THEN** the engine writes the matching label on that issue

#### Scenario: A bidirectional trigger is placed on the issue

- **WHEN** a label of a prefix declared `bidirectional` is placed on an issue
- **THEN** the engine writes the matching label on every pull request related
  to that issue

#### Scenario: A state label propagates one-way in practice

- **WHEN** a state label of a prefix declared `bidirectional`, whose
  `set_manually_on` names only the issue, is written by the engine on an
  issue
- **THEN** the engine writes the matching label on every related pull
  request, and no process places that label on a pull request directly

#### Scenario: A non-propagating prefix stays on its own subject

- **WHEN** a label of a prefix declared `none` is placed on a pull request
- **THEN** the engine writes no label of that prefix on the issue the pull
  request relates to, or on any other pull request
