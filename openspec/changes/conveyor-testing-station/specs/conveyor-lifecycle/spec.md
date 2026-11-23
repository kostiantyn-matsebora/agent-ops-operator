## MODIFIED Requirements

### Requirement: The line's state is its labels, and nothing else

Which station a change has reached SHALL be readable from the labels on its
issue and its pull request. No separate store, file or naming convention SHALL
hold that state.

Two state vocabularies SHALL exist beside the grant labels, named in the same
vocabulary file, and SHALL grant nothing:

| On | Says | Values |
|---|---|---|
| the issue | which station the line is at | implement, testing, fix, merge, archive, done, stalled |
| the pull request | what the fixing loop is doing | running, waiting, stalled, capped, mergeable |

`waiting` SHALL mean a person's answer is owed. Every remaining item is a
dispute the fixing step made, or a conflict the line cannot resolve itself.

`stalled` SHALL mean the machine stopped. There was no report, the fixing step failed or
timed out, or the next round could not be started.

On the issue, `stalled` SHALL also name a testing station that spent its budget or
stopped. The testing budget of 5 rounds SHALL be counted from the issue and kept apart
from the fixing loop's rounds.

A stale patch is neither. Nothing landed and every item is still eligible, so
that round starts the next one itself.

Each SHALL be moved by the workflow performing the transition. At most one
value of each SHALL be present at a time. State labels are read by nobody but
people, so a person removing one SHALL change nothing about what the line
does next.

**Anything else is a second source of truth that drifts.** Labels are already
what the gates read, what a person sees without leaving the page, and what a
person can change to move or stop the line.

**One label for two facts was measured misleading on #259.** `loop:stalled`
beside a red pull request and a "clean" summary could not tell a reader
whether the loop had given up or was waiting on them.

#### Scenario: A person asks where a change is

- **WHEN** somebody reads the tracking issue and its pull request
- **THEN** the standing instruction, the station reached, and whether the loop
  is waiting on them or stopped on its own are all visible there

#### Scenario: A merge station label outlives its own condition

- **WHEN** a pull request merges and nothing carries the issue's line onward
- **THEN** the issue's label no longer reads `station:merge`, since that
  label's own meaning is now false

#### Scenario: The station label follows the line

- **WHEN** a station starts, when the testing station passes, when its pull
  request goes green, when a person merges, and when the archive lands
- **THEN** the issue's station label reads implement, testing, fix, merge, archive and
  done in turn — or stalled in place of archive, when the merge left nothing
  to carry the line onward — and never two at once

#### Scenario: The loop label follows the round

- **WHEN** a round starts, when a round ends with only disputes left, when a
  round ends with its fixer failed, when the round cap is reached, and when
  the head's checks are all green
- **THEN** the pull request's loop label reads running, waiting, stalled,
  capped and mergeable in turn, and never two at once

#### Scenario: A conflict the line cannot resolve

- **WHEN** the pull request conflicts with its base in a way the line cannot resolve
- **THEN** the pull request carries the waiting loop state and a comment names the conflicted files

## ADDED Requirements

### Requirement: A pull request is mergeable only when it can merge

The fixing loop SHALL end `mergeable` only when the head has no failed required
check, no open review thread, no unaddressed analysis issue, and a merge state
GitHub reports as clean. The testing station SHALL have passed on the current head.

A failed build SHALL be work like any failed check. A head behind its base SHALL
be brought up to date.

#### Scenario: A failed build

- **WHEN** a required build check fails on the head
- **THEN** the loop treats it as work and does not end `mergeable`

#### Scenario: Everything agrees

- **WHEN** no required check failed, no thread is open, no analysis issue is unaddressed, GitHub reports a clean merge state and the testing station passed on this head
- **THEN** the loop ends `mergeable`

#### Scenario: A fixer pushes after the tests passed

- **WHEN** a fixing round pushes new commits to the head after the testing station passed
- **THEN** the testing station runs again on the new head, and the loop does not end `mergeable` until it has passed

#### Scenario: One condition disagrees

- **WHEN** the merge state is clean but a thread is open, or the testing station has not passed on this head
- **THEN** the loop does not end `mergeable`

#### Scenario: Behind its base

- **WHEN** the head is behind its base and merges cleanly
- **THEN** the line merges the base into the branch, and the tests run again before `mergeable`

### Requirement: A trivial conflict is resolved by the line and a non-trivial one is asked

A conflict SHALL be trivial when the base merges into the branch with no
conflict marker, or when every conflicted file is one a repository script
regenerates.

The line SHALL resolve a trivial conflict and push. Any other conflict SHALL stop
the loop in the waiting state, with a pull request comment naming the conflicted
files. A person's comment SHALL resume it.

#### Scenario: Only a generated file conflicts

- **WHEN** the only conflicted file is a generated one
- **THEN** the line regenerates it, pushes, and the tests run again

#### Scenario: A source file conflicts

- **WHEN** a hand-written file has conflict markers after the base is merged
- **THEN** the loop is waiting, the comment names the file, and nothing is pushed
