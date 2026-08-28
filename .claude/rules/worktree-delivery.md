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
the merge. **The review is four jobs and two roles** — `queue` is a program
(`review-input.py` over `review-queue.py`), `read` is a matrix of one
`component-reviewer` job per changed component, all at once, `consolidate`
runs the `review-coordinator` — the only role that posts — over the readings'
artifacts and RECORDS each addressed thread with `mark-thread-resolved.sh` (a
list, no privilege), and `reconcile` RESOLVES the recorded threads with no
model and the one `contents: write`. It runs by hand
too: `gh workflow run claude-review.yml -f number=<pr>` (`-f dry_run=true`
posts nothing). NOTHING THE REVIEW RUNS COMES FROM THE PULL REQUEST: `queue`
restores `review-input.py`, `review-queue.py` and `components.sh` from the
BASE branch before building the queue; `read` restores the composite action,
`review-prompt.py` and `review-reading-check.py`, and `consolidate` the
action, `review-prompt.py` and `mark-thread-resolved.sh`; both then install
the CLI through `.github/actions/claude-cli`, which restores the job's role
file; `reconcile` checks out the base branch itself. So a pull
request cannot rewrite the review that judges it, shrink its own queue, or
resolve a thread it did not earn. The
fan-out is the matrix and not a pool inside one session: `gotchas.md` has the
measurement. Triage happens IN THE THREAD, in a stated vocabulary, and one
comment acts on everything accepted:

| You type | Where | It means |
|---|---|---|
| `fix it` (or another phrase from `.github/review-triage.json`) | a reply in the finding's thread | ACCEPTED — the next dispatch fixes it |
| anything else, or nothing | the thread | not accepted; the thread and the code stay as they are |
| `/fix-accepted` | a comment on the pull request | DISPATCH — one run, one commit, over everything accepted |
| resolve the thread yourself | the thread | dismissed; the review counts it and does not raise it again |

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
- **AN UNTRIAGED FINDING KEEPS ITS THREAD OPEN, AND THE MERGE BLOCKED.** That
  is the feature: a finding nobody accepted and nobody dismissed is a decision
  still owed.
- **THE LANDED COMMIT HAS NO CI AND NO REVIEW UNTIL YOU PUSH AGAIN.** A push
  made with the workflow token starts no workflow — GitHub withholds
  `synchronize` from it — so the dispatch says so on the pull request. An empty
  commit is enough.

### WHAT THE MAIN CHECKOUT IS STILL FOR

Reading, reviewing, and work with no change behind it — a typo, a broken link.

- **A `PreToolUse` hook refuses a commit there that belongs to a change owning a
  branch**, and FAILS OPEN on anything it cannot read. A hook that blocks work it
  does not understand gets disabled, and then it enforces nothing.
- **The CI check is what makes failing open safe**, by asserting the same
  decision where it cannot be skipped.
