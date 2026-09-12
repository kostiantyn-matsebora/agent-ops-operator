# conveyor-lifecycle Specification

## Purpose
The labelled stations a change passes through unattended: who may place each
label, what a program may do with one, where the line advances itself, and where
it stops for a person.

## Requirements

### Requirement: A grant is a label a person places, and a program may only carry or consume it

Every label that authorises unattended work SHALL be placed by a person whose
write access is read from the platform at the moment it is acted on. A program
MAY place such a label when it is CARRYING a person's existing grant forward,
and SHALL record whose grant it carried. A program SHALL NOT place a label that
grants anything on its own behalf, and an automated session SHALL NOT place one
at all.

**A machine cannot approve its own work, and the mechanism must make that
impossible rather than discourage it.** A session opened its pull request
carrying the approve label because its instructions said to; it acts as an
application with no write access, so the gate that checks who labelled removed
the label and refused. The gate was correct. What the label means — a person
decided this — cannot be asserted by the thing being decided about.

A carried label SHALL be re-checked where it is acted on, against the grant it
claims to carry, rather than trusted because a workflow placed it.

#### Scenario: A session labels its own pull request

- **WHEN** an automated session places a label authorising work on the pull
  request it just opened
- **THEN** the label is removed, the refusal is visible on the pull request, and
  no work is authorised

#### Scenario: A workflow carries a person's grant

- **WHEN** a person's standing instruction authorises the next station and a
  workflow places that station's label
- **THEN** the label is honoured, and the record says whose instruction it came
  from

#### Scenario: A carried label whose grant is gone

- **WHEN** a station's label is present but the issue it came from no longer
  carries the standing instruction
- **THEN** the work is refused, exactly as a label placed by somebody without
  write access is

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
- **THEN** that station runs and the line stops there

### Requirement: The issue selects the lane, and nothing infers it

An issue already bound to an openspec change SHALL be worked through that
project's change workflow, ending with the change archived. Every other issue
SHALL be worked directly from what the issue describes, ending at the merge of
its pull request. Which lane applies SHALL be determined by reading recorded
state — the binding a promoted issue carries, or its phase label — and SHALL NOT
be inferred from the issue's wording, its size, or any judgement about whether
it deserves a proposal.

**A well-described issue is already the agreement.** A proposal exists so a
person reads the shape of a change before its code; an issue somebody wrote and
a maintainer approved has been read by two people. Requiring a proposal anyway
produces a document that restates the issue and delta specs that say nothing —
measured, on a one-line fix that arrived as a 165-line change directory.

**And what selects the lane must be a fact, not a reading.** What decides that
code is written to a branch is not a judgement call anywhere else in this
project, and it is not one here.

The plain lane SHALL owe no proposal, no task list and no specification delta,
and SHALL owe the same standard of mergeable as any other work: the checks
green, the guards clean, the review answered.

#### Scenario: An issue bound to a change

- **WHEN** the line runs for an issue that already tracks an openspec change
- **THEN** the change's own workflow is followed, and the last station archives
  it

#### Scenario: An ordinary issue

- **WHEN** the line runs for an issue that tracks no change
- **THEN** the work is implemented from what the issue describes, its pull
  request is driven to mergeable, and the line ends at the merge with nothing
  archived

#### Scenario: A lane's line ends

- **WHEN** the last station of an issue's lane completes
- **THEN** no label is placed for a station that lane does not have, and the
  line is over

#### Scenario: Two issues that read alike

- **WHEN** two issues describe similar work and only one tracks a change
- **THEN** they run different lanes, decided by that binding and by nothing
  about how they are worded

### Requirement: An unattended loop is bounded, and extended by one consumed label

The number of rounds an unattended fixing loop may run without a person SHALL be
a named constant with a stated default. Reaching it SHALL end the loop with a
summary saying so and naming what remains. A stated label SHALL grant another
set of rounds and SHALL be REMOVED when it takes effect.

**A bound is a ceiling on unattended SPEND, not on progress.** A loop that finds
new things each round is working; one that cannot land the same fix is not, and
neither is distinguishable in advance. So the bound is generous, visible when
reached, and lifted by a person who can see what the last rounds produced.

**Consumed, because a permanent extension is not a decision.** One placement
grants one set; placing it again is a fresh judgement made with newer evidence.

#### Scenario: The loop reaches its bound

- **WHEN** the configured number of rounds has run and work remains
- **THEN** the loop stops, and one summary names the rounds used, what was
  fixed, what is disputed, and how to grant more

#### Scenario: A person grants another set of rounds

- **WHEN** the extending label is placed on a bounded pull request
- **THEN** the loop runs again for another set, and the label is removed as it
  is taken

#### Scenario: The extending label is placed twice

- **WHEN** a person places the extending label after a previous set was
  exhausted
- **THEN** another set is granted, each placement having granted exactly one

### Requirement: The line's state is its labels, and nothing else

Which station a change has reached SHALL be readable from the labels on its
issue and its pull request. No separate store, file or naming convention SHALL
hold that state.

**Anything else is a second source of truth that drifts.** Labels are already
what the gates read, what a person sees without leaving the page, and what a
person can change to move or stop the line.

#### Scenario: A person asks where a change is

- **WHEN** somebody reads the tracking issue and its pull request
- **THEN** the standing instruction, the station reached, and whether the loop
  stalled are all visible there
