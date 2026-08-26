## ADDED Requirements

### Requirement: A change is reviewed per component, each in isolation

The review SHALL read a pull request as one reading per changed component,
each in a context that holds that component's diff, the rules that apply to
its path, and its own standing threads — and nothing from any other component.
Those readings SHALL run concurrently.

**The cost of a review is then the largest component changed, not the whole
pull request.** A single context reading everything serially pays for every
rule file whether or not the diff touches its subject, and for every component
whether or not it changed.

A per-component reading SHALL post nothing. It returns what it found as data;
what reaches the pull request is written once, by the consolidating reading.

#### Scenario: A pull request touches three components

- **WHEN** the review runs
- **THEN** three readings run concurrently, each seeing only its component,
  and the pull request receives one set of comments and one summary

#### Scenario: A pull request touches one component

- **WHEN** the review runs
- **THEN** one reading runs, and the rules loaded are those for that path

### Requirement: The review is consolidated across components, on what changed

After the per-component readings, the review SHALL check the whole change for
compatibility: the names each reading reports as changed — identifiers,
fields, paths, environment variables — SHALL be resolved to their consumers
mechanically, and each consumer SHALL be checked against the change.

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

### Requirement: The reviewer's definition is part of the guarded review

The definition of a per-component reviewer — its instructions and the tools it
may use — SHALL live inside the review workflow file, which the review refuses
to run when a pull request's copy differs from the default branch's.

**A pull request may not rewrite the thing that judges it.** A reviewer defined
in a file the branch controls could be weakened by the branch it reviews; one
defined inside the guarded file inherits the guard.

#### Scenario: A pull request edits the reviewer

- **WHEN** a pull request changes the reviewer's definition
- **THEN** the review does not run on that pull request and says so, exactly
  as it does for any other edit to the review workflow
