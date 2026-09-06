## MODIFIED Requirements

### Requirement: The work list of an approved pull request includes the analysis service's issues

On a labelled pull request the dispatch's work list SHALL include the open
issues the code-quality analysis reports for that pull request, collected by a
program from the service's API, per component project, beside the open review
threads — AND every required check that failed on the pull request's head,
collected by a program from the checks API, each carrying the job's name, a
link to its run and a bounded tail of its failed steps' log. The model SHALL
NOT read any of the three — the review threads' API, the analysis service's
or the checks'.

An issue the analysis raised is a finding by another reviewer, and a failed
check is a finding by a third; a loop that fixed one reviewer's findings while
another's held the merge would end with the pull request still blocked and
nobody told why. "Approved for fixing as a whole" means a pull request that
merges, not one whose threads are closed.

A check item SHALL be fixed only after the fixer has reproduced the failure
with the job's own command and re-run it on the patched tree; a failure the
tree does not explain SHALL be disputed, naming what the log said.

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
- **THEN** no reply is posted for it; the check's next run on the landed
  commit is its verdict

### Requirement: A landed fix on an approved pull request starts the next round

When a dispatch lands on a labelled pull request, the review and the
continuous-integration checks SHALL run on the landed commit without a person
pushing, and their findings SHALL start the next round. A round SHALL start
when the review completes on a labelled pull request AND when the
continuous-integration run completes with a failure on one; two starts for one
head SHALL run in sequence, each over the pull request's current state, and
both SHALL count toward the loop's bound.

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

#### Scenario: The checks fail on a labelled pull request

- **WHEN** the continuous-integration run on a labelled pull request's head
  completes with a failure
- **THEN** a round starts with the failed checks on its work list, whether or
  not the review posted anything

#### Scenario: The review and the checks both complete on one head

- **WHEN** the review completes and the checks fail on the same head
- **THEN** the two rounds run one after the other, the second collecting what
  the first left, and both count toward the bound
