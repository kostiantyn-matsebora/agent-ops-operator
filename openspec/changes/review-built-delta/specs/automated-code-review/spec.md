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

### Requirement: A file is read until independent reads go quiet, then carried

The review SHALL record, on the pull request itself, which paths it read, at
which head, and for each how many consecutive reads added no new finding. On
the next run it SHALL read every changed path that is new, that changed since
the head it was last read at, or whose quiet count is below the configured
threshold. A changed path that is unchanged since its last read and whose
quiet count has reached the threshold SHALL be CARRIED: not read, its
unresolved threads left standing, its resolved threads counted as before.

A record SHALL no longer hold when:

- the head it names is not an ancestor of the current head — a rebase or a
  force-push;
- what a file is judged against has changed since that head: a rule file, or
  the change's own delta specifications;
- a person asks for a full review.

In each case every changed path SHALL be read in full, and the review SHALL
say why.

The record SHALL be written by a program from the readings that were actually
validated and from what was actually posted — a path no reader returned, or
in a component that was unbuilt or unreviewed, is not recorded; a path's
quiet count rises only when the run posted no new finding on it, and returns
to zero when it did — and it SHALL travel with the summary, in the same post,
so that it exists exactly when the review it describes does. This is the
existing rule — the pull request is the review's memory, and there is no
second store — applied to what was read as it already applies to what was
found.

A reading of a delta SHALL still hold the whole file, and the names of every
changed sibling, carried ones included, so that a finding can land on any
line and a reference to a carried file is recognisable.

**A reading is a sample, not a function of the file.** The same file read
again yields findings the first read did not — that is why a pull request
here takes several rounds before a read adds nothing, and it is also why a
file cannot be carried after ONE read: what the first read missed would stay
missed until the merge. The review stops reading a file when independent
reads stop finding anything, which is the rule a person applies today, and
it never claims that one read found everything. The threshold is stated as a
number rather than fixed, because how many quiet reads a file needs is a
fact the record will show and nobody knows yet.

#### Scenario: A push touches two of ten changed files

- **WHEN** a pull request whose ten changed files have all gone quiet
  receives a push changing two of them
- **THEN** two files are read, their diffs taken from the head they were last
  read at, eight are carried, and the summary says so

#### Scenario: An unchanged file has not gone quiet

- **WHEN** a push leaves a file unchanged whose last read posted a finding
- **THEN** that file is read again, blind, from the head it was last read at

#### Scenario: A read adds nothing

- **WHEN** a read of an unchanged file posts no new finding and the threshold
  is one
- **THEN** the file is carried on the next run, and remains carried until it
  changes or the record is invalidated

#### Scenario: A push adds a file to a component that was otherwise quiet

- **WHEN** the push adds a changed path no record names
- **THEN** that path is read from the base, and the component's other quiet
  paths are carried

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

### Requirement: A finding already made is not made again

The review SHALL NOT post a remark it has already made and which still
stands, and SHALL NOT raise again a remark a person dismissed. That judgement
SHALL be made at the posting, by the consolidation that holds every thread —
never by a reader: a reader SHALL receive no previous finding and no thread,
so that every read of a file is an independent sample. A finding a reader
raises that an open thread already states SHALL be folded into that thread's
count; one a dismissed thread states SHALL be dropped and counted; one
matching a thread the review had judged fixed SHALL be posted, because the
fix did not hold.

Whether a standing thread is fixed, standing, gone or detached SHALL be
judged by a pass that is handed the thread and the current file and nothing
about the reader's findings — a pass meant to be primed, because its whole
job is the thread.

**Continuity here is a matter of context, not of stored state.** The previous
review's remarks live on the pull request and are handed to the next run, so no
separate record of findings is kept anywhere — no sidecar file, no database, no
labels standing in for review state. A second store would be a second thing to
keep correct, and it would be wrong precisely when a run failed part-way.

**And the context goes to the writer, not to the reader.** The reader used to
be handed its file's threads so that it would not repeat them and could
return a verdict on each. A reader that reads the last reviewer's notes before
the file tends to agree with them, so the review's rounds — which were
measured to keep finding new things on unchanged files — were less
independent than they looked. Suppressing duplicates is a comparison of a
finding with a thread, which the consolidation can make without having read
the file; judging a thread is a job of its own.

#### Scenario: An unaddressed finding survives a push

- **WHEN** a push does not address a finding from the previous review
- **THEN** the existing remark is left as it is, and no duplicate is posted

#### Scenario: A new problem is introduced

- **WHEN** a push introduces a problem not previously remarked on
- **THEN** a new remark is posted for it

#### Scenario: A blind reader raises what a thread already says

- **WHEN** a reader, holding no thread, raises a finding an open thread
  already states on that path
- **THEN** no remark is posted, and the summary counts it as carried over

#### Scenario: A blind reader raises what a person dismissed

- **WHEN** a reader raises a finding a thread a person resolved already
  states
- **THEN** no remark is posted, and the summary counts it as dismissed

#### Scenario: A fix did not hold

- **WHEN** a reader raises a finding on a path whose thread the review had
  judged fixed
- **THEN** the remark is posted as new

### Requirement: A change is reviewed per component, each in isolation

The review SHALL read a pull request as one reading per changed component,
each in its own CI job, on its own runner, all started at once and bounded
only by the platform's job limit — never by a pool sized from one runner's
processors, and never by a model deciding, turn by turn, what to spawn next
or how long to wait. The queue of components — which readings exist and what
each is handed — SHALL be built by a program from the changed paths, never by
a model.

Within a component, the reading SHALL be made PER FILE, each file in its own
model process started by the job, several at once: a reader holds its role
and the text of the rule files that apply to its component as fixed context
identical for every file of the component, and then its file, its diff since
the head the file was last read at, the names of the component's other
changed files and the change's delta specifications — and nothing read for
any other file, no thread and no previous finding. The rule text SHALL be
supplied by a program as the same bytes for every reader of the component,
so that it is paid once per job and served from cache to every further
reader. Each reader returns one reading for its file; a program merges the
readings into the component's, with a file whose reader returned nothing
usable named as unread rather than dropped. A component is one job, never
split for width, since a job costs more to start than a reading.

Each file reading SHALL return, beside its findings, the names the file
DECLARES — added, removed or renamed — and the names it REFERENCES from
outside itself, so that what happens between files is judged from the
readings and not by a context that holds every file at once.

**The cost of a review is then the largest component changed, not the whole
pull request.** A single context reading everything serially pays for every
rule file whether or not the diff touches its subject, and for every component
whether or not it changed — and a pool of two, which is what a four-processor
runner sizes, is serial reading wearing a concurrent shape: on pull request
#106 eight readings started in pairs over ten minutes and the last never
started.

**And a reading cannot be lost.** A reading started by a model's turn ends
with that turn; on pull request #74 three readings were abandoned that way and
the review reported success having posted nothing. A reading that is a
process the job started either produces its file or is a failed process with
a name, and the merge names its file as unread.

**And a context does not grow with the diff, or with the queue.** A component
reader holding every changed file paid for all of them on every turn: on the
first matrix run one component of ten files and a thousand changed lines took
four to nine minutes on the same diff while the others took one to four. The
queue reader that replaced it — one context reading its rules once and then
its files one after another — kept every file it had read, so the last file
of a long queue was read on top of a hundred thousand tokens of the files
before it, and the rules it read once were re-sent on every turn. A reader
that is one process holds the rules and one file, and the rules reach it as a
cached prefix: measured, three processes sharing a thirty-five-kilobyte
prefix cost a tenth of a dollar for the first and two cents for each after.

**And a reading is independent.** A reader that holds another file, or its
own previous findings, is not a second look at the file — it is the first
look continued. The review's rounds are worth having only if each round can
disagree with the last.

A per-component reading SHALL post nothing. It returns what it found as
DATA, in a stated shape a program validates before it is consolidated; a
reading that returns prose instead is a failed reading, reported as such,
never a silent gap.

#### Scenario: A pull request touches three components

- **WHEN** the review runs
- **THEN** three readings run at once as three jobs, each seeing only its
  component, and the pull request receives one set of comments and one summary

#### Scenario: A pull request touches one component

- **WHEN** the review runs
- **THEN** one reading runs, and the rules its readers hold are those for
  that component's paths

#### Scenario: A pull request touches more components than the pool holds

- **WHEN** the review runs with more components than one runner's processors
  would have pooled
- **THEN** every component is still read at once, and the review's wall-clock
  is that of its slowest single reading plus the consolidation

#### Scenario: A reading returns nothing usable

- **WHEN** a per-component reading ends without data in the stated shape
- **THEN** the review reports that component as unreviewed, by name, and the
  remaining readings are consolidated as usual

#### Scenario: The queue is built

- **WHEN** the review starts
- **THEN** the components, their paths, the standing threads and the change's
  delta specs are produced by a program without a model call, and the readings
  start only once that queue exists

#### Scenario: A component with many changed files

- **WHEN** a component's diff spans many files
- **THEN** each file is read by its own process holding the shared rule text
  and that file alone, the readings are merged by a program, and a file whose
  process returned nothing usable is named as unread in the component's
  reading

#### Scenario: A file is read a second time

- **WHEN** a file is read on a later run
- **THEN** its reader holds nothing from the earlier read — no finding, no
  thread — and the earlier threads are judged by the verdict pass

#### Scenario: A name removed in one file is still used in another

- **WHEN** one file's reading declares a name removed or renamed and another
  file's reading references it
- **THEN** the review raises a finding against the referencing file, from the
  two readings, without a context that held both files

#### Scenario: A component the pull request does not touch

- **WHEN** the review runs on a pull request whose diff names no path in a
  component
- **THEN** no reading is started for that component — the queue holds only
  components with changed paths, so a one-file change is one reading

### Requirement: No reading carries the project's rules as inherited context

No context the review runs — a file reader, the verdict pass, the
coordinator — SHALL inherit the project's rule files. A file reader SHALL be
handed, by a program, the text of exactly the rule files that apply to its
component's paths, as fixed context ahead of anything specific to its file;
the verdict pass and the coordinator SHALL hold none. The routing from a path
to its rules SHALL be a program, and every rule file SHALL be reachable from
some path — a rule no path routes to is a rule the review has stopped
enforcing, silently.

**The rules are what a reading is measured against, not what it thinks with.**
A context that inherits every rule file pays for all of them on every turn:
the coordinator, which reads no rules at all, spent 268 of its 273 seconds in
34 turns each re-sending some 76 thousand tokens, of which its readings and
threads were a few thousand.

**And a rule handed as the same bytes to every reader is paid for once.** A
reader told to read its rules paid for them as fresh input every time one
started; a reader handed them as a fixed prefix identical across its
component's files pays the first time and reads from cache after. The
routing is unchanged; what moved is who does the reading.

#### Scenario: A file under the chart is read

- **WHEN** a file reader is started for a path under `chart/`
- **THEN** its fixed context holds the chart rule and the rules the chart
  depends on, and it inherits no rule file

#### Scenario: Two files of one component are read

- **WHEN** two readers start for paths of the same component
- **THEN** the rule text each holds is byte-identical, and the second is
  served from cache

#### Scenario: The coordinator runs

- **WHEN** the consolidation starts
- **THEN** its context holds its role, the readings and the threads, and no
  rule file

#### Scenario: A rule file no path routes to

- **WHEN** a rule file exists that the routing program maps from no path
- **THEN** the routing program's test fails, naming the rule

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
quiet since their last read and how many sit in unbuilt components — and a
table of new findings — with no prose around them. A component that was
unbuilt SHALL appear in the summary by name, distinct from one that was
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
