## ADDED Requirements

### Requirement: A component that does not build is not read

Before a component's reading starts, the review SHALL build that component
with the same recipe the project's CI uses for it, derived from the tree. A
component whose build fails SHALL NOT be read: no model runs for it, the
summary names it as UNBUILT with the tail of the build's own output, and none
of its files is recorded as reviewed.

A group of paths that has no build — documentation, specifications, the
workflows, the rules — SHALL always be read.

**A reading of code that does not compile spends a model on what a compiler
already said.** The author is about to change that code; every finding
against it is a finding against a version that will not be merged, and the
reader's turns go to the build error rather than to the doctrine the review
exists to check.

The build SHALL hold no credential the review uses, and the recipe SHALL be
taken from the default branch, so that a pull request cannot declare itself
built.

#### Scenario: A component fails to build

- **WHEN** a pull request changes a component and that component's build
  fails on the head
- **THEN** no reading runs for it, the summary lists it as unbuilt with the
  build's last lines, and the next review reads it again in full

#### Scenario: A component builds

- **WHEN** the component's build succeeds
- **THEN** its reading runs as before, on the same job

#### Scenario: A documentation-only change

- **WHEN** a pull request changes only paths outside every component
- **THEN** nothing is built and every group is read

#### Scenario: A pull request breaks its own build recipe

- **WHEN** a pull request edits the file that decides how a component is
  built
- **THEN** the review builds with the default branch's copy and says so

### Requirement: A push is reviewed as its delta since the last review

The review SHALL record, on the pull request itself, which paths it read and
at which head, and on the next run SHALL read only the changed paths that
are new, that changed since the head they were last read at, or whose record
no longer holds. Every other changed path SHALL be CARRIED: not read, its
unresolved threads left standing, its resolved threads counted as before.

A record SHALL no longer hold when:

- the head it names is not an ancestor of the current head — a rebase or a
  force-push;
- what a file is judged against has changed since that head: a rule file, or
  the change's own delta specifications.

In either case every changed path SHALL be read in full, and the review SHALL
say why.

The record SHALL be written by a program from the readings that were actually
validated — a path no reader returned, or in a component that was unbuilt or
unreviewed, is not recorded — and it SHALL travel with the summary, in the
same post, so that it exists exactly when the review it describes does. This
is the existing rule — the pull request is the review's memory, and there is
no second store — applied to what was read as it already applies to what was
found.

A reading of a delta SHALL still hold the whole file, every thread on it, and
the names of every changed sibling, carried ones included, so that a finding
can land on any line and a reference to a carried file is recognisable.

**A file no commit touched since it was read is a file whose reading has not
changed.** Re-deriving it on every push costs the whole pull request per push
and finds nothing the threads do not already hold; what a reader needs after
a push is what is new, and that is the delta.

#### Scenario: A push touches two of ten changed files

- **WHEN** a pull request whose ten changed files were all read receives a
  push changing two of them
- **THEN** two files are read, their diffs taken from the head they were last
  read at, eight are carried, and the summary says so

#### Scenario: A push adds a file to a component that was otherwise read

- **WHEN** the push adds a changed path no record names
- **THEN** that path is read from the base, and the component's other
  unchanged paths are carried

#### Scenario: A branch is rebased

- **WHEN** the recorded head is no longer an ancestor of the current head
- **THEN** every changed path is read in full, and the summary states that
  the record was invalidated by a rebase

#### Scenario: A rule file changes

- **WHEN** a rule file differs between the recorded head and the current
  head, on whichever side brought the change
- **THEN** every changed path is read in full, and the summary states why

#### Scenario: A person asks for a full review

- **WHEN** the review is dispatched by hand with the full option
- **THEN** every changed path is read from the base, whatever the record says

#### Scenario: A component was unbuilt last run

- **WHEN** a component was unbuilt on the previous review and builds now
- **THEN** every one of its changed paths is read, none having been recorded

#### Scenario: The previous run was a dry run

- **WHEN** the previous run posted nothing
- **THEN** no record was written by it, and the next run reads against the
  last posted record

### Requirement: Carried files keep their place in the cross-check

A carried file SHALL remain a consumer the consolidation can find: the reach
search for every removed or renamed name SHALL exclude only the paths read
this run, so that a file reviewed on an earlier push is checked against what
this push changed. A carried file's unresolved threads SHALL be reported as
standing by the program that carried it, never re-judged by a model that did
not read the file.

#### Scenario: A read file removes a name a carried file uses

- **WHEN** a file read this run declares a name removed and a carried file
  references it
- **THEN** the consolidation finds the carried file by search, reads it, and
  raises a finding against it

#### Scenario: A carried file has an open thread

- **WHEN** a file is carried and one of the review's threads on it is
  unresolved
- **THEN** the thread is left as it is and counted as carried over, and no
  reply is posted in it

## MODIFIED Requirements

### Requirement: A finding is written to be triaged, not read as an essay

An inline finding SHALL be four labeled lines and nothing else:

| Line | Holds | Bound |
|---|---|---|
| `Claim` | the one thing a person can accept or reject | one clause, at most fifteen words |
| `Where` | the paths and lines concerned | paths only, no sentence |
| `Rule` | the rule file and heading, or the spec path, contradicted | nothing after the heading; omitted when none |
| `Fix` | the obvious fix | at most twelve words; omitted when not obvious |

The consequence of a claim SHALL live in `Where` and `Rule`, never in a clause
appended to the claim, and the review SHALL count the words before posting.

The summary SHALL be a count line, a reach line stating which changed names
were followed to how many consumers, a coverage line stating how many of the
pull request's changed files were read this run, how many were carried
unchanged since their last review and how many sit in unbuilt components —
and a table of new findings — with no prose around them. A component that
was unbuilt SHALL appear in the summary by name, distinct from one that was
unreviewed. The record of what was read MAY travel in the summary as a line
the rendered comment does not show.

**Triage happens by reading a thread beside a diff.** A finding that is a wall
of text is skimmed, and a skimmed finding is neither accepted nor dismissed —
it stays open, blocking the merge for a reason nobody read. A cap stated as
"a few lines" was measured and did not hold: the review wrote a bold sentence
and two long sentences, which is prose wearing a shape. Labeled lines with
counted bounds are what held.

#### Scenario: A finding is posted

- **WHEN** the review comments on a line
- **THEN** the comment is the four labeled lines, the claim is one clause
  within its bound, and the rule is named by file and heading rather than
  quoted

#### Scenario: The summary is posted

- **WHEN** the review posts its summary
- **THEN** it is the four counts, the reach line, the coverage line and a
  table of new findings, and a run with nothing new carries the counts, the
  reach line, the coverage line and a table of verdicts

#### Scenario: A component was unbuilt

- **WHEN** a component's build failed this run
- **THEN** the summary names it as unbuilt, and it is not counted as
  unreviewed
