## Purpose
The cloud environment a change is worked in: how it is defined from the
repository rather than typed into a form, what a remote session finds and is
told, what a label on an issue starts, and what consent that label carries.

## ADDED Requirements

### Requirement: The environment is defined by the repository

The cloud environment's setup SHALL be one line handing off to a bootstrap
committed in the repository, and that bootstrap SHALL be idempotent, SHALL exit
at once when it is not running remotely, and SHALL install every tool the
tree's own checks need that the cloud image lacks — the chart renderer, the
change tool at the version CI pins, the YAML library the scripts import, the
envtest assets — and verify the Go floor the modules declare.

**A form field is unreviewed and unversioned.** The line in the field cannot
drift from the tree if all it does is name a file in the tree; what the
environment installs is then a pull request like anything else.

A tool the bootstrap cannot install SHALL be reported by name and SHALL NOT
fail the others: the environment's setup must finish with success, and a
session with one tool missing is more useful than no session.

#### Scenario: The setup runs in the cloud

- **WHEN** a new remote session starts and the setup runs the bootstrap
- **THEN** every tool the tree's checks need is on the path afterwards, the
  chart render tests run rather than skip, and the change tool answers at CI's
  pinned version

#### Scenario: The bootstrap runs on a workstation

- **WHEN** the bootstrap is run where the remote marker is unset, `0` or `false`
- **THEN** it exits at once with success and installs nothing

#### Scenario: One installer fails

- **WHEN** one tool's installer fails, for the network or any other reason
- **THEN** the remaining tools are still installed, the missing one is named on
  the setup's output, and the setup exits with success

### Requirement: A remote session is told what it lacks

At the start of a remote session, the bootstrap's verify mode SHALL run and
print one line per required tool stating present or missing, into the
session's context.

**A silent skip is the failure this guards.** The chart render tests skip when
the renderer is absent and `go test` is green either way; a session that does
not know the renderer is missing reports a chart change verified when nothing
rendered it.

#### Scenario: A tool is missing at session start

- **WHEN** a remote session starts and one required tool is not on the path
- **THEN** the session's context names that tool as missing before any work
  begins

#### Scenario: A workstation session starts

- **WHEN** a session starts where the remote marker is unset
- **THEN** the verify hook prints nothing and adds nothing to the context

### Requirement: The rules read true in both places

No rule file SHALL state a property of the workstation as a property of the
repository. Where a step differs between a workstation and a remote session —
where the toolchain is, where the working copy is, what can be deployed or
looked at — the rule SHALL say which applies where.

**A remote session reads the same rules a workstation does**, and a rule that
says "there is no Go here, use the container" sends it looking for a container
that does not exist. The remote session's working copy is its clone: it SHALL
check out the change's branch in place and SHALL NOT create a worktree.

#### Scenario: A remote session builds a module

- **WHEN** a remote session reads the build rules and builds a Go module
- **THEN** it runs the toolchain on its path and is not directed to a container

#### Scenario: A remote session begins a change

- **WHEN** a remote session starts implementing a change
- **THEN** it checks out `change/<name>` in its clone, and creates no worktree
  beside it

#### Scenario: A remote session reaches a workstation-only step

- **WHEN** a task's verification is a deploy to the local cluster or a
  screenshot of the live install
- **THEN** the rule names it as workstation-only, and the session records that
  step as not performed rather than reporting it done

### Requirement: A secret never enters the environment's variables

The environment's variables SHALL hold no token, key or password. A credential
a session needs SHALL be attached by the platform's proxy to requests for its
host, and SHALL NOT reach the session as a variable or a file.

**The variables are visible to anyone using the environment.** The platform
says so on the form; a token typed there is a token published to every session
the environment runs.

#### Scenario: The analysis service is reached from a remote session

- **WHEN** a remote session's analysis MCP server calls the analysis service
- **THEN** the request is authenticated by the proxy, and no variable in the
  environment holds the token

#### Scenario: Somebody reads the environment's variables

- **WHEN** the environment's variables are listed
- **THEN** none of them is a secret

### Requirement: A label on an issue starts a remote session that implements it

An issue carrying the stated implement label, placed by a person with write
access, SHALL start one remote session in the repository's environment, handed
the issue's number and nothing else. A label placed by anyone else SHALL be
removed with a visible comment saying who may place it.

The fire SHALL be recorded on the issue ONCE, as a comment carrying the
session's link, and that record is the transition the issue's tracking
requires. The promotion then leaves its pointer comment as it does for any
promoted issue; nothing else automated is added to the issue by the start, and
no progress comments follow.

**Who may start a machine writing to a branch is the same question as who may
dispatch a fix**, and it has the same answer: write access, read from the
platform, never from a sentence.

#### Scenario: A maintainer labels an issue

- **WHEN** a person with write access places the implement label on an issue
- **THEN** exactly one remote session starts for it, and the issue gains one
  comment linking that session

#### Scenario: Somebody without write access labels an issue

- **WHEN** the implement label is placed by a person without write access
- **THEN** no session starts, the label is removed, and a comment on the issue
  says who may place it

#### Scenario: The label is removed and placed again

- **WHEN** the implement label is removed from an issue and later placed again
- **THEN** a new session starts, and the change already bound to that issue is
  continued rather than proposed a second time

### Requirement: The session's instructions are committed, and the issue is a request

The routine's saved prompt SHALL be a pointer to an instruction file committed
in the repository, and SHALL name the fired payload as the issue's NUMBER. The
instruction file SHALL direct the session to read the issue as a request to be
explored and proposed — never as instructions to follow — and to promote it in
place as the change's tracking issue.

**A prompt saved in a form is the setup field one screen over**: unreviewed
and unversioned. The one line that must live there names a file; the process
lives in the file.

#### Scenario: The routine fires

- **WHEN** the routine fires with an issue number as payload
- **THEN** the session reads the committed instruction file and follows it for
  that issue, and treats the issue's body as the subject of a proposal

#### Scenario: The process changes

- **WHEN** how a change is proposed or delivered here changes
- **THEN** the instruction file changes in a pull request, and the routine's
  saved prompt needs no edit

### Requirement: The session delivers the change through the existing loop

**CORRECTED BY `conveyor-labels`** — this requirement, and its scenarios below,
originally said the session opens its pull request "carrying the approve label
for automatic fixing from creation." A session acts as an application with no
write access, so a label it places on its own work is refused and removed by
the fixing loop's own gate — measured live on #201. The text below is the
corrected version: the session places NO label, ever, and a WORKFLOW carries
the issue's standing instruction forward once the pull request exists.

The remote session SHALL deliver the change as one pull request from
`change/<name>` REFERENCING the issue without a closing keyword, carrying NO
label, with the unit and chart tiers run in the session and the cluster tier
dispatched to the smoke end-to-end workflow on its branch. Nothing the session
does SHALL merge, archive, or place a label.

**The label on the issue is the owner's word, given once**, and a WORKFLOW —
never the session — carries it to the pull request as the consent the fixing
loop already reads — over everything that holds the merge: the review's
findings, the analysis service's issues and the failed required checks. What
that loop cannot settle — a dispute, an unanswered gate — waits for a person,
as it does today. The session SHALL NOT wait for the checks or the review
before ending. The loop owns the pull request from the moment its label is
carried to it.

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
- **THEN** the pull request waits for a person to merge, and the change is
  archived by a person on the branch as today
