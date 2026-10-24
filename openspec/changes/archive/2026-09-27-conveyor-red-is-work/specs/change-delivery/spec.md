## MODIFIED Requirements

### Requirement: A change is not archived while its fixing loop is open

A change SHALL NOT be archived while a fixing round is running on its pull
request, or while a dispute posted by the loop has no reply from a person and
its thread is unresolved.

The refusal SHALL be the archive command's alone. No required check SHALL
carry it, and no check SHALL turn red for either condition.

Archiving folds the deltas into the published specs. Doing so under a loop
that may still land a commit, or over a disagreement nobody has ruled on,
records the change as finished while the pull request still cannot merge.

A check carrying the same question made the loop stop on its own refusal.
Measured on #259: the check turned red for a dispute, the red was not the
loop's to fix, and the pull request sat blocked with nobody told.

#### Scenario: Archive is attempted mid-loop

- **WHEN** the archive is requested while a round is running
- **THEN** it is refused, naming the running round

#### Scenario: Archive is attempted over an unanswered dispute

- **WHEN** the archive is requested while a disputed thread has no reply from
  a person and is unresolved
- **THEN** it is refused, naming the thread

#### Scenario: The loop has ended and every dispute is answered

- **WHEN** the summary has posted and every disputed thread carries a person's
  reply or is resolved
- **THEN** the archive proceeds as before

#### Scenario: CI evaluates a head with a dispute unanswered

- **WHEN** a dispute the loop posted has no answer and CI runs on the head
- **THEN** no required check is red for it
