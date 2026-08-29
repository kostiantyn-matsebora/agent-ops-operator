## ADDED Requirements

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

## MODIFIED Requirements

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
