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

The review SHALL receive its previous findings as context, and SHALL NOT post a
remark it has already made and which still stands.

**Continuity here is a matter of context, not of stored state.** The previous
review's remarks live on the pull request and are handed to the next run, so no
separate record of findings is kept anywhere — no sidecar file, no database, no
labels standing in for review state. A second store would be a second thing to
keep correct, and it would be wrong precisely when a run failed part-way.

#### Scenario: An unaddressed finding survives a push

- **WHEN** a push does not address a finding from the previous review
- **THEN** the existing remark is left as it is, and no duplicate is posted

#### Scenario: A new problem is introduced

- **WHEN** a push introduces a problem not previously remarked on
- **THEN** a new remark is posted for it

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
once, and the work SHALL arrive as ONE commit.

Acting on findings one at a time would put several runs on one branch
simultaneously, each pushing to it, each triggering the review again — and the
last to land would decide what the branch says.

#### Scenario: Several findings are accepted

- **WHEN** a dispatch runs against more than one accepted finding
- **THEN** all of them are addressed together and the branch gains one commit

#### Scenario: Nothing has been accepted

- **WHEN** a dispatch runs with no finding accepted
- **THEN** it reports that and writes nothing

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

When a dispatch lands a fix, each accepted finding's thread SHALL receive a
reply naming the commit that addressed it, and SHALL then be closed by the
existing separated step.

A thread nobody accepted SHALL be left open. Since a pull request cannot merge
with an unresolved conversation, an untriaged finding holds the merge until a
person deals with it — which is the intended cost, not a side effect.

#### Scenario: A finding is fixed

- **WHEN** the patch for an accepted finding is pushed
- **THEN** its thread is answered with the commit and then closed

#### Scenario: A finding was never accepted

- **WHEN** a dispatch completes
- **THEN** every thread nobody accepted is still open, and the pull request
  still cannot merge

### Requirement: A finding is written to be triaged, not read as an essay

An inline finding SHALL open with a one-sentence claim a person can accept or
reject, followed by at most a few lines naming what breaks or which rule is
contradicted, and SHALL NOT restate the diff or the rule's full argument. The
summary SHALL be a count line and a table of new findings, with no prose around
it.

**Triage happens by reading a thread beside a diff.** A finding that is a wall
of text is skimmed, and a skimmed finding is neither accepted nor dismissed —
it stays open, blocking the merge for a reason nobody read. The authoring rules
that bind this project's own documents bind its review for the same reason.

#### Scenario: A finding is posted

- **WHEN** the review comments on a line
- **THEN** the comment leads with a one-sentence claim, stays within a few
  lines, and names the rule or spec by file rather than quoting it

#### Scenario: The summary is posted

- **WHEN** the review posts its summary
- **THEN** it is the four counts and a table of new findings, and a run with
  nothing new carries the counts and one line
