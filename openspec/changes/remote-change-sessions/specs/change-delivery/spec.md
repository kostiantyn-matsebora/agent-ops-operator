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

**THIS BLOCK IS SUPERSEDED BY `conveyor-labels`, WHICHEVER OF THE TWO CHANGES
ARCHIVES SECOND CORRECTS IT.** The published requirement already carries a
defect — "the session working the change" places the approve label "under
the owner's own credentials" — that a session cannot actually do: it acts as
an application with no write access, and placing the label that way is
exactly what #201 measured failing live (the gate refused it and stripped the
label). This change's own contribution is the SECOND scenario below, stated
so it does not depend on that broken claim continuing to be there;
`conveyor-labels` carries the fully corrected requirement (the session places
NO label; a workflow carries the owner's standing instruction forward
instead) in its own `specs/change-delivery/spec.md` delta, and whichever of
the two changes archives second is what leaves `openspec/specs/` correct.

The owner of a change SHALL approve its pull request for automatic fixing
ONCE, by a stated label on the pull request, placed by someone with write
access to this repository — directly, or by a program relaying that person's
standing instruction. The owner's word MAY be given as the implement label
placed on the issue the change was started from, before the pull request
exists.

The approval is the change's, not the finding's: the owner has read the
proposal — or, having labelled the issue, has said the issue is to be built
and its reviewers satisfied — and is saying "make the reviewers happy", which
is a decision about the change. What the reviewers then say is not
re-decided per thread.

#### Scenario: The owner approves the change

- **WHEN** the owner tells a remote session the change is approved for
  fixing, and no standing instruction on the issue exists for a program to
  relay
- **THEN** the session places no label — it acts as an application with no
  write access, and there is no interactive `gh` session of the owner's to
  hand the command to mid-run — and instead names the exact command in its
  own pull request description, for the owner to run afterward under their
  own credentials

#### Scenario: The owner approves interactively, on a workstation

- **WHEN** the owner asks an interactive session running under their own `gh`
  login whether the change is approved for fixing, and confirms it is
- **THEN** the assistant tells the owner the command rather than running it,
  and only the owner's own `gh` session — never the assistant — executes it

#### Scenario: The owner labelled the issue

- **WHEN** a change was started by the implement label on its issue, placed
  by a person with write access
- **THEN** the pull request that opens for it carries the approve label
  before it needs to be asked for again, and its description says the
  approval came from the issue

#### Scenario: The label is placed with nobody's word

- **WHEN** no explicit approval from the owner exists in either form
- **THEN** no label is placed, and a pull request without one is triaged per
  thread
