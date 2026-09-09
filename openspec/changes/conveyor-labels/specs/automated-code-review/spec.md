## MODIFIED Requirements

### Requirement: A landed fix on an approved pull request starts the next round

When a dispatch lands on a labelled pull request, the review and the
continuous-integration checks SHALL run on the landed commit without a person
pushing, and their findings SHALL start the next round. A round SHALL start when
the review completes on a labelled pull request AND when the
continuous-integration run completes with a failure on one; two starts for one
head SHALL run in sequence, each over the pull request's current state, and both
SHALL count toward the loop's bound.

**THE BOUND IS A NAMED CONSTANT WITH A STATED DEFAULT OF FIVE**, not a number
buried in a workflow. Reaching it SHALL end the loop with one summary naming the
rounds used, what was fixed, what is disputed and how to grant more. A stated
label SHALL grant another set of rounds and SHALL be removed as it is taken, so
that continuing is always a fresh decision made knowing what the last rounds
produced.

A push made with the workflow's own token starts nothing, so the landed commit
of an unlabelled dispatch has no checks and no review until somebody pushes
again — a limitation this project documented as the safe side. The loop makes
the next round automatic, and it does so with a credential held ONLY by the
model-free landing step.

#### Scenario: A fix lands on a labelled pull request

- **WHEN** the landing step pushes a fix
- **THEN** the review and the required checks run on that commit, and the pull
  request's merge gate sees them on its head

#### Scenario: The loop reaches its bound

- **WHEN** the configured number of rounds has run and work remains
- **THEN** the loop ends with one summary naming the rounds used and the label
  that grants another set

#### Scenario: Another set of rounds is granted

- **WHEN** a person places the extending label on a bounded pull request
- **THEN** the loop runs another set, and the label is removed as it is taken

#### Scenario: The checks fail on a labelled pull request

- **WHEN** the continuous-integration run on a labelled pull request's head
  completes with a failure
- **THEN** a round starts with the failed checks on its work list, whether or
  not the review posted anything

#### Scenario: A fix lands on an unlabelled pull request

- **WHEN** the landing step pushes a fix under a per-thread dispatch
- **THEN** behaviour is unchanged: the landing comment says a further push is
  needed

### Requirement: A dispatch is authorised by who sent it

A dispatch SHALL act only for a person the platform says may push to this
repository, read at the moment it is acted on rather than claimed in the text of
a comment. A label authorising unattended fixing SHALL be honoured only from
such a person, or from a workflow CARRYING such a person's standing instruction
— which SHALL be re-checked against that instruction rather than trusted. A
label placed by anyone else SHALL be removed with a visible comment naming who
may place it.

**A refusal that leaves the label in place reads as a loop that broke**, and a
bot exempted from the check is the hole the check exists to close: an automated
session placing its own approve label is a machine approving its own work.

#### Scenario: A person without write access labels a pull request

- **WHEN** the approve label is placed by somebody who cannot push here
- **THEN** no round starts, the label is removed, and a comment says who may
  place it

#### Scenario: An automated session labels its own pull request

- **WHEN** the approve label is placed by the session that opened the pull
  request
- **THEN** it is refused exactly as any other actor without write access is

#### Scenario: A workflow carries a standing instruction

- **WHEN** the approve label was placed by a workflow carrying a person's
  standing instruction, and that instruction still stands
- **THEN** the round proceeds, and the summary names the person it came from

#### Scenario: A dispatch arrives from someone without write access

- **WHEN** a dispatch comment is sent by somebody who cannot push here
- **THEN** it is refused, visibly, and nothing is written

#### Scenario: A dispatch arrives on a fork's pull request

- **WHEN** a dispatch names a pull request from a fork
- **THEN** it is refused: the branch is not in this repository, and there is
  nothing here to push to
