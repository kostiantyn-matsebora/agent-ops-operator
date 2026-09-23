## MODIFIED Requirements

### Requirement: One standing instruction carries a change through every station

A single label on the tracking issue SHALL mean: implement this, drive its pull
request to mergeable, and archive it once merged. It SHALL be read at EVERY
transition rather than recorded at the first, and removing it SHALL stop the
line at the next station while leaving work already running to finish.

**The decision is made once, at the moment there is something to decide about.**
Asking again when the pull request opens, and again when it merges, asks the
same person the same question about work they already approved — while the
moments they actually decide something are the merges.

**A merge with nothing to carry it onward SHALL mark the line stalled.** The
merge station's job ends the moment the pull request merges, so an opsx-lane
issue with no standing instruction SHALL move off that label at once.

#### Scenario: The line runs end to end

- **WHEN** a person places the standing instruction on an issue
- **THEN** a change is implemented, its pull request is opened and driven to
  mergeable, and after a person merges it the change is archived — with no
  further label placed BY A PERSON, each station's label carried by a workflow
  on the strength of that one instruction

#### Scenario: The instruction is removed mid-flight

- **WHEN** the standing instruction is removed while a station is running
- **THEN** that station completes and no further station begins

#### Scenario: A single station is driven by hand

- **WHEN** a person places one station's label instead of the standing
  instruction
- **THEN** that station runs and the line stops there

#### Scenario: A merge with no standing instruction to carry

- **WHEN** an opsx-lane pull request merges and its issue carries no standing
  instruction
- **THEN** the issue's station moves from merge to stalled, and no archive
  session starts

### Requirement: The line's state is its labels, and nothing else

Which station a change has reached SHALL be readable from the labels on its
issue and its pull request. No separate store, file or naming convention SHALL
hold that state.

**A station's label SHALL remain true to its own stated meaning for as long
as it is shown.** A label whose condition has already resolved SHALL be
replaced at the next observable event, never left describing the past as the
present.

**Anything else is a second source of truth that drifts.** Labels are already
what the gates read, what a person sees without leaving the page, and what a
person can change to move or stop the line.

| Label | Its own stated meaning | What makes it false |
|---|---|---|
| `station:merge` | the pull request is mergeable and waits for a person | the pull request merges |
| `station:stalled` | a merge landed but nothing carries the line onward | a person places the standing instruction, or the archive label, to resume it |

Measured live: an issue whose pull request merged two days earlier still read
`station:merge`, indistinguishable from one still waiting on a person.

#### Scenario: A person asks where a change is

- **WHEN** somebody reads the tracking issue and its pull request
- **THEN** the standing instruction, the station reached, and whether the loop
  stalled are all visible there

#### Scenario: A merge station label outlives its own condition

- **WHEN** a pull request merges and nothing carries the issue's line onward
- **THEN** the issue's label no longer reads `station:merge`, since that
  label's own meaning is now false
