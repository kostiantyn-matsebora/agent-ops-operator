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
#106 eight readings started in pairs over ten minutes and the last never
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

### Requirement: The review is consolidated across components, on what changed

After the per-component readings, the review SHALL check the whole change for
compatibility: the names each reading reports as changed — identifiers,
fields, paths, environment variables — SHALL be resolved to their consumers
mechanically, and each consumer SHALL be checked against the change.

The consolidation SHALL be a reading of its own — the COORDINATOR — that runs
as its own job once every reading's job has finished, and is handed every
reading's data as files: a reading whose job produced none is handed as
absent, by name. It SHALL never wait on a running reading, and it is the only
reading that writes to the pull request.

**This repository's modules import nothing from one another**, so a contract
change compiles everywhere, passes every module's tests, and breaks at runtime
in a component the diff never names. The contract file reads as correct because
it is; it is no longer what its consumers speak. Only a reading that follows
the name to where it is used can see that.

The summary SHALL state the reach that was checked, so a reader sees what the
review considered rather than trusting that it considered everything.

#### Scenario: A contract field is renamed

- **WHEN** a pull request renames a field in an HTTP contract and updates the
  manager's handler
- **THEN** every adapter that speaks the old name is found and a finding is
  raised against each that still does

#### Scenario: A change reaches nothing outside itself

- **WHEN** the changed names have no consumer outside the changed components
- **THEN** the summary says so, and no cross-component finding is raised

#### Scenario: The coordinator is the only writer

- **WHEN** the readings have returned
- **THEN** exactly one reading posts to the pull request, once, and a run in
  which it posted no summary is reported as failed

#### Scenario: A reading's job failed

- **WHEN** a reading's job ended without a validated reading
- **THEN** the coordinator still runs, with that component handed as absent,
  and the summary names it as unreviewed

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
