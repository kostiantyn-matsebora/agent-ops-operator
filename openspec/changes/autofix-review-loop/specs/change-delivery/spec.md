## ADDED Requirements

### Requirement: A change is approved for automatic fixing by its owner, once

The owner of a change SHALL approve its pull request for automatic fixing
ONCE, by a stated label on the pull request, and that label SHALL be placed by
the session working the change on the owner's explicit word, under the owner's
own credentials.

The approval is the change's, not the finding's: the owner has read the
proposal and the pull request and is saying "make the reviewers happy", which
is a decision about the change. What the reviewers then say is not re-decided
per thread.

#### Scenario: The owner approves the change

- **WHEN** the owner tells the session the change is approved for fixing
- **THEN** the session places the label on the change's pull request, and says
  so

#### Scenario: The label is placed with nobody's word

- **WHEN** a session has no explicit approval from the owner
- **THEN** it does not place the label, and a pull request without it is
  triaged per thread

### Requirement: A change is not archived while its fixing loop is open

A change SHALL NOT be archived while a fixing round is running on its pull
request, or while a dispute posted by the loop has no reply from a person.

Archiving folds the deltas into the published specs; doing so under a loop
that may still land a commit, or over a disagreement nobody has ruled on,
records the change as finished while the pull request still cannot merge.

#### Scenario: Archive is attempted mid-loop

- **WHEN** the archive is requested while a round is running
- **THEN** it is refused, naming the running round

#### Scenario: Archive is attempted over an unanswered dispute

- **WHEN** the archive is requested while a disputed thread has no reply from
  a person
- **THEN** it is refused, naming the thread

#### Scenario: The loop has ended and every dispute is answered

- **WHEN** the summary has posted and every disputed thread carries a person's
  reply
- **THEN** the archive proceeds as before
