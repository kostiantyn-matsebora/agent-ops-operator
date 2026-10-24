## ADDED Requirements

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
