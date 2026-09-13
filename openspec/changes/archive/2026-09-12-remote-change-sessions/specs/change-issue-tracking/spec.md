## MODIFIED Requirements

### Requirement: An inbound issue is promoted in place

When an issue somebody else filed becomes an openspec change, that issue SHALL
become the change's tracking issue. It SHALL NOT be closed in favour of a new
issue authored by the project.

The change SHALL be seeded from what the reporter actually wrote.

**The reporter is waiting in that thread.** Closing it discards both their words
and every reply attached to them, and answers a person who asked a question by
pointing them at a different page. A promoted issue keeps the conversation and
gains the phase label; nothing is duplicated and nothing is lost.

A remote session started by a label on an issue SHALL promote that issue, in
the same way and with the same script. The start of such a session adds
exactly two automated comments: the one recording the start, and the pointer
the promotion leaves as it does for any promoted issue.

**THE TRACKING ISSUE CARRIES THE STANDING INSTRUCTION**, and its labels SHALL be
read at every transition of the change rather than at the first. A workflow
deciding whether to advance the change to its next station SHALL consult the
issue's labels as they stand at that moment.

#### Scenario: A filed issue becomes a change

- **WHEN** an issue reported by somebody else is turned into an openspec change
- **THEN** that same issue tracks the change, keeps its original text and
  comments, and gains the change's link and phase label

#### Scenario: A remote session is started from a filed issue

- **WHEN** a label starts a remote session for an issue
- **THEN** that session promotes the same issue as the change's tracking issue,
  the start's comment and the promotion's pointer are the only automated
  comments the start adds, and the reporter's body is untouched

#### Scenario: The change reaches its next station

- **WHEN** a workflow must decide whether to advance the change unattended
- **THEN** it reads the tracking issue's labels at that moment, and advances
  only while the standing instruction is there

#### Scenario: The standing instruction is removed

- **WHEN** the standing instruction is taken off the tracking issue
- **THEN** the change advances no further, and work already running finishes

#### Scenario: The change advances

- **WHEN** a change promoted from an inbound issue moves to another phase
- **THEN** the reporter's body is untouched, and the pointer the promotion left
  as a comment is what carries the refreshed links and phase

#### Scenario: The change is archived

- **WHEN** a change promoted from an inbound issue is archived
- **THEN** the issue closes with the reporter's original thread intact
