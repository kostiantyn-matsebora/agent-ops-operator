## MODIFIED Requirements

### Requirement: A change is implemented in its own working copy

Every openspec change SHALL be implemented in a working copy dedicated to it:
on a workstation a git worktree, created when implementation begins and removed
when the change is archived; in a remote session the session's own clone, which
is that copy by construction and SHALL NOT have a worktree added beside it.

**One session, one change, one working copy.** Sessions already carry the name
`<phase> <change>`, and two of them sharing a working copy share both a HEAD and
a set of files. That is not a hypothetical in either direction: a branch created
by one session moved another's HEAD and the branches diverged, and separately a
session cleaning the tree deleted a second session's entire unstaged change
directory. Isolation is what makes a branch per change safe at all, so it is a
requirement of this capability rather than a convenience within it.

**A remote session is the isolation, not a place to re-create it.** Its clone
is fresh, its HEAD is its own, and its push reaches the branch it works on; a
worktree added there is a second copy of the tree that doubles the derived
inventory for no isolation it did not already have.

#### Scenario: Two changes are worked at once

- **WHEN** two sessions implement two different changes at the same time
- **THEN** each has its own working copy and its own HEAD, and neither session's
  commits, checkouts, branch operations or working files are visible to the
  other

#### Scenario: One session cleans its tree

- **WHEN** a session discards untracked files in its own working copy
- **THEN** no other session's work is affected, because no other session's work
  is in that copy

#### Scenario: The change is archived

- **WHEN** a change completes and is archived
- **THEN** its worktree is removed, leaving no working copy that would be
  mistaken for live work

#### Scenario: A remote session implements a change

- **WHEN** a change is implemented in a remote session
- **THEN** the session checks out `change/<name>` in its clone and works there,
  and no worktree exists for that change anywhere

### Requirement: A change is approved for automatic fixing by its owner, once

The owner of a change SHALL approve its pull request for automatic fixing
ONCE, by a stated label on the pull request, and that label SHALL be placed by
the session working the change on the owner's explicit word, under the owner's
own credentials.

The owner's word SHALL be given in one of two forms: told to the session
working the change, or the implement label placed by the owner on the issue
the change was started from. In the second form the session SHALL place the
approve label on the pull request at creation.

The approval is the change's, not the finding's: the owner has read the
proposal — or, having labelled the issue, has said the issue is to be built and
its reviewers satisfied — and is saying "make the reviewers happy", which is a
decision about the change. What the reviewers then say is not re-decided per
thread.

#### Scenario: The owner approves the change

- **WHEN** the owner tells the session the change is approved for fixing
- **THEN** the session places the label on the change's pull request, and says
  so

#### Scenario: The owner labelled the issue

- **WHEN** a change was started by the implement label on its issue, placed by
  a person with write access
- **THEN** the session opening its pull request places the approve label at
  creation, and the pull request's description says the approval came from the
  issue

#### Scenario: The label is placed with nobody's word

- **WHEN** a session has no explicit approval from the owner in either form
- **THEN** it does not place the label, and a pull request without it is
  triaged per thread
