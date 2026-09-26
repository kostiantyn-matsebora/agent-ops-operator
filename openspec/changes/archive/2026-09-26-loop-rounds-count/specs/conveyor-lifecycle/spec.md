## MODIFIED Requirements

### Requirement: An unattended loop is bounded, and extended by one consumed label

The number of rounds an unattended fixing loop may run without a person SHALL
be a named constant with a stated default. Reaching it SHALL end the loop with
a summary saying so and naming what remains.

A stated label SHALL grant another set of rounds and SHALL be REMOVED when it
takes effect.

**Every round that ran counts, whatever ended it.** A round whose fixing step
reached its time limit SHALL count toward the bound as a landed round does,
and its summary SHALL say it timed out.

A round in which no model ran at all SHALL NOT count, since nothing was spent
on it.

**The bound is enforced BEFORE a round starts.** A round SHALL begin only
while the rounds used are below the ceiling or the extending label stands.
Counting after the fact let the commit a capped round pushed start the next
round through its review's completion.

**A bound is a ceiling on unattended SPEND, not on progress.** A loop that
finds new things each round is working, and one that cannot land the same fix
is not, and neither is distinguishable in advance.

So the bound is generous, visible when reached, and lifted by a person who can
see what the last rounds produced.

A round that ran out of time spent the ceiling exactly as one that landed did.
A loop that times out every round is the one the ceiling exists for.

**Consumed, because a permanent extension is not a decision.** One placement
grants one set, and placing it again is a fresh judgement made with newer
evidence.

#### Scenario: The loop reaches its bound

- **WHEN** the configured number of rounds has run and work remains
- **THEN** the loop stops, and one summary names the rounds used, what was
  fixed, what is disputed, and how to grant more

#### Scenario: Every round times out

- **WHEN** the fixing step reaches its time limit in each of the configured
  number of rounds
- **THEN** the loop stops at the bound exactly as if each had landed, each
  ending summarised as timed out, and a person is told how to grant more

#### Scenario: A review completes after the last round

- **WHEN** the rounds used equal the ceiling and a review completes on the
  pushed head
- **THEN** no round starts and the loop is marked capped

#### Scenario: A person grants another set of rounds

- **WHEN** the extending label is placed on a bounded pull request
- **THEN** the loop runs again for another set, and the label is removed as it
  is taken

#### Scenario: The extending label is placed twice

- **WHEN** a person places the extending label after a previous set was
  exhausted
- **THEN** another set is granted, each placement having granted exactly one

## ADDED Requirements

### Requirement: No required check reports whether a round is running

Whether a fixing round is queued or running for a pull request SHALL NOT be a
required check's verdict. It is the loop's own transient state, moved by the
loop.

A check reporting it red is a red the loop produced itself, and that red
starts the next round.

The questions a person owes an answer to, such as a dispute the loop posted
that nobody answered, MAY hold a check, since a person's reply re-runs it.

The archive command SHALL still refuse while a round runs, since it acts on
the branch a round may push to.

**A failed check that is only the loop's own guard SHALL start no round.** It
waits for the person it names, and no fixer can answer for them.

**Measured on #248.** Every review completion started a round, and every
round ran to its time limit. While it ran, the documentation gate refused on
"a round is still running".

That red started the next round, five pushes in a row. The pull request
merged only after the grant was removed by hand.

#### Scenario: A round is running while CI evaluates the head

- **WHEN** a fixing round is queued or running for a pull request and its CI
  run evaluates the documentation gate
- **THEN** the gate reports on the tasks file and the unanswered disputes
  alone, and the running round does not turn it red

#### Scenario: CI is red only on an unanswered dispute

- **WHEN** the head's only failed required check is the documentation gate,
  failed because a dispute the loop posted has no answer
- **THEN** no round starts from that failure, and the summary names the
  dispute as what is waited on

#### Scenario: A queued run is cancelled before it starts

- **WHEN** replies on review threads queue dispatch runs that the concurrency
  group cancels at once
- **THEN** no required check on the head turns red for it

#### Scenario: The archive is asked for while a round runs

- **WHEN** a person runs the archive command on a change whose pull request
  has a round running
- **THEN** the command is refused until the round ends, exactly as before

### Requirement: Every re-check of a carried grant reads one rule

A program that re-checks a grant a workflow carried SHALL read the same rule
for what a standing grant is as the program that carried it.

**The re-check runs on every start, completions included.**

A round started by a review or CI completion on a pull request whose fix
label a workflow placed SHALL re-read the grant and the write access of the
person behind it.

Where none stands, the carried label SHALL be removed and the loop marked stalled.

Removing the standing instruction from the issue therefore stops a loop that is
already running.

Where the carry accepts a label as the grant for a station, every gate on
that station SHALL accept it too.

**Two copies of the rule is how it ships half.** #238 made the archive label
its own grant for the archive pull request's fix station in the carry, and the
gate that starts the round kept looking for the standing instruction alone.

The carry placed the label, and the gate refused the round it authorised.

#### Scenario: A bot starts a round on the archive pull request

- **WHEN** a workflow starts a round on the archive pull request of an issue
  carrying the archive grant and no standing instruction
- **THEN** the gate accepts it, exactly as the carry that placed the label did

#### Scenario: A bot starts a round on an ordinary pull request under the archive grant

- **WHEN** a workflow starts a round on a pull request that merely proposes or
  applies a change whose issue carries the archive grant alone
- **THEN** the gate refuses it, since that grant is for the archive station
  only

#### Scenario: The standing instruction is removed while a loop runs

- **WHEN** a review completes on a pull request carrying a workflow-placed fix
  label and its issue no longer carries a grant for that station
- **THEN** no round starts, the carried label is removed and the loop is
  marked stalled

### Requirement: The line's decisions are one machine that every program asks

The stations, the loops and every decision that moves them SHALL be defined
once, as pure functions over stated facts, and every workflow and script that
acts on the line SHALL ask that definition rather than restate it.

The transition tables SHALL be complete: every state paired with every event
either names a next state or is stated to leave the state alone, and an event
outside the vocabulary is an error.

A state label SHALL be written only by translating an event through the table,
never by naming the value.

**Unreadable facts fail closed.** Where the fire records or the open pull
requests cannot be read, the machine SHALL be told a session is at work.

#### Scenario: A caller names an event the table skips

- **WHEN** a green CI arrives for a pull request whose loop is running
- **THEN** the label is left as it is

#### Scenario: A caller tries to name a state

- **WHEN** the state writer is invoked with a state value instead of an event
- **THEN** it refuses and writes nothing

#### Scenario: The fire records cannot be read

- **WHEN** a person places the standing instruction and the comments cannot be
  read
- **THEN** no session starts

### Requirement: The archive station starts only from a finished change

The archive carry SHALL place the archive grant only when the merged pull
request belongs to a change whose every task is ticked. A merge of a proposal
or an applying pull request SHALL carry nothing.

A fire SHALL start the station the change is at, and SHALL start nothing while
a pull request from the change's branch is open.

#### Scenario: A proposal merges

- **WHEN** a pull request that only proposes a change merges and the change has
  unticked tasks
- **THEN** the archive grant is not placed and no archive session starts

#### Scenario: The instruction is placed on a finished change

- **WHEN** the standing instruction is placed on an issue whose change is
  finished and no archive fire is recorded
- **THEN** the archive station starts, not the implement station

#### Scenario: A session is already at work

- **WHEN** a pull request from the change's branch is open and the grant is
  placed again
- **THEN** no second session starts

### Requirement: A round that ends with a person's answer awaited is not running

A round that ends because nothing is left to fix while a thread or a dispute
waits for a person SHALL mark the loop stalled, never mergeable and never
running.

A review that opens a thread on a pull request marked mergeable SHALL move it
to stalled.

#### Scenario: A clean round with an open thread

- **WHEN** a round finds nothing accepted to fix and a thread is open
- **THEN** the loop is marked stalled
