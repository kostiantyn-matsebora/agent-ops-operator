## MODIFIED Requirements

### Requirement: One standing instruction carries a change through every station

A single label on the tracking issue SHALL mean: implement this, drive its pull
request to mergeable, and archive it once merged. It SHALL be read at EVERY
transition rather than recorded at the first, and removing it SHALL stop the
line at the next station while leaving work already running to finish.

A station's label placed by a workflow carrying that instruction SHALL start
the station by itself:

- the mechanism running the station SHALL accept a start the repository's own
  workflow relayed, after re-checking the grant it relayed
- it SHALL accept it exactly as it accepts a person's own label
- a carried label that starts nothing is a line stopped by nobody, and it is
  what #220 measured

A station that cannot run its work SHALL say so where the line is read. No
round SHALL end silently:

- one summary on the pull request names the failure, the run and the approver
- the pull request's loop state says stalled

The archive station SHALL run unattended under the standing instruction, on
the lane that has one:

1. A session archives the change on its branch.
2. It opens the archive pull request closing the tracking issue.
3. That pull request is driven to mergeable under the same instruction.
4. A person merges it.

**The decision is made once, at the moment there is something to decide about.**
Asking again when the pull request opens, and again when it merges, asks the
same person the same question about work they already approved — while the
moments they actually decide something are the merges.

#### Scenario: The line runs end to end

- **WHEN** a person places the standing instruction on an issue
- **THEN** a change is implemented, its pull request is opened and driven to
  mergeable, and after a person merges it the change is archived — with no
  further label placed BY A PERSON, each station's label carried by a workflow
  on the strength of that one instruction

#### Scenario: A carried label starts its station

- **WHEN** a workflow carries the standing instruction forward as the fixing
  station's label
- **THEN** a fixing round starts and runs its fixer without any further event
  from a person, and the gate that accepts it has re-checked the instruction
  it carried

#### Scenario: A round cannot run

- **WHEN** a fixing round's fixer fails before or while doing its work
- **THEN** one summary on the pull request names the failure, links the run
  and mentions the approver, the pull request's loop state says stalled, and
  nothing about the round is reported only in a log

#### Scenario: The archive station runs

- **WHEN** a merged pull request's tracking issue still carries the standing
  instruction and its change is bound to an openspec change
- **THEN** a session archives the change on its branch, opens the archive
  pull request closing the tracking issue with no label placed by the session,
  and that pull request is driven to mergeable under the same instruction

#### Scenario: The instruction is removed mid-flight

- **WHEN** the standing instruction is removed while a station is running
- **THEN** that station completes and no further station begins

#### Scenario: A single station is driven by hand

- **WHEN** a person places one station's label instead of the standing
  instruction
- **THEN** that station runs and the line stops there

### Requirement: The line's state is its labels, and nothing else

Which station a change has reached SHALL be readable from the labels on its
issue and its pull request. No separate store, file or naming convention SHALL
hold that state.

Two state vocabularies SHALL exist beside the grant labels, named in the same
vocabulary file, and SHALL grant nothing:

| On | Says | Values |
|---|---|---|
| the issue | which station the line is at | implement, fix, merge, archive, done |
| the pull request | what the fixing loop is doing | running, stalled, capped, mergeable |

Each SHALL be moved by the workflow performing the transition. At most one
value of each SHALL be present at a time. State labels are read by nobody but
people, so a person removing one SHALL change nothing about what the line
does next.

**Anything else is a second source of truth that drifts.** Labels are already
what the gates read, what a person sees without leaving the page, and what a
person can change to move or stop the line.

#### Scenario: A person asks where a change is

- **WHEN** somebody reads the tracking issue and its pull request
- **THEN** the standing instruction, the station reached, and whether the loop
  stalled are all visible there

#### Scenario: The station label follows the line

- **WHEN** a station starts, when its pull request goes green, when a person
  merges, and when the archive lands
- **THEN** the issue's station label reads implement, fix, merge, archive and
  done in turn, and never two at once

#### Scenario: The loop label follows the round

- **WHEN** a round starts, when a round ends with every item disputed or with
  its fixer failed, when the round cap is reached, and when the head's checks
  are all green
- **THEN** the pull request's loop label reads running, stalled, capped and
  mergeable in turn, and never two at once

## ADDED Requirements

### Requirement: A dispute only a person can settle is settled by that person's ordinary action

Some items no edit to the tree can fix: a required check whose verdict is the
state of the review's own threads. Where the loop disputes one, the line SHALL
treat the person's ordinary answer as the settlement.

Resolving or unresolving a review thread SHALL re-evaluate the check that
reads those threads, on the same head, with no hand re-run.

**A dispute that cannot be answered is a dead end, not a decision owed.** On
#220 the loop disputed `review-clean` correctly. The only way to green was a
person re-running two workflows from a terminal.

#### Scenario: A person dismisses a finding

- **WHEN** a person resolves a review-authored thread on a pull request whose
  head's checks reported the review as not clean
- **THEN** the check re-evaluates against the live threads and the head's
  required checks report green, with no push and no hand re-run

#### Scenario: A person reopens a finding

- **WHEN** a person unresolves a review-authored thread on a green head
- **THEN** the check re-evaluates and the head's required checks report red
