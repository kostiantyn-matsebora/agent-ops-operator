## MODIFIED Requirements

### Requirement: A change is reviewed per component, each in isolation

The review SHALL read a pull request as one reading per changed component,
each in a context that holds that component's diff, the rules that apply to
its path, and its own standing threads — and nothing from any other component.
Each reading SHALL run as its own CI job, on its own runner, in its own
process, all started at once and bounded only by the platform's job limit —
never by a pool sized from one runner's processors, and never by a model
deciding, turn by turn, what to spawn next or how long to wait. The queue of
components — which readings exist and what each is handed — SHALL be built by
a program from the changed paths, never by a model.

**The cost of a review is then the largest component changed, not the whole
pull request.** A single context reading everything serially pays for every
rule file whether or not the diff touches its subject, and for every component
whether or not it changed — and a pool of two, which is what a four-processor
runner sizes, is serial reading wearing a concurrent shape: on pull request
#106 nine readings started in pairs over ten minutes and the last never
started.

**And a reading cannot be lost.** A reading started by a model's turn ends
with that turn; on pull request #74 three readings were abandoned that way and
the review reported success having posted nothing. A reading that is a job
either produces its data or is a failed job with a name.

A per-component reading SHALL post nothing. It returns what it found as
DATA, in a stated shape a program validates before it is consolidated; a
reading that returns prose instead is a failed reading, reported as such,
never a silent gap.

#### Scenario: A pull request touches three components

- **WHEN** the review runs
- **THEN** three readings run at once as three jobs, each seeing only its
  component, and the pull request receives one set of comments and one summary

#### Scenario: A pull request touches one component

- **WHEN** the review runs
- **THEN** one reading runs, and the rules loaded are those for that path

#### Scenario: A pull request touches more components than the pool holds

- **WHEN** the review runs with more components than one runner's processors
  would have pooled
- **THEN** every component is still read at once, and the review's wall-clock
  is that of its slowest single reading plus the consolidation

#### Scenario: A reading returns nothing usable

- **WHEN** a per-component reading ends without data in the stated shape
- **THEN** the review reports that component as unreviewed, by name, and the
  remaining readings are consolidated as usual

#### Scenario: The queue is built

- **WHEN** the review starts
- **THEN** the components, their paths, the standing threads and the change's
  delta specs are produced by a program without a model call, and the readings
  start only once that queue exists

#### Scenario: A component the pull request does not touch

- **WHEN** the review runs on a pull request whose diff names no path in a
  component
- **THEN** no reading is started for that component — the queue holds only
  components with changed paths, so a one-file change is one reading

### Requirement: The reviewer's definition is part of the guarded review

The definitions of the review's roles — the per-component reviewer and the
coordinator, each its instructions and the tools it may use — and the steps
that install the model's tooling and hand each role its input SHALL be taken
from the DEFAULT BRANCH when the review runs, never from the pull request's
checkout.

**A pull request may not rewrite the thing that judges it.** A definition read
from the branch under review could be weakened by that branch; one restored
from the default branch before the run inherits the workflow file's own
guard, which the review already refuses to run when a pull request's copy
differs.

The definitions SHALL live in ordinary files — one agent definition per role,
each one file with one system prompt — so the same review is runnable by hand
against any pull request, by dispatching the same workflow. The plan that
starts and collects the readings is the workflow itself; there is no second
copy of it to keep in step.

#### Scenario: A pull request edits the reviewer

- **WHEN** a pull request changes a role definition or the review's tooling
  step
- **THEN** the review runs with the default branch's copies and says so on the
  pull request; the edited files are reviewed like any other change

#### Scenario: The review is run from a checkout

- **WHEN** a person dispatches the review against a pull request from their
  own checkout
- **THEN** the same jobs run, with the same roles, and MAY stop after the
  readings without posting when asked to
