# change-issue-tracking Specification

## Purpose
The GitHub issue that tracks an openspec change through its life — what it is
allowed to contain, how it is bound to the change, how its state advances, and
how an issue somebody else filed becomes a change without losing their thread.

An issue here is a WINDOW onto work, not a second place the work is described.
The change directory is the source of truth, and anything the issue restates is
a second thing to keep true.

## Requirements

### Requirement: A change has exactly one tracking issue

Every openspec change SHALL have exactly one GitHub issue tracking it, opened
when the change is proposed and closed when the change is archived.

**One issue, not a tree of them.** Changes here carry between eight and dozens
of tasks, and a child issue per task group would mirror a task list that is
already the checklist — in a place where it cannot be ticked, and which must
then be kept in step with the file that can. Progress within a change is
reported by advancing the one issue, not by creating more of them.

#### Scenario: A change is proposed

- **WHEN** a change's planning artifacts are created
- **THEN** exactly one issue exists for it, linking the change

#### Scenario: A change is archived

- **WHEN** a change is archived
- **THEN** its issue is closed, and no other issue was opened for it at any
  point in its life

### Requirement: The issue is a pointer and a status, never a copy

A tracking issue SHALL contain only what identifies the change, where to read
it, where its pull request is, and what phase it is in. It SHALL NOT restate the
change's rationale, design, specifications or task list.

Its body SHALL be generated from the change rather than written by hand, so that
it cannot drift from what it points at.

**A copied proposal is a second source of truth**, and this project refuses that
shape everywhere else it appears. The failure is not that the copy is wrong on
the day it is made — it is that nothing ever tells a reader which of the two
they are reading.

**Sections SHALL be few and fixed**, so that a reader learns the shape once and
can then scan any tracking issue without reading it.

#### Scenario: The change's proposal is revised

- **WHEN** a change's proposal is edited after its issue exists
- **THEN** the issue needs no edit to remain true, because it restated nothing
  that changed

#### Scenario: A reader opens a tracking issue

- **WHEN** somebody opens the issue to find out what the change is
- **THEN** they are pointed at the change directory and the pull request, and
  the issue itself claims nothing the change does not

### Requirement: The binding between change and issue is stored as a reference

The issue's number SHALL be recorded inside the change's own directory, and
nothing else about the issue SHALL be recorded there.

The binding SHALL survive archiving, so that an archived change can still be
traced to the discussion that accompanied it.

**A reference is stored; content is not.** Storing the number means a lookup is
always current. Storing anything the issue says means storing something that can
go stale with no mechanism to notice.

#### Scenario: The issue is renamed or relabelled

- **WHEN** the issue's title, labels or body change
- **THEN** the stored binding is still correct and nothing in the change
  directory needs updating

#### Scenario: An archived change is traced

- **WHEN** somebody reads an archived change
- **THEN** the issue that tracked it is identifiable from the change itself

### Requirement: The issue's phase reflects the change's phase

The issue SHALL carry a label naming the phase the change is in, and that label
SHALL advance as the change moves: proposed, being implemented, in review, and
archived.

Each transition SHALL be recorded on the issue once, in a form that says what
changed and where to look — not by restating progress the task list already
holds.

**Tracking must be readable at a glance or it is not tracking.** A stream of
automated progress comments makes the issue longer than the artifact it points
at, which is the failure this requirement exists to prevent.

#### Scenario: Implementation begins

- **WHEN** work on a change starts
- **THEN** its issue's phase label advances, and the transition is recorded once

#### Scenario: Somebody scans the issue list

- **WHEN** the open issues are listed
- **THEN** each change's phase is visible from the list without opening it

### Requirement: An inbound issue is promoted in place

When an issue somebody else filed becomes an openspec change, that issue SHALL
become the change's tracking issue. It SHALL NOT be closed in favour of a new
issue authored by the project.

The change SHALL be seeded from what the reporter actually wrote.

**The reporter is waiting in that thread.** Closing it discards both their words
and every reply attached to them, and answers a person who asked a question by
pointing them at a different page. A promoted issue keeps the conversation and
gains the phase label; nothing is duplicated and nothing is lost.

#### Scenario: A filed issue becomes a change

- **WHEN** an issue reported by somebody else is turned into an openspec change
- **THEN** that same issue tracks the change, keeps its original text and
  comments, and gains the change's link and phase label

#### Scenario: The change advances

- **WHEN** a change promoted from an inbound issue moves to another phase
- **THEN** the reporter's body is untouched, and the pointer the promotion left
  as a comment is what carries the refreshed links and phase

#### Scenario: The change is archived

- **WHEN** a change promoted from an inbound issue is archived
- **THEN** the issue closes with the reporter's original thread intact
