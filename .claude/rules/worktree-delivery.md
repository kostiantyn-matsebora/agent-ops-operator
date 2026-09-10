## Worktree delivery (how a change reaches master)

**ONE SESSION, ONE CHANGE, ONE WORKTREE, ONE BRANCH, ONE PULL REQUEST.**

`session-naming.md` already names a session `<phase> <change>`. This is the
other half: the session also OWNS a working copy, and that is what makes
branching safe here.

### THE SHARED WORKING COPY WAS THE DEFECT. IT COST WORK TWICE

Several sessions run this repository at once. While they shared one checkout
they shared one HEAD **and one set of files**:

| Date | What happened |
|---|---|
| 2026-08-23 | a session created a branch, a concurrent session's commit landed on it, the branches diverged, `--ff-only` refused, the fix was cherry-picked back |
| 2026-08-24 | a session cleaned the tree and **deleted another session's entire unstaged change directory** — four artifacts, no branch and no stash to recover from |

- **The old rule — commit straight to `master`, never branch — contained the
  first and not the second.** It is RETIRED, and the history is kept because the
  argument was correct under the conditions that produced it: anyone who meets
  one shared checkout again will re-derive it.
- **A worktree removes the CAUSE.** Own HEAD, own files, own index.

### A REMOTE SESSION'S CLONE IS THE OTHER WORKING COPY

**A change is worked in a working copy dedicated to it, and there are exactly
two shapes of that** — a worktree on this workstation, or a cloud session's own
clone with `change/<name>` checked out IN PLACE.

- **The clone IS the isolation the worktree rule was written to create.** Own
  HEAD, own files, own index, by construction.
- **NEVER `git worktree add` in a remote session.** A second copy of the tree
  there breaks the two readers below for no isolation it did not already have.
- **`.claude/rules/remote-session.md` owns the rest** — what that environment
  installs, what a label on an issue starts, and what stays workstation-only.

### PLACEMENT IS FORCED, NOT PREFERRED

```
../agent-ops-worktrees/<change>/        branch: change/<change>
```

**A worktree INSIDE the repository breaks two things that read the tree, and
both fail silently:**

| Reader | Breaks how |
|---|---|
| `.github/components.sh` | it finds components with `find . -name go.mod -mindepth 2` and `find . -name Dockerfile`, so a second copy reports **26** components and hands every CI matrix twice the work, publishing under names that describe nothing |
| `structure.md`'s standing test | `find . -type d -name docs` must return ONE line |

- **That test needs `-not -path '*/node_modules/*'` to be run honestly**, exactly
  as `components.sh` does. An installed console tree carries five vendored
  `docs/` directories, so the bare command already returns six and a worktree
  would be lost in the noise rather than caught by it.
- **`.gitignore` does not help.** `find` does not read it. An ignore entry makes
  the copy invisible to `git status` and leaves it visible to everything that
  actually breaks.
- **The tell is a matrix that grew**, which reads as a new component rather than
  as a working copy.

### THE COMMANDS OWN THE LIFECYCLE

| Command | Does |
|---|---|
| `/opsx:propose` | opens the tracking issue, writes the binding |
| `/opsx:apply` | creates the worktree and branch if absent, advances the label |
| `/opsx:archive` | archives INSIDE the pull request, closes the issue, removes the worktree |

```sh
git worktree add -b change/<name> ../agent-ops-worktrees/<name> origin/master
git worktree remove ../agent-ops-worktrees/<name>      # after archive
git worktree list                                      # what is in flight
```

### THE MERGE IS A SQUASH, AND THE TITLE IS THE SUBJECT

**One change is ONE line of `git log --oneline`**, which `CONTRIBUTING.md` says
is meant to read as an account of the project.

- **The pull request TITLE becomes the commit subject**, so it obeys the commit
  convention — `type(scope): what it does, as a sentence`. A CI check enforces
  that, because a title is the one field nobody proofreads.
- **A merge commit adds a line describing nothing.** A rebase of the working
  history adds twenty describing the writing rather than the change.

### THE BRANCH IS THE STATEMENT OF PROVENANCE

`change/<name>` names the change, so **nothing asks for it again**. The pull
request template does not carry a "which openspec change" field, and anything
needing the name reads the branch.

### ARCHIVE INSIDE THE PULL REQUEST

**UNCHANGED BY EVERY LOOP ABOVE, AND STATED SO.** Neither the fixing loop nor a
remote session archives anything: a machine may write to the branch, and a
person merges and archives. `/opsx:archive` is refused while a round is running
or a dispute is unanswered.

`openspec archive` folds the delta specs into `openspec/specs/`, so doing it on
the branch means **the diff shows the contract changing** — which is what a
reviewer of this project should be looking at, since `openspec/specs/` is the
answer to "is this behaviour intended".

- **Archiving on `master` after the merge was rejected**: the spec change would
  reach the published contract reviewed by nobody, and it puts direct commits
  back on the default branch in the one change meant to stop them.
- **Cost, accepted:** an archive commit that must reflect review feedback needs
  an amend or a follow-up commit on the branch.
- **THE ARCHIVE PULL REQUEST MUST SAY `Closes #<n>`**, and the `pr-closes` check
  refuses one that does not. GitHub closes on the keyword and on nothing else;
  the script's `close` is a second step, and #38 and #67 stayed open under
  `opsx:archived` because both were skipped.
- **AND EVERY EARLIER PULL REQUEST OF THAT CHANGE MUST SAY `Refs #<n>`.** The
  same check refuses a `Closes` that would end a tracking issue whose change is
  merely being PROPOSED or APPLIED — the issue has to follow the change through
  review and archiving, and merging is not where it ends. The two halves are one
  rule read from both sides, and typing `Closes` out of habit on an applying
  pull request is how it is broken: #198 did exactly that and the check caught
  it.

### THE GATES ARE ALREADY REQUIRED — THROUGH ONE CHECK

Branch protection names **`ci-green`** as its only status check. Every real gate
reports through it, so **adding a required gate is a line in that job's
`needs:`**, never a settings change somebody has to remember.

That is what `continuous-integration`'s always-present-check requirement is for:
a protection rule names a check by NAME, and a job skipped for untouched paths
never reports that name.

**THE ONE GATE NOT REPORTED THROUGH IT IS CONVERSATION RESOLUTION**, and it is
not an inconsistency to fix. Branch protection ALSO requires every review thread
resolved before merging (`required_conversation_resolution`, verified live on
2026-08-26). It cannot be a job in `needs:` because it is not a check at all —
it is a property of the pull request, evaluated by GitHub at merge time, and a
job asserting it would race every reply. It is what makes an untriaged review
finding block the merge, below.

### THE REVIEW FOUND SOMETHING. NOW WHAT

`claude-review.yml` posts findings as review threads, and an open thread blocks
the merge. **The review is four jobs and three roles** — `queue` is a program
(`review-input.py` over `review-queue.py`) that also decides, per changed
path, READ or CARRIED (below). `read` is a matrix of one job per component
with a read path, all at once. EACH JOB BUILDS ITS COMPONENT FIRST —
`review-build.sh`, the SAME RECIPE CI uses, no credential in that step's env —
and a failed build SKIPS EVERY READER: the job writes the reading itself,
`unbuilt` with the build's own tail, and no model runs. A built component is
then read ONE `claude -p` PROCESS PER FILE, blind — no thread, no previous
finding — several at once from the shell (`xargs -P $REVIEW_READERS`), each
holding the `file-reviewer` role and the rules `review-rules.py` routes to
the component's paths as a SHARED SYSTEM PREFIX (`review-prompt.py
reader-system`) — the same bytes for every file, paid once and served from
cache after. NO CONTEXT INHERITS A RULE FILE, and `review-context.py` prints
what each one holds at the top of the job. A file carrying an UNRESOLVED
THREAD also gets a second, primed process — the `thread-verdict` role, no
rules — judging `fixed`/`standing`/`gone`/`detached` for that thread alone; a
`detached` verdict's relocated finding folds into the file's findings.
`review-reading-check.py` merges a job's file and verdict readings into the
component's (or, unbuilt, passes through the build step's own reading).
`consolidate` runs the `review-coordinator` — the only role that posts,
holding the readings and threads and no rules — which is where "not made
again" now lives: a blind finding matching an OPEN thread is folded in
(`carried over`), one matching a DISMISSED thread is dropped (`dismissed`),
one matching a thread judged `fixed` is posted (the fix did not hold). It
cross-reviews from what the files declare and reference, never re-reads the
diff, and posts everything through ONE command, `review-post.py`, which
RECORDS each addressed thread with `mark-thread-resolved.sh` (a list, no
privilege) and APPENDS THE COVERAGE MARKER — a hidden HTML comment on the
summary, `{"sha", "paths": {path: {"quiet": N}}}` — so the NEXT run knows
what it read and how many consecutive reads found nothing new. The model and
its effort are the workflow's (`--model`, `--effort` — `gotchas.md` has why);
`reconcile` RESOLVES the recorded threads with no model and the one
`contents: write`. It runs by hand too: `gh workflow run claude-review.yml -f
number=<pr>` (`-f dry_run=true` posts nothing; `-f full=true` ignores the
coverage record and reads every changed path from the base — the same
override `REVIEW_QUIET_READS`, a workflow variable, tunes: how many
consecutive quiet reads of an unchanged path earn a CARRY, default one).
NOTHING THE REVIEW RUNS COMES FROM THE PULL REQUEST: `queue` restores
`review-input.py`, `review-queue.py` and `components.sh` from the BASE
branch before building the queue; `read` restores the composite action,
`review-build.sh`, `review-prompt.py`, `review-reading-check.py`,
`review-rules.py`, `review-context.py` and `review-trace.py`, and
`consolidate` the action, `review-prompt.py`, `review-reading-check.py`,
`review-rules.py`, `review-context.py`, `review-trace.py`, `review-post.py`
and `mark-thread-resolved.sh`; both then install the CLI through
`.github/actions/claude-cli`, which restores the job's role files (`read`
restores `file-reviewer.md` AND `thread-verdict.md`); `reconcile` checks out
the base branch itself. So a pull request cannot rewrite the review that
judges it, shrink its own queue, or resolve a thread it did not earn. The
fan-out is the matrix and not a pool inside one session: `gotchas.md` has the
measurement, and the per-file loop replaces the retired `review-component.js`
saved workflow entirely — the loop is the job's shell now, one level in from
where it was. A component the build step could not build is `unbuilt`,
DISTINCT from `unreviewed` (the job itself failed) and `unread` (a file's own
reader returned nothing) — three different gaps, in the summary's table by
name. Triage happens IN THE THREAD, in a stated vocabulary, and one
comment acts on everything accepted:

| You type | Where | It means |
|---|---|---|
| `fix it` (or another phrase from `.github/review-triage.json`) | a reply in the finding's thread | ACCEPTED — the next dispatch fixes it |
| anything else, or nothing | the thread | not accepted; the thread and the code stay as they are |
| `/fix-accepted` | a comment on the pull request | DISPATCH — one run, one commit, over everything accepted |
| resolve the thread yourself | the thread | dismissed; the review counts it and does not raise it again |
| the `conveyor:fix` LABEL | a pull request | APPROVED AS A WHOLE — every open finding, every open SonarCloud issue AND every FAILED REQUIRED CHECK on the head is fixed or DISPUTED by CI, round after round, no reply and no dispatch needed. Placed by a person with WRITE access, or CARRIED forward by a workflow relaying a `conveyor:run` instruction still standing on the tracking issue — never by a session |
| the `conveyor:implement` LABEL | an ISSUE | APPROVED TO BE BUILT, ONE STATION — a remote session proposes, implements and opens the pull request, UNLABELLED. Placed by a person with WRITE access; anyone else's is removed with a comment |
| the `conveyor:run` LABEL | an ISSUE | THE STANDING INSTRUCTION — implement, drive the pull request to mergeable, archive once merged: the whole line for that issue's LANE, read at every transition rather than recorded at the first. Removing it halts the line at the next station |
| the `conveyor:archive` LABEL | the tracking ISSUE of a MERGED pull request | ARCHIVE, ONE STATION — placed by a person with write access, or carried forward the same way `conveyor:fix` is. Only the opsx lane has this station; the plain lane's line ends at the merge |
| the `conveyor:keep-going` LABEL | a pull request whose loop stopped on the round cap | GRANTS ANOTHER SET of rounds, and is REMOVED the moment a round runs under it — one placement, one grant |
| a reply under `<!-- conveyor:disputed -->` | a thread (or a pull request comment, for a Sonar issue) | THE LOOP DISAGREES — the code is untouched, the thread stays open, you are mentioned. Answer it (a reply, or resolve to dismiss); nothing re-disputes it |

**A PROGRAM MAY CARRY A GRANT FORWARD OR CONSUME ONE. IT MAY NEVER MINT ONE.**
Every label above that authorises unattended work is placed by a person whose
write access the platform confirms — directly, or CARRIED forward by
`.github/scripts/carry-grant.py`, which records whose instruction it relayed
and is RE-CHECKED at the point it is acted on, never trusted because a
workflow placed it.

- **THIS IS THE WHOLE FIX FOR #201.** A remote session opened its pull request
  carrying `autofix` because its own instructions said to; it acts as an
  application with no write access, so the gate that checks who labelled
  removed the label and refused, and the pull request sat green, reviewed and
  unlabelled with no session left to act. The gate was right; the instruction
  was wrong. The session now opens its pull request with NO LABEL, and a
  workflow reads the issue's `conveyor:run` again once the pull request opens
  (or merges) and carries it forward.
- **THE BOUND IS A NAMED CONSTANT**, `max_rounds` in `.github/review-triage.json`,
  default 5 — read once by the gate and passed down, rather than a static
  workflow constant, because a workflow's `env:` block cannot read a file.

- **THE VOCABULARY IS A FILE, MATCHED BY A PROGRAM** —
  `.github/review-triage.json`, read by `accepted-findings.py`. Whole reply,
  trimmed, trailing punctuation dropped, case-insensitive. "Sure, if you think
  so" is not an acceptance, and neither is `fix it` inside a longer sentence:
  what decides that code is written to a branch may not be a judgement call.
- **THE DISPATCH IS A TRIGGER, NEVER AN INSTRUCTION.** The work list is derived
  by walking the threads; nothing reads the dispatch comment's text. A dispatch
  "asking for" work no thread accepted performs none of it.
- **WHO MAY DISPATCH: write access**, from the comment's own author
  association. A fork's pull request is refused. Both refusals are posted on
  the pull request, because a silent no-op reads as a broken bot.
- **THE MODEL CANNOT PUSH.** `review-dispatch.yml` produces the fix under
  `contents: read` as a patch artifact; a model-free job applies it, pushes,
  replies `Fixed in <sha>` in each fixed thread, and only then hands the ids to
  `resolve-review-threads.py`. A thread is resolved only where its patch landed;
  a stale patch pushes nothing, resolves nothing, and says so — rebase and
  dispatch again.
- **A RED `ci-green` STARTS A ROUND TOO, AND A FAILED CHECK IS A WORK ITEM.**
  Under the label, `review-dispatch.yml` also runs on a `ci` run that COMPLETED
  WITH `failure`, and `collect` reads the head's failed required checks
  (`failed-checks.py`, under `actions: read` — that job alone, where no model
  runs). Which checks count is read from `ci-green`'s own `needs:`, never
  restated; the review's jobs and `ci-green` itself are excluded, the one being
  another reviewer and the other the aggregate.
  - **The fixer REPRODUCES a check before fixing it**, with the job's own
    command, and re-runs it before the patch is cut. A failure the tree does
    not explain — an outage, a rate limit, a flake — is DISPUTED with the log's
    reason, because a false fix for a flake is worse than the flake.
  - **A fixed check gets no reply.** There is no thread to reply in, and the
    check's next run on the landed commit is its verdict; the round's summary
    is where it is accounted for.
  - **Two starts for one head run in SEQUENCE**, serialised by the existing
    `concurrency` group — the review's completion and CI's failure — each
    collecting the live state, both counting toward `max_rounds`
    (`.github/review-triage.json`).
- **AN UNTRIAGED FINDING KEEPS ITS THREAD OPEN, AND THE MERGE BLOCKED.** That
  is the feature: a finding nobody accepted and nobody dismissed is a decision
  still owed.
- **ON AN UNLABELLED PULL REQUEST THE LANDED COMMIT HAS NO CI AND NO REVIEW
  UNTIL YOU PUSH AGAIN.** A push made with the workflow token starts no
  `pull_request` workflow — GitHub withholds `synchronize` from it — so the
  dispatch says so on the pull request. An empty commit is enough.
- **ON A LABELLED ONE THE LOOP PUSHES FOR YOU, THROUGH A WRITE DEPLOY KEY.**
  `land` pushes over SSH with `AUTOFIX_DEPLOY_KEY` — repository-scoped,
  `contents` only, read by that model-free job and no other — so the commit
  is an ordinary push: `ci-green` and the review run on it, and the review's
  completion is the next round's trigger. Self-dispatching `ci.yml` was
  measured first and REJECTED: a `workflow_dispatch` run's check runs never
  reach the merge box (#131, `gotchas.md`). Without the secret the round
  lands with the token and the summary says the loop cannot go on.
- **THE LOOP IS BOUNDED AND EVERY ENDING IS ONE SUMMARY.** `max_rounds`
  (`.github/review-triage.json`, default 5) is read once by `review-dispatch.yml`'s
  gate and passed down, counted from the landing comments' `<!-- conveyor:round
  N -->` markers since the label was placed — so removing and re-adding the
  label starts the count afresh. A round that changes nothing (every item
  disputed, or a stale patch) ends it early. The summary names what was fixed,
  what was disputed, what was UNADDRESSED (an item the fixing step's report
  never named — worded as such, never as a dispute nobody made), the rounds
  used and the approver — and, at the cap, that `conveyor:keep-going` grants
  another set, consumed the moment a round runs under it.
- **SILENCE IS NOT A DECISION.** A round whose fixing step wrote NO report at
  all ends as its own outcome, `no report`, disputing nothing — distinct from
  a report that named every item disputed. Reading a missing report as "every
  item disputed" tells a person the machine considered each finding and
  declined it, when nobody looked; measured while this rule's own change was
  under review, where three findings on a proposal-only pull request came back
  "disputed... not addressed" and no model had spoken.
- **THE SECOND REVIEWER IS SONARCLOUD, AND ITS GATE IS REQUIRED.** `collect`
  reads its open issues per component project (`sonar-issues.py`, the token in
  the model-free job only) beside the threads; the scan step waits on the
  quality gate and fails the component's job on `ERROR`, so it reports through
  `ci-green`. The loop never marks anything in Sonar — a disputed issue is a
  comment for you, and the service's state is yours to change in its UI.
- **`/opsx:archive` IS REFUSED WHILE THE LOOP IS OPEN** — a round running, or
  a dispute no person has answered. `autofix-guard.py` (the script keeps its
  original filename; it reads `approve_label` — `conveyor:fix` — from the
  vocabulary file rather than a hardcoded name), in the same hook as
  the documentation gate and the same CI job; it fails open on anything it
  cannot read.

### WHAT THE MAIN CHECKOUT IS STILL FOR

Reading, reviewing, and work with no change behind it — a typo, a broken link.

- **A `PreToolUse` hook refuses a commit there that belongs to a change owning a
  branch**, and FAILS OPEN on anything it cannot read. A hook that blocks work it
  does not understand gets disabled, and then it enforces nothing.
- **The CI check is what makes failing open safe**, by asserting the same
  decision where it cannot be skipped.
