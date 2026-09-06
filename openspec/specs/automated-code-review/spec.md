# automated-code-review Specification

## Purpose
The automated review a pull request receives on every push: what it looks for,
what it must not say twice, which of its own remarks it closes when they are
addressed, and the privilege boundary that lets it close anything at all.

A review that restates its previous findings on each push is worse than no
review, because the signal a reader needs — *what is new since I last looked* —
is buried under everything already dealt with.

## Requirements

### Requirement: Every push to a pull request is reviewed

A pull request SHALL be reviewed when it opens, on every subsequent push, and
when it leaves draft. Findings SHALL be delivered as comments on the specific
lines they concern, alongside one summary.

A superseded review SHALL be abandoned rather than raced, so that a burst of
pushes produces one review of the latest state rather than several of stale
ones.

#### Scenario: A pull request receives a second push

- **WHEN** a contributor pushes again to an open pull request
- **THEN** the review runs again against the updated state

#### Scenario: Several pushes arrive together

- **WHEN** pushes arrive faster than a review completes
- **THEN** only the newest is reviewed, and the superseded runs produce no
  comments

### Requirement: A pull request the review cannot run on is skipped, not failed

Where the review cannot run because the pull request comes from a fork — and the
credential it needs is therefore unavailable by design — it SHALL report as
skipped rather than fail.

**A red check a contributor cannot turn green is a barrier, not a signal.** The
repository is public and takes contributions from forks, where a workflow's
token is read-only and repository secrets are withheld deliberately. Failing
there would tell an outside contributor their change is broken when nothing
about their change is.

The skip SHALL be visible, so that a maintainer can tell an unreviewed pull
request from a reviewed one.

#### Scenario: A fork opens a pull request

- **WHEN** a pull request arrives from a fork
- **THEN** the review reports skipped, the contributor sees no failure, and the
  pull request is visibly unreviewed

#### Scenario: A branch in this repository opens a pull request

- **WHEN** a pull request arrives from a branch of this repository
- **THEN** the review runs normally

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

### Requirement: A finding that has been addressed is closed

When a push resolves a finding, the review SHALL reply saying so and mark that
remark's thread resolved. The same SHALL apply when the code a finding concerned
no longer exists.

#### Scenario: A finding is fixed

- **WHEN** a push fixes the problem a remark described
- **THEN** the remark's thread is answered and marked resolved

#### Scenario: The code is deleted

- **WHEN** a push removes the code a remark concerned
- **THEN** the remark's thread is answered as no longer applicable and marked
  resolved

### Requirement: The review never closes a remark it did not make

Resolution SHALL be restricted to threads the review itself authored, and that
restriction SHALL be enforced mechanically rather than by instruction.

**This is the one failure here that destroys information.** Every other mistake
adds noise a reader can ignore; resolving a human reviewer's thread hides a
person's objection and reports it as handled. An instruction can be
misinterpreted, so the constraint belongs where it cannot be.

#### Scenario: A human leaves a review remark

- **WHEN** a person comments on a line and the review runs afterwards
- **THEN** that thread is never resolved by the review, whatever the code now
  says

### Requirement: A stale anchor is not treated as a fix

When a remark's thread has been detached from the code by an unrelated edit, the
review SHALL re-check whether the finding still holds. If it does, the finding
SHALL be raised again against its current location.

A detached thread SHALL NOT be resolved on the strength of being detached.

**Detachment means the remark has become invisible, not that it has been
addressed.** A reformat detaches a live finding, and a fix elsewhere leaves a
dead one attached — so the anchor's state says nothing about the code, while
looking exactly as though it does.

#### Scenario: An unrelated edit moves the line

- **WHEN** a push edits the region around a finding without addressing it
- **THEN** the finding is raised against its new location and the detached
  thread is closed as superseded

### Requirement: A finding dismissed by a person stays dismissed, and is reported

When a person resolves one of the review's threads, the review SHALL NOT raise
that finding again.

The summary SHALL state how many findings were dismissed this way.

**Deferring to the person is right; doing it silently is not.** A finding that
was resolved rather than fixed has left the reader's view, and a count is what
keeps the gap visible instead of making the summary read as though everything
raised had been dealt with.

#### Scenario: A maintainer resolves a finding without changing the code

- **WHEN** a person marks one of the review's threads resolved and pushes
  nothing
- **THEN** the review does not raise it again, and the summary reports it as
  dismissed

### Requirement: What may close a thread holds no power to change the code

The step that resolves threads SHALL be separated from the step that produces
the review, and SHALL run without any model deciding what it does. Only the
separated step SHALL hold the privilege that resolution requires.

**Resolving a thread requires write access to the repository's contents**, not
merely to its pull requests. Granting that to the reviewing step would give a
process driven by generated output the ability to push — so the privilege is
held by a step that follows instructions it cannot rewrite, acting on a list it
is handed.

This is the project's existing rule applied one layer out: *the component
running untrusted model output must not out-rank the one orchestrating it.*

#### Scenario: The review runs

- **WHEN** the reviewing step executes
- **THEN** it can read the code and comment on the pull request, and it cannot
  write to the repository

#### Scenario: Threads are resolved

- **WHEN** findings are closed
- **THEN** the closing is performed by a step whose behaviour is fixed, acting
  only on the threads it was given

### Requirement: The review is measured against the project's own rules

The review SHALL evaluate a change against the project's recorded invariants,
retired vocabulary and the change's own specifications, in addition to ordinary
correctness concerns.

**This is the reading nothing else performs.** Several rules in this repository
record decisions that were re-made, reverted, and re-made again — a class of
regression that compiles, passes every test, renders every chart, and is visible
only to a reader who knows what was decided before.

#### Scenario: A change contradicts a recorded invariant

- **WHEN** a diff reintroduces behaviour the project recorded as removed, or
  contradicts a stated invariant
- **THEN** the review raises it, naming the rule it contradicts

#### Scenario: A change contradicts its own specification

- **WHEN** a diff implements something other than what the change's specs
  require
- **THEN** the review raises the discrepancy

### Requirement: A finding is accepted in its own thread, in a stated vocabulary

A person SHALL accept a finding by replying to its thread, and the phrases that
count as acceptance SHALL be a fixed, documented set matched mechanically.

**What decides that code will be written to a branch may not be a judgement
call.** An interpreted acceptance would make the same sentence mean different
things on different days, and the reader typing it could not know which.

Anything that is not an acceptance — a question, a disagreement, silence — SHALL
leave the thread and the code exactly as they are. A finding is never actioned
because the conversation around it sounded agreeable.

#### Scenario: A finding is accepted

- **WHEN** a person replies to a finding's thread with a phrase in the accept
  vocabulary
- **THEN** that finding is included in the next dispatch

#### Scenario: A finding is answered with anything else

- **WHEN** a person replies with a question, an objection, or nothing at all
- **THEN** the finding is not actioned, the thread stays open, and the code is
  untouched

### Requirement: One dispatch acts on everything accepted

A dispatch SHALL be a single instruction that acts on every accepted finding at
once, and the work SHALL arrive as ONE commit. On a pull request approved as a
whole, "accepted" SHALL mean every open finding and every open analysis issue,
and the instruction SHALL be the review's own completion rather than a comment.

Acting on findings one at a time would put several runs on one branch
simultaneously, each pushing to it, each triggering the review again — and the
last to land would decide what the branch says. Under the loop that hazard is
a ROUND: one dispatch per review completion, serialised per pull request.

#### Scenario: Several findings are accepted

- **WHEN** a dispatch runs against more than one accepted finding
- **THEN** all of them are addressed together and the branch gains one commit

#### Scenario: Nothing has been accepted

- **WHEN** a dispatch runs with no finding accepted
- **THEN** it reports that and writes nothing

#### Scenario: A round starts while one is running

- **WHEN** a review completes on a labelled pull request while a round is
  still landing
- **THEN** the new round queues behind it and collects afresh when it starts

### Requirement: The work list comes from the threads, not from the dispatch

The set of findings a dispatch acts on SHALL be derived by a program reading the
review threads, and SHALL NOT be taken from the text of the dispatch itself.

The dispatch is a TRIGGER. Were it also the instruction, the sentence that
authorises work and the sentence that describes it would be the same sentence,
and a person could direct arbitrary work through a mechanism whose authorisation
was granted for something else.

Each item SHALL carry the finding, where it points, and the words the person
answered with — an acceptance often says which part of the finding is agreed.

#### Scenario: A dispatch names work of its own

- **WHEN** a dispatch comment asks for something no thread accepted
- **THEN** that work is not performed, because the list is built from threads

### Requirement: What writes the fix holds no power to push it

The step that produces a fix SHALL NOT hold write access to the repository. It
SHALL emit its work as a patch, and a SEPARATE step, running no model, SHALL
apply and push it.

This is the project's rule — *the component running untrusted model output must
not out-rank the one orchestrating it* — applied to the step that changes code,
exactly as the existing thread-closing requirement applies it to the step that
closes threads. A fixing step with push rights is a model with commit access to
a branch, gated only by who summoned it.

#### Scenario: A fix is produced

- **WHEN** the fixing step runs
- **THEN** it can read the repository and emit a patch, and it cannot write to
  the repository

#### Scenario: A fix is landed

- **WHEN** a patch is applied to the branch
- **THEN** it is applied by a step whose behaviour is fixed, acting only on the
  patch it was handed

### Requirement: A dispatch is authorised by who sent it

A dispatch SHALL be honoured only from a person with write access to the
repository, established from the triggering comment's own author association,
and SHALL be refused on a pull request from a fork.

A comment is a stranger's trigger. Without this, anybody who can type in a
public pull request can start a run that writes to a branch.

#### Scenario: A dispatch arrives from someone without write access

- **WHEN** a person without write access sends a dispatch
- **THEN** nothing runs, and the refusal is visible rather than silent

#### Scenario: A dispatch arrives on a fork's pull request

- **WHEN** the pull request comes from a fork
- **THEN** the dispatch is refused

### Requirement: A fixed finding is answered and closed, and nothing else is

When a dispatch lands a fix, each fixed finding's thread SHALL receive a reply
naming the commit that addressed it, and SHALL then be closed by the existing
separated step.

A thread nobody accepted, and a thread the fixer disputed, SHALL be left open.
Since a pull request cannot merge with an unresolved conversation, an untriaged
or disputed finding holds the merge until a person deals with it — which is the
intended cost, not a side effect.

#### Scenario: A finding is fixed

- **WHEN** the patch for an accepted finding is pushed
- **THEN** its thread is answered with the commit and then closed

#### Scenario: A finding was never accepted

- **WHEN** a dispatch completes
- **THEN** every thread nobody accepted is still open, and the pull request
  still cannot merge

#### Scenario: A finding was disputed

- **WHEN** a round completes with a disputed thread
- **THEN** that thread is still open, carries the dispute, and the pull request
  still cannot merge

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

### Requirement: The review is consolidated across components, on what changed

After the per-component readings, the review SHALL check the whole change for
compatibility: the names each reading reports as changed — identifiers,
fields, paths, environment variables — SHALL be resolved to their consumers
mechanically, and each consumer SHALL be checked against the change. Consumers
INSIDE the change SHALL be found from the readings' own references, and
consumers outside it by searching the repository.

The consolidation SHALL be a reading of its own — the COORDINATOR — that runs
as its own job once every reading's job has finished, and is handed every
reading's data as files: a reading whose job produced none is handed as
absent, by name. It SHALL never wait on a running reading, and it is the only
reading that writes to the pull request. Its context SHALL hold its role, the
readings and the threads, and no rule file; consumers inside the change SHALL
be judged from the readings' declares and references, and a consumer outside
it by reading that one file.

**This repository's modules import nothing from one another**, so a contract
change compiles everywhere, passes every module's tests, and breaks at runtime
in a component the diff never names. The contract file reads as correct because
it is; it is no longer what its consumers speak. Only a reading that follows
the name to where it is used can see that.

The summary SHALL state the reach that was checked, so a reader sees what the
review considered rather than trusting that it considered everything.

#### Scenario: A contract field is renamed

- **WHEN** a pull request renames a field in an HTTP contract and updates the
  manager's handler
- **THEN** every adapter that speaks the old name is found and a finding is
  raised against each that still does

#### Scenario: A change reaches nothing outside itself

- **WHEN** the changed names have no consumer outside the changed components
- **THEN** the summary says so, and no cross-component finding is raised

#### Scenario: The coordinator is the only writer

- **WHEN** the readings have returned
- **THEN** exactly one reading posts to the pull request, once, and a run in
  which it posted no summary is reported as failed

#### Scenario: A removed name is referenced by another changed file

- **WHEN** one file's reading declares a name removed or renamed and another
  file's reading references it
- **THEN** the coordinator raises a finding against the referencing file from
  the two readings, without reading either file

#### Scenario: A reading's job failed

- **WHEN** a reading's job ended without a validated reading
- **THEN** the coordinator still runs, with that component handed as absent,
  and the summary names it as unreviewed

### Requirement: The reviewer's definition is part of the guarded review

The definitions of the review's roles — the file reader and the
coordinator, each its instructions and the tools it may use — and the steps
that install the model's tooling and hand each role its input SHALL be taken
from the DEFAULT BRANCH when the review runs, never from the pull request's
checkout.

**A pull request may not rewrite the thing that judges it.** A definition read
from the branch under review could be weakened by that branch; one restored
from the default branch before the run inherits the workflow file's own
guard, which the review already refuses to run when a pull request's copy
differs.

The definitions SHALL live in ordinary files — one agent definition per role,
each one file with one system prompt — so the same review is runnable by hand
against any pull request, by dispatching the same workflow. The plan that
starts and collects the readings is the workflow itself; there is no second
copy of it to keep in step.

#### Scenario: A pull request edits the reviewer

- **WHEN** a pull request changes a role definition or the review's tooling
  step
- **THEN** the review runs with the default branch's copies and says so on the
  pull request; the edited files are reviewed like any other change

#### Scenario: The review is run from a checkout

- **WHEN** a person dispatches the review against a pull request from their
  own checkout
- **THEN** the same jobs run, with the same roles, and MAY stop after the
  readings without posting when asked to

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

### Requirement: A pull request may be approved for fixing as a whole

A person with write access SHALL be able to approve a pull request for fixing
as a whole, by placing a stated label on it, and that approval SHALL mean every
finding the review raises on that pull request — now and in later rounds — is
accepted without a reply in its thread.

The per-thread vocabulary stays the ONLY consent on an unlabelled pull request.
The label is a second consent, wider by declaration: it is set by a person whose
access already authorises a dispatch, on a change that person has approved, and
it is visible on the pull request for as long as it applies.

#### Scenario: A labelled pull request is reviewed

- **WHEN** the review posts findings on a pull request carrying the label
- **THEN** a dispatch starts with no reply in any thread and no dispatch
  comment, acting on every open finding

#### Scenario: An unlabelled pull request is reviewed

- **WHEN** the review posts findings on a pull request without the label
- **THEN** nothing is dispatched until a person accepts findings in their
  threads and sends a dispatch, exactly as before

#### Scenario: The label is placed by someone without write access

- **WHEN** the label appears on a pull request and the person who placed it
  lacks write access, or the pull request comes from a fork
- **THEN** no dispatch starts, and the refusal is visible on the pull request

#### Scenario: The label is removed mid-loop

- **WHEN** the label is removed while a round is running
- **THEN** the running round completes and lands, and no further round starts

### Requirement: The work list of an approved pull request includes the analysis service's issues

On a labelled pull request the dispatch's work list SHALL include the open
issues the code-quality analysis reports for that pull request, collected by a
program from the service's API, per component project, beside the open review
threads. The model SHALL NOT read either API.

An issue the analysis raised is a finding by another reviewer, and a loop that
fixed one reviewer's findings while the other's held the merge would end with
the pull request still blocked and nobody told why.

#### Scenario: The analysis reports issues on a labelled pull request

- **WHEN** the analysis has open issues for the pull request's components
- **THEN** each is an item in the dispatch's work list, carrying the issue's
  key, rule, file and line

#### Scenario: The analysis has not yet reported

- **WHEN** a dispatch collects while the analysis for the head sha is absent
- **THEN** the round proceeds over the review threads alone and the summary
  says the analysis was not consulted

#### Scenario: An issue was fixed

- **WHEN** a landed fix removes the code an issue pointed at
- **THEN** the dispatch does not change the issue's state in the analysis
  service; the service's next analysis closes it

### Requirement: A finding is fixed or disputed, never dropped

Under whole-pull-request approval, the fixing step SHALL either fix each item
or DISPUTE it with a stated reason, and SHALL NOT leave an item unaddressed.

A dispute SHALL be posted as a reply in the finding's thread, or — for an
analysis issue, which has no thread — as one comment on the pull request naming
the issue's key. A disputed thread SHALL stay open, and the dispute SHALL NOT be
recorded in the analysis service. The person who approved the pull request
SHALL be mentioned in the round's summary for every dispute.

A disagreement is a decision still owed to a person. An open thread already
holds the merge, so a dispute costs nothing new — it is the notification that
is new.

#### Scenario: The fixer disagrees with a finding

- **WHEN** the fixing step judges a finding wrong
- **THEN** the thread receives a reply stating why, the thread stays open, the
  code is untouched, and the summary names the thread and mentions the approver

#### Scenario: The fixer disagrees with an analysis issue

- **WHEN** the fixing step judges an analysis issue wrong
- **THEN** one pull request comment names the issue key and the reason, the
  issue is left as the service reports it, and the summary mentions the approver

#### Scenario: A previously disputed finding is raised again

- **WHEN** a later round finds a thread already carrying a dispute
- **THEN** the thread is not disputed a second time and is not fixed; it is
  counted as awaiting the person

### Requirement: A landed fix on an approved pull request starts the next round

When a dispatch lands on a labelled pull request, the review and the
continuous-integration checks SHALL run on the landed commit without a person
pushing, and their findings SHALL start the next round.

A push made with the workflow's own token starts nothing, so the landed commit
of an unlabelled dispatch has no checks and no review until somebody pushes
again — a limitation this project documented as the safe side. The loop makes
the next round automatic, and it does so with a credential held ONLY by the
model-free landing step.

#### Scenario: A fix lands on a labelled pull request

- **WHEN** the landing step pushes a fix
- **THEN** the review and the required checks run on that commit, and the pull
  request's merge gate sees them on its head

#### Scenario: A fix lands on an unlabelled pull request

- **WHEN** the landing step pushes a fix under a per-thread dispatch
- **THEN** behaviour is unchanged: the landing comment says a further push is
  needed

### Requirement: The loop is bounded and every ending is summarised

Rounds on one pull request SHALL be capped at a stated number, and a round
that changes nothing — every item disputed, or no patch applied — SHALL end
the loop early. Every ending SHALL post ONE summary comment stating what was
fixed, what was disputed, how many rounds ran, what remains open, and mention
the approver.

The review's verdicts vary between runs of the same file, and the reviewer
reviews the fixer's own commits; without a bound, a self-reviewing loop can
oscillate indefinitely at a cost nobody approved.

#### Scenario: No finding remains

- **WHEN** a round's review posts no finding and the analysis reports no open
  issue
- **THEN** the loop ends and the summary says the pull request is clean

#### Scenario: Only disputes remain

- **WHEN** a round disputes every item and fixes none
- **THEN** the loop ends, the summary lists each dispute, and the approver is
  mentioned

#### Scenario: The cap is reached

- **WHEN** the stated number of rounds has run and findings remain
- **THEN** no further round starts, and the summary lists what remains and
  mentions the approver

#### Scenario: A round's patch is stale

- **WHEN** the branch moved between collection and landing so the patch does
  not apply
- **THEN** the round lands nothing, the loop ends, and the summary says so
