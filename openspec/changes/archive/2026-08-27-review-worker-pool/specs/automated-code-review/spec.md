## MODIFIED Requirements

### Requirement: A change is reviewed per component, each in isolation

The review SHALL read a pull request as one reading per changed component,
each in a context that holds that component's diff, the rules that apply to
its path, and its own standing threads — and nothing from any other component.
Those readings SHALL run concurrently, bounded by a pool the runtime sizes,
and SHALL be started and collected by a SCRIPT the review runs — never by a
model deciding, turn by turn, what to spawn next or how long to wait.

**The cost of a review is then the largest component changed, not the whole
pull request.** A single context reading everything serially pays for every
rule file whether or not the diff touches its subject, and for every component
whether or not it changed.

**And a reading cannot be lost.** A reading started by a model's turn ends
with that turn; on pull request #74 three readings were abandoned that way and
the review reported success having posted nothing. A reading the script
started is returned to the script, or the run fails saying which one did not.

A per-component reading SHALL post nothing. It returns what it found as
DATA, in a stated shape the script validates; a reading that returns prose
instead is a failed reading, reported as such, never a silent gap.

#### Scenario: A pull request touches three components

- **WHEN** the review runs
- **THEN** three readings run concurrently, each seeing only its component,
  and the pull request receives one set of comments and one summary

#### Scenario: A pull request touches one component

- **WHEN** the review runs
- **THEN** one reading runs, and the rules loaded are those for that path

#### Scenario: A pull request touches more components than the pool holds

- **WHEN** the review runs with more components than the runtime's concurrent
  agent cap
- **THEN** every component is still read, the readings beyond the cap queue
  behind the first, and the run's record shows how many ran at once

#### Scenario: A reading returns nothing usable

- **WHEN** a per-component reading ends without data in the stated shape
- **THEN** the review reports that component as unreviewed, by name, and the
  remaining readings are consolidated as usual

### Requirement: The review is consolidated across components, on what changed

After the per-component readings, the review SHALL check the whole change for
compatibility: the names each reading reports as changed — identifiers,
fields, paths, environment variables — SHALL be resolved to their consumers
mechanically, and each consumer SHALL be checked against the change.

The consolidation SHALL be a reading of its own — the COORDINATOR — that is
handed every per-component reading's data by the script once all have
returned. It SHALL never wait on a running reading, and it is the only reading
that writes to the pull request.

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

### Requirement: The reviewer's definition is part of the guarded review

The definitions of the review's roles — the per-component reviewer and the
coordinator, each its instructions and the tools it may use — and the script
that runs them SHALL be taken from the DEFAULT BRANCH when the review runs,
never from the pull request's checkout.

**A pull request may not rewrite the thing that judges it.** A definition read
from the branch under review could be weakened by that branch; one restored
from the default branch before the run inherits the workflow file's own
guard, which the review already refuses to run when a pull request's copy
differs.

The definitions MAY therefore live in ordinary files — agent definitions and a
saved workflow — so the same review is runnable from a checkout by hand, and
each role is one file with one system prompt.

#### Scenario: A pull request edits the reviewer

- **WHEN** a pull request changes a role definition or the review's script
- **THEN** the review runs with the default branch's copies and says so on the
  pull request; the edited files are reviewed like any other change

#### Scenario: The review is run from a checkout

- **WHEN** a person runs the review's command locally against a pull request
- **THEN** the same roles and script run, from that checkout's copies
