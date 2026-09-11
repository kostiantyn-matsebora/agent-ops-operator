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

<!-- The "A change is approved for automatic fixing by its owner, once"
requirement WAS modified here too, and is REMOVED from this delta:
`conveyor-labels` modifies the same requirement, with different wording (the
`conveyor:` vocabulary, the carry/mint distinction) and a fuller scenario set
— two pending deltas rewriting one requirement is an unreviewable conflict,
not two independent changes. The one scenario unique to this delta (the
interactive-workstation case) was folded into conveyor-labels' own delta
instead of being lost, and this change no longer touches that requirement at
all. -->
