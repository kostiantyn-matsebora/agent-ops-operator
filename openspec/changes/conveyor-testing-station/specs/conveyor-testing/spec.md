## Purpose

The testing station of the conveyor. It proves a change's new code is tested at
each tier, loops until the tests pass and the floors hold, and reports the
numbers on the change's tracking issue.

## ADDED Requirements

### Requirement: A session runs the tests, and CI is not the runner

The testing station SHALL be one session started for the change's issue. It SHALL run
the unit, system and e2e tiers itself, in its own environment, and SHALL measure their
coverage there.

CI MAY run the same suites for its own gate. The station SHALL NOT read its numbers from
CI's runs.

#### Scenario: The session measures its own run

- **WHEN** the station runs a tier
- **THEN** the coverage it judges comes from that run in the session's environment

#### Scenario: An environment that cannot run a tier

- **WHEN** the session's environment cannot run a tier the change needs
- **THEN** the station stops with the reason named in the report, and no tier reads as passed

### Requirement: Behaviour that depends on the model is tested against a real model

The session SHALL judge, for the change, whether the behaviour of the real model affects
the functionality it adds or changes. The judgement SHALL be recorded in the report with
its reason.

Where it does, the system and e2e tiers SHALL run on a real runtime, with the credential
read from an environment variable only.

The vocabulary names that variable (`testing.credential_env`), and the vendor's own name
is accepted beside it, because a cloud session strips the vendor's name. A run on the
stub runtime SHALL NOT count as passing for such a change.

A path list SHALL name the code that is agent-facing by default, as a floor the judgement
may extend and SHALL NOT shrink. The list is `testing.agent_facing_paths` in
`.github/review-triage.json`.

Assertions on a real model SHALL tolerate its nondeterminism and SHALL NOT assert exact
wording. A failing real-model test SHALL be re-run once, inside its round, before it is
work. A red outside that rule follows the re-run rule of the failing-test requirement.

The session SHALL NOT read, print or store the credential. When it is required and
absent, the station SHALL stop and say so.

#### Scenario: A change to the dispatch prompt

- **WHEN** the diff changes a prompt the runtime sends to the model
- **THEN** the report records real-model testing as required, and the system and e2e tiers run on a real runtime

#### Scenario: A change no model output reaches

- **WHEN** the diff touches only a chart template's labels
- **THEN** the report records real-model testing as not required, with the reason, and the stub runtime suffices

#### Scenario: The credential is absent

- **WHEN** real-model testing is required and the credential variable is unset
- **THEN** the station stops and no tier reads as passed

### Requirement: The testing station sits between implement and the fixing loop

When the implement session opens the change's pull request, the issue's station label
SHALL read `station:testing`. The station SHALL end in one of three ways. Passing moves
the line to the fixing loop. Exhausting the budget moves it to `stalled`.

A stop the report names, because the environment cannot run a tier or the credential is
absent, leaves it at `stalled` as well, until a person acts.

#### Scenario: Implement hands over to testing

- **WHEN** the implement session opens the pull request
- **THEN** the issue carries `station:testing` and no other station label, and the testing session starts

#### Scenario: Passing moves the line on

- **WHEN** every tier passes and holds its floor on the current head
- **THEN** the station label becomes `station:fix` and the fixing loop starts

### Requirement: Coverage is judged on the lines the change added

Each tier SHALL be measured over the executable lines the pull request adds or changes,
never over the repository. A tier SHALL apply only to the code it can exercise.

A tier with no executable changed line in its domain SHALL be reported `not applicable`
and SHALL hold no floor.

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
instrumentation, and SHALL collect its coverage data when the run ends. The e2e tier
SHALL collect browser coverage of the console UI source.

A tier that ran but yielded no coverage data SHALL fail, never read as zero changed
lines.

#### Scenario: Missing coverage data fails the tier

- **WHEN** the system pack ran and wrote no coverage data
- **THEN** the system tier reports a collection failure and the station does not pass

### Requirement: Missing tests are written by a model that did not write the code

A shortfall of coverage SHALL be answered by a separate agent, a different model from
the one that implemented the change, running with its own context. That agent SHALL read
the change's specs, its design and the public interfaces.

It SHALL NOT read the implementing session's transcript. It SHALL write only test files,
and a patch touching any other path SHALL be refused.

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

Each round SHALL run the three tiers, then fix or write according to the result, then
run the tiers again. The station SHALL have a budget of 5 rounds, counted apart from the
fixing loop's rounds.

`conveyor:keep-going` SHALL extend a capped testing station by another 5 rounds, as it
extends the fixing loop, and one label SHALL extend only the budget that is spent: the
testing station's when it is stalled, the fixing loop's otherwise.

A round in which no model ran SHALL spend nothing. A round that changes nothing SHALL
end the station early.

#### Scenario: The budget is spent

- **WHEN** the fifth testing round ends with a tier still failing
- **THEN** the station ends with its budget spent, the issue label becomes `station:stalled` and the report names the failing tier

#### Scenario: A round in which no model ran

- **WHEN** a round ends without a model having run
- **THEN** the round is not counted against the budget

#### Scenario: A round that changes nothing

- **WHEN** a round ends with the tree unchanged and a tier still failing
- **THEN** the station ends early, because the next round would read the same tree

#### Scenario: Keep-going extends the budget

- **WHEN** a person places `conveyor:keep-going` on a capped testing station
- **THEN** another 5 rounds are allowed and the label is consumed

### Requirement: One comment on the tracking issue reports the coverage

The station SHALL keep exactly one coverage comment on the change's tracking issue,
found by a hidden marker and edited in place each round.

It SHALL show, per tier, the floor, the diff coverage, the covered and total changed
lines, and the tests' pass or fail. It SHALL show the round number against the budget
and the verdict.

It SHALL name uncovered files and line numbers and SHALL NOT quote code.

#### Scenario: A second round edits the comment

- **WHEN** round 2 ends
- **THEN** the same comment is edited and the issue holds one coverage comment

#### Scenario: The report names no code

- **WHEN** a tier is under its floor
- **THEN** the comment lists file paths and line numbers and quotes no source line
