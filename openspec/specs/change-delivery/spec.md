# change-delivery Specification

## Purpose
How a change reaches the default branch: the isolated working copy it is built
in, and the branch and pull request that carry it.

The concern is not ceremony. Several sessions work this repository at once, and
while they shared one working copy they shared one HEAD — which made branching
unsafe, left the default branch the only place work could land, and destroyed
uncommitted work when one session cleaned the tree under another.

## Requirements

### Requirement: A change is implemented in its own working copy

Every openspec change SHALL be implemented in a git worktree dedicated to it,
created when implementation begins and removed when the change is archived.

**One session, one change, one worktree.** Sessions already carry the name
`<phase> <change>`, and two of them sharing a working copy share both a HEAD and
a set of files. That is not a hypothetical in either direction: a branch created
by one session moved another's HEAD and the branches diverged, and separately a
session cleaning the tree deleted a second session's entire unstaged change
directory. Isolation is what makes a branch per change safe at all, so it is a
requirement of this capability rather than a convenience within it.

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

### Requirement: Worktrees live outside the repository tree

A worktree SHALL be created outside the repository's own directory.

**This is forced, not preferred.** The release inventory is DERIVED from the
filesystem by searching the tree for module manifests and container recipes, so
a second copy of the tree beneath the root doubles that inventory and hands
every matrix in CI twice the work it should have — building and publishing under
names that describe nothing. The repository's own structural test, that exactly
one documentation directory exists anywhere in the tree, fails in the same
instant.

Both failures are silent: the inventory grows, the matrix grows, and nothing
reports that the cause is a working copy rather than a new component.

#### Scenario: The inventory is derived while worktrees exist

- **WHEN** the component inventory is derived from the tree and any number of
  worktrees exist
- **THEN** it reports exactly the components the repository ships, unchanged by
  how many changes are in flight

#### Scenario: A tool resolves a path out of the repository

- **WHEN** any tool searches the repository for a well-known directory or file
- **THEN** a worktree cannot appear in its results, because no worktree is
  inside the searched tree

### Requirement: A change reaches the default branch through one pull request

Each change SHALL be delivered on a branch named for it, through exactly one
pull request, and SHALL NOT be committed directly to the default branch.

The pull request SHALL state the change it implements. Because the branch is
named for the change, that statement SHALL be derivable rather than typed —
nothing readable from the branch is asked for again by hand.

**A merge SHALL leave exactly one commit on the default branch**, whose subject
follows the project's commit convention. That branch's history is read as an
account of the project, so one change is one line in it; a merge leaving either
a mechanical merge subject or a change's whole working history breaks that
reading.

#### Scenario: A change is delivered

- **WHEN** a change is implemented and ready
- **THEN** it arrives as a pull request from its own branch, and the default
  branch receives no direct commit for it

#### Scenario: The pull request is merged

- **WHEN** a change's pull request is merged
- **THEN** the default branch gains exactly one commit, whose subject follows
  the commit convention and describes the change

#### Scenario: The change name is needed

- **WHEN** anything needs to know which change a pull request implements
- **THEN** it is read from the branch, and the contributor is not asked to
  repeat it

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
