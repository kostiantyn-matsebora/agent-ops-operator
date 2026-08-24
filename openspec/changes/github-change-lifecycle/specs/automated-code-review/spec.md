## Purpose

The automated review a pull request receives on every push: what it looks for,
what it must not say twice, which of its own remarks it closes when they are
addressed, and the privilege boundary that lets it close anything at all.

A review that restates its previous findings on each push is worse than no
review, because the signal a reader needs — *what is new since I last looked* —
is buried under everything already dealt with.

## ADDED Requirements

### Requirement: Every push to a pull request is reviewed

A pull request SHALL be reviewed when it opens, on every subsequent push, and
when it leaves draft. Findings SHALL be delivered as comments on the specific
lines they concern, alongside one summary.

A superseded review SHALL be abandoned rather than raced, so that a burst of
pushes produces one review of the latest state rather than several of stale
ones.

#### Scenario: A pull request receives a second push

- **WHEN** a contributor pushes again to an open pull request
- **THEN** the review runs again against the updated state

#### Scenario: Several pushes arrive together

- **WHEN** pushes arrive faster than a review completes
- **THEN** only the newest is reviewed, and the superseded runs produce no
  comments

### Requirement: A finding already made is not made again

The review SHALL receive its previous findings as context, and SHALL NOT post a
remark it has already made and which still stands.

**Continuity here is a matter of context, not of stored state.** The previous
review's remarks live on the pull request and are handed to the next run, so no
separate record of findings is kept anywhere — no sidecar file, no database, no
labels standing in for review state. A second store would be a second thing to
keep correct, and it would be wrong precisely when a run failed part-way.

#### Scenario: An unaddressed finding survives a push

- **WHEN** a push does not address a finding from the previous review
- **THEN** the existing remark is left as it is, and no duplicate is posted

#### Scenario: A new problem is introduced

- **WHEN** a push introduces a problem not previously remarked on
- **THEN** a new remark is posted for it

### Requirement: A finding that has been addressed is closed

When a push resolves a finding, the review SHALL reply saying so and mark that
remark's thread resolved. The same SHALL apply when the code a finding concerned
no longer exists.

#### Scenario: A finding is fixed

- **WHEN** a push fixes the problem a remark described
- **THEN** the remark's thread is answered and marked resolved

#### Scenario: The code is deleted

- **WHEN** a push removes the code a remark concerned
- **THEN** the remark's thread is answered as no longer applicable and marked
  resolved

### Requirement: The review never closes a remark it did not make

Resolution SHALL be restricted to threads the review itself authored, and that
restriction SHALL be enforced mechanically rather than by instruction.

**This is the one failure here that destroys information.** Every other mistake
adds noise a reader can ignore; resolving a human reviewer's thread hides a
person's objection and reports it as handled. An instruction can be
misinterpreted, so the constraint belongs where it cannot be.

#### Scenario: A human leaves a review remark

- **WHEN** a person comments on a line and the review runs afterwards
- **THEN** that thread is never resolved by the review, whatever the code now
  says

### Requirement: A stale anchor is not treated as a fix

When a remark's thread has been detached from the code by an unrelated edit,
the review SHALL re-check whether the finding still holds. If it does, the
finding SHALL be raised again against its current location.

A detached thread SHALL NOT be resolved on the strength of being detached.

**Detachment means the remark has become invisible, not that it has been
addressed.** A reformat detaches a live finding, and a fix elsewhere leaves a
dead one attached — so the anchor's state says nothing about the code, while
looking exactly like it does.

#### Scenario: An unrelated edit moves the line

- **WHEN** a push edits the region around a finding without addressing it
- **THEN** the finding is raised against its new location and the detached
  thread is closed as superseded

### Requirement: A finding dismissed by a person stays dismissed, and is reported

When a person resolves one of the review's threads, the review SHALL NOT raise
that finding again.

The summary SHALL state how many findings were dismissed this way.

**Deferring to the person is right; doing it silently is not.** A finding that
was resolved rather than fixed has left the reader's view, and a count is what
keeps the gap visible instead of making the summary read as though everything
raised had been dealt with.

#### Scenario: A maintainer resolves a finding without changing the code

- **WHEN** a person marks one of the review's threads resolved and pushes
  nothing
- **THEN** the review does not raise it again, and the summary reports it as
  dismissed

### Requirement: What may close a thread holds no power to change the code

The step that resolves threads SHALL be separated from the step that produces
the review, and SHALL run without any model deciding what it does. Only the
separated step SHALL hold the privilege that resolution requires.

**Resolving a thread requires write access to the repository's contents**, not
merely to its pull requests. Granting that to the reviewing step would give a
process driven by generated output the ability to push — so the privilege is
held by a step that follows instructions it cannot rewrite, acting on a list it
is handed.

This is the project's existing rule applied one layer out: *the component
running untrusted model output must not out-rank the one orchestrating it.*

#### Scenario: The review runs

- **WHEN** the reviewing step executes
- **THEN** it can read the code and comment on the pull request, and it cannot
  write to the repository

#### Scenario: Threads are resolved

- **WHEN** findings are closed
- **THEN** the closing is performed by a step whose behaviour is fixed, acting
  only on the threads it was given

### Requirement: The review is measured against the project's own rules

The review SHALL evaluate a change against the project's recorded invariants,
retired vocabulary and the change's own specifications, in addition to ordinary
correctness concerns.

**This is the reading nothing else performs.** Several rules in this repository
record decisions that were re-made, reverted, and re-made again — a class of
regression that compiles, passes every test, renders every chart, and is only
visible to a reader who knows what was decided before.

#### Scenario: A change contradicts a recorded invariant

- **WHEN** a diff reintroduces behaviour the project recorded as removed, or
  contradicts a stated invariant
- **THEN** the review raises it, naming the rule it contradicts

#### Scenario: A change contradicts its own specification

- **WHEN** a diff implements something other than what the change's specs
  require
- **THEN** the review raises the discrepancy
