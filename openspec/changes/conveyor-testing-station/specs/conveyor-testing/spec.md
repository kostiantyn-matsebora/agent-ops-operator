## Purpose

The testing station of the conveyor. It proves a change's new code is tested at
each tier, loops until the tests pass and the floors hold, and reports the
numbers on the change's tracking issue.

## ADDED Requirements

### Requirement: The testing station sits between implement and the fixing loop

When the implement session opens the change's pull request, the issue's station
label SHALL read `station:testing`. The station SHALL end in one of two ways.
Passing moves the line to the fixing loop. Exhausting the budget moves it to
`stalled`.

#### Scenario: Implement hands over to testing

- **WHEN** the implement session opens the pull request
- **THEN** the issue carries `station:testing` and no other station label

#### Scenario: Passing moves the line on

- **WHEN** every tier passes and holds its floor on the current head
- **THEN** the station label becomes `station:fix` and the fixing loop starts

### Requirement: Coverage is judged on the lines the change added

Each tier SHALL be measured over the executable lines the pull request adds or
changes, never over the repository. A tier SHALL apply only to the code it can
exercise. A tier with no executable changed line in its domain SHALL be reported
`not applicable` and SHALL hold no floor.

| Tier | Floor | Domain |
|---|---|---|
| unit | 90% | every changed executable line |
| system | 70% | changed Go lines in the operator and components the system pack runs |
| e2e | 50% | changed lines under the console UI source |

#### Scenario: A change below the unit floor

- **WHEN** 85% of the changed executable lines are covered by unit tests
- **THEN** the unit tier fails its floor and the round has work

#### Scenario: A tier with nothing to measure

- **WHEN** the change touches no console UI source line
- **THEN** the e2e tier is `not applicable` and holds no floor

### Requirement: The system tier is measured on an instrumented operator

The system pack SHALL run against an operator binary built with coverage
instrumentation, and SHALL collect its coverage data when the run ends. The e2e
tier SHALL collect browser coverage of the console UI source. A tier that ran
but yielded no coverage data SHALL fail, never read as zero changed lines.

#### Scenario: Missing coverage data fails the tier

- **WHEN** the system pack ran and wrote no coverage data
- **THEN** the system tier reports a collection failure and the station does not pass

### Requirement: Missing tests are written by a model that did not write the code

A shortfall of coverage SHALL be answered by a separate session, a different
model from the one that implemented the change. That session SHALL read the
change's specs, its design and the public interfaces. It SHALL NOT read the
implementing session's transcript. It SHALL write only test files, and a patch
touching any other path SHALL be refused.

A test the separate model wrote SHALL NOT be edited by the fixing session. A
fixer that holds such a test wrong SHALL dispute it, and the separate model SHALL
judge the dispute.

#### Scenario: The tester writes outside test paths

- **WHEN** the separate model's patch changes a production file
- **THEN** the patch is refused and the round records a failure

#### Scenario: The fixer disagrees with a test

- **WHEN** the fixing session believes a tester-authored test is wrong
- **THEN** it raises a dispute and does not edit the test

### Requirement: A failing test is answered in the code, never by weakening the test

A failing test SHALL be work for the fixing session, which changes production
code. A tier that fails SHALL be re-run before any patch, and a red that does not
reproduce SHALL be re-run on every round the budget allows before a person is asked.

#### Scenario: A failing unit test

- **WHEN** a tester-authored unit test fails on the head
- **THEN** the round's work item is the production code, and the test is unchanged

### Requirement: The testing station loops until the tests pass, within its own budget

Each round SHALL run the three tiers, then fix or write according to the result,
then run the tiers again. The station SHALL have a budget of 5 rounds, counted
apart from the fixing loop's rounds. `conveyor:keep-going` SHALL extend a capped
testing station by another 5 rounds, as it extends the fixing loop. A round in
which no model ran SHALL spend nothing. A round that changes nothing SHALL end the
station early.

#### Scenario: The budget is spent

- **WHEN** the fifth testing round ends with a tier still failing
- **THEN** the station ends `capped`, the issue label becomes `station:stalled` and the report names the failing tier

#### Scenario: Keep-going extends the budget

- **WHEN** a person places `conveyor:keep-going` on a capped testing station
- **THEN** another 5 rounds are allowed and the label is consumed

### Requirement: One comment on the tracking issue reports the coverage

The station SHALL keep exactly one coverage comment on the change's tracking
issue, found by a hidden marker and edited in place each round. It SHALL show, per
tier, the floor, the diff coverage, the covered and total changed lines, and the
tests' pass or fail. It SHALL show the round number against the budget and the
verdict. It SHALL name uncovered files and line numbers and SHALL NOT quote code.

#### Scenario: A second round edits the comment

- **WHEN** round 2 ends
- **THEN** the same comment is edited and the issue holds one coverage comment

#### Scenario: The report names no code

- **WHEN** a tier is under its floor
- **THEN** the comment lists file paths and line numbers and quotes no source line
