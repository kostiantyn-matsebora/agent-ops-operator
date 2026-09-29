# automated-code-review Specification (delta)

## ADDED Requirements

### Requirement: A component's readers hold the review criteria of the role routed to its paths

The fixed context of a component's file readers SHALL additionally carry the
review criteria of the role agent routed to that component's paths, ahead of
anything specific to a file.

The routing from a path to its role SHALL be a program, beside the routing
from a path to its rules.

Three properties of the rules routing SHALL hold for the role too:

1. The criteria are the same bytes for every reader of the component's job.
2. The role definitions a reading holds are taken from the base branch — a
   pull request may not rewrite the reviewer that judges it.
3. The reading contract is unchanged: the reader's role, its return shape
   and its tools stay the file-reviewer's.

#### Scenario: A console UI file is read

- **WHEN** a file reader starts for a path under `platform/console/ui/`
- **THEN** its fixed context holds the frontend role's review criteria beside
  the routed rules, and its return keeps the file-reviewer's shape

#### Scenario: Two files of one component are read

- **WHEN** two readers start for paths of the same component
- **THEN** the role criteria each holds are byte-identical, and the second is
  served from cache

#### Scenario: A pull request edits a role agent's definition

- **WHEN** a pull request changes a role agent's file and the review runs on
  that pull request
- **THEN** the readers hold the base branch's copy of the role, and the
  changed copy is itself a reviewed path

#### Scenario: A path no role fits

- **WHEN** a file reader starts for a path the role routing maps to no role
- **THEN** its fixed context holds the routed rules alone, exactly as before
  this requirement existed

#### Scenario: The verdict pass and the coordinator run

- **WHEN** the thread-verdict pass or the coordinator starts
- **THEN** its context holds no role criteria — judging a thread or
  consolidating readings is not judging a diff against a role's bar
