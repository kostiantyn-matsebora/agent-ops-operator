## MODIFIED Requirements

### Requirement: A finding is fixed or disputed, never dropped

Under whole-pull-request approval, the fixing step SHALL either fix each item
or DISPUTE it with a stated reason, and SHALL NOT leave an item unaddressed.

**WHERE IT NEVERTHELESS SAID NOTHING, THE ROUND SHALL SAY SO RATHER THAN CALL
IT A DISPUTE.**

- An item the step's report never mentions SHALL be recorded as UNADDRESSED
  and SHALL NOT be reported as disputed.
- A round whose fixing step produced NO report at all SHALL end as its own
  stated outcome naming that the step returned nothing.
- An unaddressed item SHALL remain eligible for a later round rather than
  being treated as settled.

**A CLAIMED FIX THE PATCH DOES NOT EVIDENCE IS UNADDRESSED TOO.**

- An item the report names as fixed while the patch touches nothing that could
  fix it SHALL be recorded as unaddressed with that reason.
- It SHALL receive no reply in its thread, and it SHALL stay eligible.

A dispute is a statement the fixing step makes about a finding. A failed fix
is not one. Posting it as one asks a person to rule on a question nobody
raised.

Silence and refusal are different facts, and only one of them is a decision.
Reporting an item nobody looked at as disputed tells a reader the machine
considered their finding and declined it.

Observed while this capability's own change was under review: three findings
came back "disputed by the fixing step: not addressed by the fixing step" on a
pull request that changed no code.

The step had produced nothing, and an empty report was substituted before the
recording step ran.

**Observed on #259.** A CRD finding was reported fixed and the patch did not
touch the file. The landing step posted "reported fixed, but the patch does
not touch" as a dispute.

The loop then waited for a person to answer a question the fixing step never
asked.

A dispute SHALL be posted as a reply in the finding's thread, or — for an
analysis issue, which has no thread — as one comment on the pull request naming
the issue's key.

A disputed thread SHALL stay open, and the dispute SHALL NOT be recorded in
the analysis service. The person who approved the pull request SHALL be
mentioned in the round's summary for every dispute.

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

- **WHEN** a later round finds a thread already carrying a dispute with no
  person's comment after it
- **THEN** the thread is not disputed a second time and is not fixed, and it
  is counted as awaiting the person

#### Scenario: A disputed finding was answered

- **WHEN** a later round finds a thread carrying a dispute and a person's
  comment after it
- **THEN** the thread is on the work list again, the person's words with it

#### Scenario: The fixing step omits an item from its report

- **WHEN** a work item appears in neither the fixed nor the disputed list of a
  report that exists
- **THEN** it is recorded as unaddressed, is not described as disputed, and is
  eligible for a later round

#### Scenario: A claimed fix landed nothing

- **WHEN** the report names an item fixed and the patch touches nothing that
  could fix it
- **THEN** the item is recorded as unaddressed with that reason, its thread
  receives no reply, and it is eligible for the next round

#### Scenario: The fixing step produced no report

- **WHEN** no report was written by the fixing step at all
- **THEN** the round ends as its own outcome saying the step returned nothing,
  and no item is reported as disputed

### Requirement: The loop is bounded and every ending is summarised

Rounds on one pull request SHALL be capped at a stated number.

A round that lands nothing SHALL start the next round while any item remains
eligible. It SHALL end the loop only when nothing eligible remains: every
remaining item disputed, or no item at all.

Every ending SHALL post ONE summary comment. It states what was fixed, what
was disputed, what was unaddressed, how many rounds ran, what remains open and
whether the head is red or green, and it mentions the approver.

**THE CAP SHALL BE A NAMED CONSTANT WITH A STATED DEFAULT OF FIVE**, declared
where the vocabulary the programs read is declared rather than buried in a
workflow.

A stated label SHALL grant another set of rounds and SHALL be REMOVED as it is
taken, so that continuing is always a fresh decision made knowing what the
last rounds produced.

The summary of an ending on the cap SHALL name that label as the way to grant
more.

The review's verdicts vary between runs of the same file, and the reviewer
reviews the fixer's own commits. Without a bound, a self-reviewing loop can
oscillate indefinitely at a cost nobody approved.

Five rather than three because a bound of three was measured stopping a loop
mid-progress: four rounds produced fourteen, ten, eight and one finding,
nearly all of them new each round.

**A round that changed nothing used to end the loop.** That read a failed
fix as a decision. The bound is what limits a fixer that cannot land the same
fix, and it is generous for that reason.

#### Scenario: No finding remains

- **WHEN** a round's review posts no finding, the analysis reports no open
  issue and no required check is red
- **THEN** the loop ends and the summary says the pull request is clean

#### Scenario: Only disputes remain

- **WHEN** a round fixes nothing and every remaining item carries a dispute
  the fixing step made
- **THEN** the loop ends waiting, the summary lists each dispute and says
  whether the head is red or green, and the approver is mentioned

#### Scenario: Nothing landed and items remain eligible

- **WHEN** a round fixes nothing and at least one item is unaddressed
- **THEN** the round counts, the summary lists the unaddressed items, and the
  next round starts on its own within the bound

#### Scenario: The cap is reached

- **WHEN** the stated number of rounds has run and findings remain
- **THEN** no further round starts, and the summary lists what remains, names
  the label that grants another set, and mentions the approver

#### Scenario: A round's patch is stale

- **WHEN** the branch moved between collection and landing so the patch does
  not apply
- **THEN** the round lands nothing, the summary says so, and the next round
  starts on its own within the bound

#### Scenario: Another set of rounds is granted

- **WHEN** a person places the extending label on a pull request whose loop
  ended on the cap
- **THEN** the loop runs another set of rounds, and the label is removed as it
  is taken

### Requirement: The work list of an approved pull request includes the analysis service's issues

On a labelled pull request the dispatch's work list SHALL include three
sources, each collected by a program:

- the open review threads
- the open issues the code-quality analysis reports for that pull request,
  from the service's API, per component project
- every required check that failed on the pull request's head, from the
  checks API, each carrying the job's name, a link to its run and a bounded
  tail of its failed steps' log

The model SHALL NOT read any of the three APIs.

The checks SHALL be reported consulted only when every required check run on
the head has completed. A run in progress SHALL be reported as not consulted,
and the summary SHALL say so rather than imply green.

An issue the analysis raised is a finding by another reviewer, and a failed
check is a finding by a third. A loop that fixed one reviewer's findings while
another's held the merge would end with the pull request still blocked and
nobody told why.

"Approved for fixing as a whole" means a pull request that merges, not one
whose threads are closed.

A check item SHALL be fixed only after the fixer has reproduced the failure
with the job's own command and re-run it on the patched tree. A failure the
tree does not explain SHALL be disputed, naming what the log said.

#### Scenario: The analysis reports issues on a labelled pull request

- **WHEN** the analysis has open issues for the pull request's components
- **THEN** each is an item in the dispatch's work list, carrying the issue's
  key, rule, file and line

#### Scenario: The analysis has not yet reported

- **WHEN** a dispatch collects while the analysis for the head sha is absent
- **THEN** the round proceeds over the review threads alone and the summary
  says the analysis was not consulted

#### Scenario: The checks have not yet concluded

- **WHEN** a dispatch collects while a required check run on the head is in
  progress
- **THEN** the checks are reported as not consulted, and the summary says so

#### Scenario: An issue was fixed

- **WHEN** a landed fix removes the code an issue pointed at
- **THEN** the dispatch does not change the issue's state in the analysis
  service, and the service's next analysis closes it

#### Scenario: A required check failed on the head

- **WHEN** a dispatch collects on a labelled pull request whose head has a
  failed required check
- **THEN** that check is an item in the work list, carrying its job name, run
  link and log tail, and the fixer reproduces it before fixing it

#### Scenario: A check failed for a reason not in the tree

- **WHEN** the fixer reproduces a failed check and finds the tree is not its
  cause
- **THEN** the check is disputed with one pull request comment naming the job
  and the log's reason, the code is untouched, and the summary mentions the
  approver

#### Scenario: A check was fixed

- **WHEN** a landed fix makes a failed check pass
- **THEN** no reply is posted for it, and the check's next run on the landed
  commit is its verdict
