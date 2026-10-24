## Worktree delivery (how a change reaches master)

**ONE SESSION, ONE CHANGE, ONE WORKTREE, ONE BRANCH, ONE PULL REQUEST.**

A session named `<phase> <change>` (`session-naming.md`) OWNS a working
copy — own HEAD, own files, own index. Sharing one checkout once let a
cleanup in one session delete another's unstaged work.

A remote session's clone IS that same isolation, in place on
`change/<name>`. NEVER `git worktree add` there (`remote-session.md`).

```
../agent-ops-worktrees/<change>/        branch: change/<change>
```

A worktree INSIDE the repo silently breaks `components.sh`'s component
count and `structure.md`'s one-`docs/` test. `.gitignore` doesn't help.

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

**THE MERGE IS A SQUASH. THE PR TITLE BECOMES THE COMMIT SUBJECT** (CI checks
the commit-convention format) — one change, one `git log --oneline` line.
`change/<name>` is the only record of which openspec change a PR belongs to.

**ARCHIVE INSIDE THE PULL REQUEST, ON THE BRANCH, BEFORE MERGE** — so the diff
shows the contract changing.

The archive PR must say `Closes #<n>`. Every earlier PR of that change
must say `Refs #<n>` (a `Closes` on a proposing/applying PR is refused).
`/opsx:archive` is refused while a round is running or a dispute is
unanswered.

**BRANCH PROTECTION'S ONLY STATUS CHECK IS `ci-green`.** Every real gate
reports through its `needs:`. The one exception is conversation
resolution — evaluated LIVE at merge time, not a job, so an open review
thread blocks the merge without racing replies.

### The review and the fixing loop

`claude-review.yml` posts findings as threads. An open one blocks the merge.

`queue` marks each path READ/CARRIED. `read` builds and reads each
component blind, one `claude -p` per file. `consolidate` (the only poster)
folds blind findings against open/dismissed/fixed threads. `reconcile`
resolves recorded threads, failing on nothing else. Nothing the review
runs comes from the PR — scripts/prompts restore from the BASE branch.

| You type | Where | Means |
|---|---|---|
| `fix it` (or `.github/review-triage.json`) | a finding's thread | ACCEPTED |
| `/fix-accepted` | a PR comment | DISPATCH — fixes everything accepted |
| resolve it yourself | the thread | dismissed, not raised again |
| `conveyor:fix` | a PR | fix every finding/Sonar issue/failed check, round after round |
| `conveyor:run` | an issue | STANDING: implement → mergeable → archive |
| `conveyor:implement` / `conveyor:archive` | issue | one station each |
| `station:<x>` / `loop:<x>` | issue/PR | STATE only, set by the workflow |
| `conveyor:keep-going` | a capped PR | one more set of rounds |

**A PROGRAM MAY CARRY A GRANT FORWARD OR CONSUME ONE.

IT MAY NEVER MINT ONE.** A label is placed by a write-access person, or
CARRIED by `carry.py`, re-checked at every start (#201). A session opens
its own PRs unlabelled.

Dispatch is a trigger, not an instruction — the work list comes from the
threads, and only write access may dispatch.

The model never pushes — a model-free job applies the patch via a scoped
deploy key, resolving threads only where it landed. New files land only
where the report declares them (#243).

Every failed required check is a work item (read from `ci-green`'s
`needs:`). The fixer reproduces and re-runs each before patching, disputing
what the tree doesn't explain, and re-runs an unexplained red every round
the budget allows before asking anyone.

`max_rounds` (default 5) is bounded, a round changing nothing ends it
early. A carried round runs as `github-actions` only.

A person's comment — never a check — restarts a waiting round, and a
15-minute sweep catches a resolved thread (fires no event).

A failed fix is UNADDRESSED, not a dispute. SonarCloud is the second
required reviewer. `/opsx:archive` is refused while any of this is open.

### `conveyor.py` decides. Nothing else does

One pure function, facts in, a `Decision` out — ten places once disagreed
(#248/254/255) with the rule copied into several scripts.

| Decision | Asked by |
|---|---|
| `standing_grant`, `carry_fix`, `carry_archive` | `carry.py` |
| `fire` | `remote-implement.py` |
| `gate`, `round_may_start` | `dispatch-gate.py` |
| `ending`, `check_is_work`, `cap_for` | `land-dispatch.py`, `failed-checks.py` |
| `guard` | the archive hook |

State labels are written only by the event that earns them, never a
caller-supplied value. A table test walks every state against every event.

`conveyor:run` stands for every station, and `conveyor:archive` also stands
for the fix station of a `Closes #<n>` PR. Unreadable facts fail CLOSED
(told a session is at work).

### The main checkout

Still for reading, reviewing, and work with no change behind it. A
`PreToolUse` hook refuses a commit there that belongs to a branch-owning
change, failing OPEN on anything unreadable — CI asserts the same decision
where the hook can't be skipped.
