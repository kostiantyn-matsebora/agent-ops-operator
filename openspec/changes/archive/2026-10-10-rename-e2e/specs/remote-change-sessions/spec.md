## MODIFIED Requirements

### Requirement: The session delivers the change through the existing loop

**CORRECTED BY `conveyor-labels`** — this requirement, and its scenarios below,
originally said the session opens its pull request "carrying the approve label
for automatic fixing from creation."

A session acts as an application with no write access, so a label it places on
its own work is refused and removed by the fixing loop's own gate — measured
live on #201.

The text below is the corrected version: the session places NO label, ever,
and a WORKFLOW carries the issue's standing instruction forward once the pull
request exists.

The remote session SHALL deliver the change as one pull request from
`change/<name>` REFERENCING the issue without a closing keyword, carrying NO
label, with the unit and chart tiers run in the session and the cluster tier
dispatched to the smoke system workflow on its branch.

Nothing the session does SHALL merge, or place a label. Nothing the IMPLEMENT
station does SHALL archive.

**The label on the issue is the owner's word, given once**, and a WORKFLOW —
never the session — carries it to the pull request as the consent the fixing
loop already reads.

That consent stands over everything that holds the merge: the review's
findings, the analysis service's issues and the failed required checks. What
that loop cannot settle — a dispute, an unanswered gate — waits for a
person, as it does today.

The session SHALL NOT wait for the checks or the review before ending. The
loop owns the pull request from the moment its label is carried to it.

The ARCHIVE station's session SHALL archive the change on its branch, open the
archive pull request CLOSING the tracking issue with no label, and stop.

That pull request is carried the same consent. The loop drives it to
mergeable, and a person merges it.

#### Scenario: The session opens the pull request

- **WHEN** the remote session finishes implementing the change
- **THEN** one pull request exists from `change/<name>`, it REFERENCES the
  issue without closing it — the issue it promoted is now the change's tracking
  issue, and that closes when the change is ARCHIVED — it carries NO LABEL, and
  its description states which verifications were run here and which are
  workstation-only

#### Scenario: A workflow carries the owner's standing instruction to the pull request

- **WHEN** the issue this pull request references still carries the owner's
  standing instruction for automatic fixing
- **THEN** a workflow places the approve label on the pull request, recording
  whose instruction authorised it — never the session

#### Scenario: The review finds something

- **WHEN** the review posts findings on that pull request AND it carries the
  approve label
- **THEN** the fixing loop fixes or disputes them under the label, and no
  person is asked to reply in a thread first

#### Scenario: A required check fails on the pull request

- **WHEN** a required check fails on the pull request's head AND it carries
  the approve label
- **THEN** the fixing loop starts a round over it without a person, and the
  check is fixed and re-run or disputed with the log's reason

#### Scenario: The loop ends

- **WHEN** the fixing loop posts its summary
- **THEN** the pull request waits for a person to merge, and after the merge
  the archive station runs under the standing instruction, or a person
  archives on the branch where no instruction stands

#### Scenario: The archive session opens its pull request

- **WHEN** the archive station's session finishes archiving the change
- **THEN** one pull request exists from `change/<name>`, closing the tracking
  issue, carrying no label, and a workflow carries the approve label onto it
  as it did the first one
