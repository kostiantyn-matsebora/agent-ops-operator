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
- **THEN** that station runs and the line stops there — EXCEPT the archive
  station, whose own label already promises to drive its resulting pull
  request to mergeable

#### Scenario: A merge with no standing instruction to carry

- **WHEN** an opsx-lane pull request merges and its issue carries no standing
  instruction
- **THEN** the issue's station moves from merge to stalled, and no archive
  session starts

#### Scenario: The archive station drives its own pull request

- **WHEN** a person places the archive label directly, with no standing
  instruction on the issue
- **THEN** the session it starts opens a pull request, and that pull request
  is driven to mergeable by the fixing loop the same as any other — the
  archive label is itself sufficient authorisation for its own pull
  request's fix station
