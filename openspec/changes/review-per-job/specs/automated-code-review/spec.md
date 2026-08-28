## MODIFIED Requirements

### Requirement: A change is reviewed per component, each in isolation

The review SHALL read a pull request as one reading per changed component,
each in its own CI job, on its own runner, in its own process, all started at
once and bounded only by the platform's job limit — never by a pool sized
from one runner's processors, and never by a model deciding, turn by turn,
what to spawn next or how long to wait. The queue of components — which
readings exist and what each is handed — SHALL be built by a program from the
changed paths, never by a model.

Within a component, the reading SHALL be made PER FILE: one reader per
changed file, whose context holds that file's diff, its own standing threads,
the names of the component's other changed files, and the rule files that
apply to that path — read on demand, never inherited — and nothing else. The
file readers SHALL be started and collected by a script, and their readings
merged by it into the component's, with a file whose reader returned nothing
usable named as unread rather than dropped.

Each file reading SHALL return, beside its findings, the names the file
DECLARES — added, removed or renamed — and the names it REFERENCES from
outside itself, so that what happens between files is judged from the
readings and not by a context that holds every file at once.

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
either produces its data or is a failed job with a name; a file reading a
script started is returned to the script or named as unread.

**And a context does not grow with the diff.** A component reader holding
every changed file paid for all of them on every turn: on the first matrix
run one component of ten files and a thousand changed lines took four to nine
minutes on the same diff while the others took one to four. A file reader's
context is bounded by its file.

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

#### Scenario: A component with many changed files

- **WHEN** a component's diff spans many files
- **THEN** each file is read by its own reader in a context bounded by that
  file, the readings are merged by the script, and a file whose reader
  returned nothing usable is named as unread in the component's reading

#### Scenario: A name removed in one file is still used in another

- **WHEN** one file's reading declares a name removed or renamed and another
  file's reading references it
- **THEN** the review raises a finding against the referencing file, from the
  two readings, without a context that held both files

#### Scenario: A component the pull request does not touch

- **WHEN** the review runs on a pull request whose diff names no path in a
  component
- **THEN** no reading is started for that component — the queue holds only
  components with changed paths, so a one-file change is one reading

### Requirement: The review is consolidated across components, on what changed

After the per-component readings, the review SHALL check the whole change for
compatibility: the names each reading reports as changed — identifiers,
fields, paths, environment variables — SHALL be resolved to their consumers
mechanically, and each consumer SHALL be checked against the change. Consumers
INSIDE the change SHALL be found from the readings' own references, and
consumers outside it by searching the repository.

The consolidation SHALL be a reading of its own — the COORDINATOR — that runs
as its own job once every reading's job has finished, and is handed every
reading's data as files: a reading whose job produced none is handed as
absent, by name. It SHALL never wait on a running reading, and it is the only
reading that writes to the pull request. Its context SHALL hold its role, the
readings and the threads, and no rule file; consumers inside the change SHALL
be judged from the readings' declares and references, and a consumer outside
it by reading that one file.

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

#### Scenario: A removed name is referenced by another changed file

- **WHEN** one file's reading declares a name removed or renamed and another
  file's reading references it
- **THEN** the coordinator raises a finding against the referencing file from
  the two readings, without reading either file

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

## ADDED Requirements

### Requirement: No reading carries the project's rules as inherited context

No context the review runs — a file reader, the component's script session,
the coordinator — SHALL inherit the project's rule files. A file reader SHALL
be told which rule files apply to its path and SHALL read those; the
coordinator SHALL read none. The routing from a path to its rules SHALL be a
program, and every rule file SHALL be reachable from some path — a rule no
path routes to is a rule the review has stopped enforcing, silently.

**The rules are what a reading is measured against, not what it thinks with.**
A context that inherits every rule file pays for all of them on every turn:
the coordinator, which reads no rules at all, spent 268 of its 273 seconds in
34 turns each re-sending some 76 thousand tokens, of which its readings and
threads were a few thousand.

#### Scenario: A file under the chart is read

- **WHEN** a file reader is started for a path under `chart/`
- **THEN** it is told to read the chart rule and the rules the chart depends
  on, reads those, and inherits no rule file

#### Scenario: The coordinator runs

- **WHEN** the consolidation starts
- **THEN** its context holds its role, the readings and the threads, and no
  rule file

#### Scenario: A rule file no path routes to

- **WHEN** a rule file exists that the routing program maps from no path
- **THEN** the routing program's test fails, naming the rule
