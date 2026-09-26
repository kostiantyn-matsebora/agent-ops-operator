## ADDED Requirements

### Requirement: Every red on the head of an approved pull request is the loop's work

On a pull request approved for fixing as a whole, every failed required check
on the head SHALL be an item on the round's work list. No check SHALL be
exempt for being the loop's own.

A round that lands nothing while items remain eligible SHALL start the next
round itself, counting toward the bound. Only a round that leaves nothing
eligible ends the loop.

An item is eligible while it is neither fixed nor disputed by the fixing
step. A dispute is what the fixing step says about an item, never what the
landing step infers from a patch.

**Measured on #259.** The loop's own guard turned `docs-task` red over a
dispute the landing step had manufactured, the exemption kept the red off the
work list, and the loop ended with the pull request blocked.

#### Scenario: A required check is red on the head

- **WHEN** a round collects on an approved pull request whose head has a
  failed required check
- **THEN** that check is on the work list, whatever its failing step

#### Scenario: A round landed nothing and items remain eligible

- **WHEN** a round's report fixed nothing, and at least one item is neither
  fixed nor disputed by the fixing step
- **THEN** the round counts, its summary names the eligible items, and the
  next round starts without a push or a person

#### Scenario: A round landed nothing and only disputes remain

- **WHEN** a round's report fixed nothing and every remaining item is a
  dispute the fixing step made
- **THEN** the loop is marked waiting, and no round starts on its own

### Requirement: A round starts only on a head whose checks and review have concluded

A round SHALL start on a head only once the head's CI run and its review run
have both concluded. Whichever concludes second SHALL start the round.

A grant placed, or a dispatch asked for, while either is in progress SHALL be
deferred with a notice on the pull request, and the completion SHALL start
the round.

The checks SHALL be reported consulted only when every required check run on
the head has completed. A check run that exists but has not concluded is not
a verdict.

A round reading a half-run CI declares "0 failures" over jobs that have not
spoken. Measured on #259: 38 seconds into a ten-minute run.

#### Scenario: The review completes while CI is running

- **WHEN** the review run on a head concludes while the head's CI run is in
  progress
- **THEN** no round starts, and the CI run's completion starts it

#### Scenario: A grant is placed mid-run

- **WHEN** a person places the fix label while the head's CI or review is in
  progress
- **THEN** the pull request says the round is deferred to the completion, and
  the completion starts it

#### Scenario: A check run exists but has not concluded

- **WHEN** a round collects while any required check run on the head is still
  in progress
- **THEN** the checks are reported as not consulted, and the round claims
  nothing about them

### Requirement: A waiting loop is resumed by a person's comment or by a sweep

A pull request whose loop is waiting SHALL have a round started by a comment
from a person on it, and by a scheduled sweep once no dispute on it is
unanswered.

A dispute is answered by a person's comment after it, or by the thread being
resolved.

A thread resolution fires no workflow event, so a sweep is the only way a
dismissal by resolving ever reaches the loop.

**Measured on #259.** The person resolved the disputed thread, and the next
round repeated the check's stale step name instead of reading the thread.

#### Scenario: A person answers a dispute in its thread

- **WHEN** a person comments on a pull request whose loop is waiting
- **THEN** a round starts, and a disputed thread carrying the person's answer
  is back on the work list

#### Scenario: A person resolves the disputed thread

- **WHEN** the only unanswered dispute is resolved and nobody comments
- **THEN** the next sweep finds no unanswered dispute and starts a round

#### Scenario: A dispute is still unanswered at the sweep

- **WHEN** the sweep reads a waiting pull request whose dispute has neither a
  person's reply nor a resolution
- **THEN** nothing starts, and the loop stays waiting

## MODIFIED Requirements

### Requirement: The line's state is its labels, and nothing else

Which station a change has reached SHALL be readable from the labels on its
issue and its pull request. No separate store, file or naming convention SHALL
hold that state.

Two state vocabularies SHALL exist beside the grant labels, named in the same
vocabulary file, and SHALL grant nothing:

| On | Says | Values |
|---|---|---|
| the issue | which station the line is at | implement, fix, merge, stalled, archive, done |
| the pull request | what the fixing loop is doing | running, waiting, stalled, capped, mergeable |

`waiting` SHALL mean a person's answer is owed: every remaining item is a
dispute the fixing step made. `stalled` SHALL mean the machine stopped: no
report, the fixing step failed or timed out, a stale patch, or the next round
could not start.

Each SHALL be moved by the workflow performing the transition. At most one
value of each SHALL be present at a time. State labels are read by nobody but
people, so a person removing one SHALL change nothing about what the line
does next.

**Anything else is a second source of truth that drifts.** Labels are already
what the gates read, what a person sees without leaving the page, and what a
person can change to move or stop the line.

**One label for two facts was measured misleading on #259.** `loop:stalled`
beside a red pull request and a "clean" summary could not tell a reader
whether the loop had given up or was waiting on them.

#### Scenario: A person asks where a change is

- **WHEN** somebody reads the tracking issue and its pull request
- **THEN** the standing instruction, the station reached, and whether the loop
  is waiting on them or stopped on its own are all visible there

#### Scenario: A merge station label outlives its own condition

- **WHEN** a pull request merges and nothing carries the issue's line onward
- **THEN** the issue's label no longer reads `station:merge`, since that
  label's own meaning is now false

#### Scenario: The station label follows the line

- **WHEN** a station starts, when its pull request goes green, when a person
  merges, and when the archive lands
- **THEN** the issue's station label reads implement, fix, merge, archive and
  done in turn — or stalled in place of archive, when the merge left nothing
  to carry the line onward — and never two at once

#### Scenario: The loop label follows the round

- **WHEN** a round starts, when a round ends with only disputes left, when a
  round ends with its fixer failed, when the round cap is reached, and when
  the head's checks are all green
- **THEN** the pull request's loop label reads running, waiting, stalled,
  capped and mergeable in turn, and never two at once

### Requirement: No required check reports whether a round is running

Whether a fixing round is queued or running for a pull request SHALL NOT be a
required check's verdict. Whether a dispute the loop posted has been answered
SHALL NOT be one either.

Both are the loop's own conversation, moved by the loop and read by people.

A check reporting either red is a red the loop produced itself. A running
round's red starts the next round. An unanswered dispute's red is one no
fixer can clear, so the loop stops on its own refusal.

The archive command SHALL still refuse while a round runs or a dispute is
unanswered, since it acts on the branch a round may push to and folds
disputed work into the published contract.

**Measured on #248.** Every review completion started a round, and every
round ran to its time limit. While it ran, the documentation gate refused on
"a round is still running", and that red started the next round, five pushes
in a row.

**Measured on #259.** The same gate refused on an unanswered dispute, and the
loop ended with the pull request red on a check only a person could clear.
The merge was already held by the open thread. The check added nothing but
the red.

#### Scenario: A round is running while CI evaluates the head

- **WHEN** a fixing round is queued or running for a pull request and its CI
  run evaluates the documentation gate
- **THEN** the gate reports on the tasks file alone, and the running round
  does not turn it red

#### Scenario: CI is red only on an unanswered dispute

- **WHEN** a dispute the loop posted has no answer, and nothing else on the
  head would fail a required check
- **THEN** no required check is red, the merge is held by the open thread
  alone, and the loop is waiting rather than stopped on a red of its own

#### Scenario: A queued run is cancelled before it starts

- **WHEN** replies on review threads queue dispatch runs that the concurrency
  group cancels at once
- **THEN** no required check on the head turns red for it

#### Scenario: The archive is asked for while a round runs

- **WHEN** a person runs the archive command on a change whose pull request
  has a round running
- **THEN** the command is refused until the round ends, exactly as before

### Requirement: A round that ends with a person's answer awaited is not running

A round that ends because nothing is left but disputes awaiting a person, or
because a thread the loop did not author is open, SHALL mark the loop
waiting, never mergeable, never stalled and never running.

A review that opens a thread on a pull request marked mergeable SHALL move it
to waiting.

#### Scenario: A clean round with an open thread

- **WHEN** a round finds nothing accepted to fix and a thread is open
- **THEN** the loop is marked waiting

#### Scenario: A round ends with only disputes left

- **WHEN** a round fixes nothing and every remaining item carries a dispute
  the fixing step made
- **THEN** the loop is marked waiting, and the summary names each dispute and
  whether the head is red or green

## REMOVED Requirements

### Requirement: A required check that reads a person's answer is re-run on that answer

**Reason**: no required check reads a person's answer any more. The dispute
guard left CI, so there is nothing to re-run. A person's comment starts a
round instead, and the round reads the answer.

**Migration**: `dispute-answered.yml` and `rerun-ci-job.py` are deleted. A
person who answered a dispute and saw `docs-task` re-run now sees a round
start.
