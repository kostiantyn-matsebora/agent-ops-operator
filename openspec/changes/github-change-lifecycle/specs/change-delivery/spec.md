## Purpose

How a change reaches the default branch: the isolated working copy it is built
in, the branch and pull request that carry it, and the checks that pull request
must show before anyone decides to merge it.

The concern is not ceremony. Several sessions work this repository at once, and
until now they shared one working copy and therefore one HEAD — which made
branching unsafe and left the default branch the only place work could land,
unreviewed and unchecked.

## ADDED Requirements

### Requirement: A change is implemented in its own working copy

Every openspec change SHALL be implemented in a git worktree dedicated to it,
created when implementation begins and removed when the change is archived.

**One session, one change, one worktree.** Sessions already carry the name
`<phase> <change>`, and two of them sharing a working copy share a HEAD: a
branch created by one moves the other's HEAD out from under it. That is not a
hypothetical — it happened, the branches diverged, and the recovery was a
cherry-pick. Isolation is what makes a branch per change safe at all, so it is
a requirement of this capability rather than a convenience within it.

#### Scenario: Two changes are worked at once

- **WHEN** two sessions implement two different changes at the same time
- **THEN** each has its own working copy and its own HEAD, and neither
  session's commits, checkouts or branch operations are visible to the other

#### Scenario: The change is archived

- **WHEN** a change completes and is archived
- **THEN** its worktree is removed, leaving no working copy that would be
  mistaken for live work

### Requirement: Worktrees live outside the repository tree

A worktree SHALL be created outside the repository's own directory.

**This is forced, not preferred.** The release inventory is DERIVED from the
filesystem by searching the tree for module manifests and container recipes, so
a second copy of the tree beneath the root doubles that inventory and hands
every matrix in CI twice the work it should have — building and publishing
under names that describe nothing. The repository's own structural test, that
exactly one documentation directory exists anywhere in the tree, fails in the
same instant.

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
nothing that can be read from the branch is asked for again by hand.

**A merge SHALL leave exactly one commit on the default branch**, whose subject
follows the project's commit convention. The history of that branch is read as
an account of the project, so one change is one line in it; a merge that leaves
either a mechanical merge subject or a change's whole working history breaks
that reading.

#### Scenario: A change is delivered

- **WHEN** a change is implemented and ready
- **THEN** it arrives as a pull request from its own branch, and the default
  branch receives no direct commit for it

#### Scenario: The pull request is merged

- **WHEN** a change's pull request is merged
- **THEN** the default branch gains exactly one commit, whose subject follows
  the commit convention and describes the change

### Requirement: A pull request carries the project's gates as checks

Every gate that decides whether a change may land SHALL run on the pull request
and report there.

This SHALL include validation of the openspec artifacts themselves and the
requirement that a change carries a completed documentation task. Both are
gates the project already asserts; neither ran anywhere a reviewer could see.

**A gate enforced only inside one contributor's local tooling is not enforced.**
It is absent for every other contributor, for every session whose tooling is not
installed, and for automation. Lifting such a gate onto the pull request is
what makes it a property of the change rather than of whoever happened to write
it.

**A check that is skipped because it had nothing to do SHALL be treated as
passed**, and a check that was cancelled SHALL NOT be. Path filtering and
reporting are otherwise incompatible: a job that does not run reports no result
at all.

#### Scenario: A change's openspec artifacts are invalid

- **WHEN** a pull request carries a change whose openspec artifacts do not
  validate
- **THEN** a check fails on that pull request, naming what is invalid

#### Scenario: A change's documentation task is unfinished

- **WHEN** a pull request carries a change whose documentation task is missing
  or incomplete
- **THEN** a check fails on that pull request, and the failure names both
  halves the task must cover

#### Scenario: A pull request touches one component

- **WHEN** a pull request changes files belonging to one component only
- **THEN** the checks that had nothing to do report as skipped and the overall
  gate still reports a result
