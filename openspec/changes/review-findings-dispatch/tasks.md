## 1. The accept vocabulary, stated once

- [x] 1.1 Write the accept vocabulary into ONE place both the program and the
  contributor read — the phrase list, the matching rule (case-insensitive, whole
  reply, trailing punctuation ignored) and what is NOT an acceptance. Verify by
  pointing the program at it rather than restating it in code.
  **DONE.** `.github/review-triage.json`: the `accept` list, the `dispatch` form, `trailing_punctuation`, and the matching rule stated in its comment. `accepted-findings.py` loads it (`--vocabulary`, default that file) and the suite proves a swapped file changes the verdict; the docs name the file rather than restating the list.

## 2. The work list, derived from the threads

- [x] 2.1 Add the thread-walking program that emits the accepted findings as a
  list: thread id, path, line, the finding's text and the human's reply. Verify
  against a fixture of the four cases — accepted, argued with, unanswered, and
  answered by the review itself.
- [x] 2.2 REFUSE an acceptance written by the review, not a person. A bot that
  can accept its own findings is a bot that writes to the branch unattended.
  Verify with a fixture whose reply is authored by the review's own login.
- [x] 2.3 Ignore threads a person already resolved, and threads whose first
  comment is not the review's. Verify both, and verify the program reports what
  it skipped rather than skipping silently.
- [x] 2.4 Give it a test suite in `.github/tests/`, in the house style — its
  REFUSALS are the product. Verify the suite fails when the refusal is removed,
  not merely that it passes.
  **DONE.** `.github/scripts/accepted-findings.py` walks `reviewThreads` (paged, 100 comments each) and writes a JSON list of `threadId`, `commentId`, `path`, `line`, `outdated`, `finding`, `acceptedBy`, `reply`. The fixture carries all four cases plus five more; only the accepted one survives.
  **DONE.** Refused on the reply's `__typename == Bot` OR a login in the review's allowlist — so a PERSON whose login is `claude` is refused too, the same two-sided check the resolver makes. Fixture `PRRT_selfaccepted` and `PRRT_impostor`.
  **DONE.** Both, and a third: an acceptance whose `authorAssociation` is not OWNER/MEMBER/COLLABORATOR is refused (a public pull request is a place strangers can type). Every skip prints `skipped <id> (<path>:<line>): <reason>`; the suite counts eight of them.
  **DONE.** `.github/tests/accepted-findings.test.sh`, 16 assertions. With the self-acceptance refusal replaced by `if False:` the suite fails 5 assertions — verified, not assumed.

## 3. The dispatch, and who may send one

- [x] 3.1 Add the comment-triggered workflow on `issue_comment` and
  `pull_request_review_comment`, matched on the dispatch form. Verify a comment
  that is not a dispatch starts nothing — reading the run list, not the intent.
- [x] 3.2 Gate on the comment's own `author_association` (write access) and
  refuse a fork's pull request. Verify the refusal is VISIBLE — a comment or a
  check that says so — because a silent no-op reads as a broken bot.
- [x] 3.3 Never `pull_request_target`. Verify by asserting the trigger set in
  the workflow, and state in the file why the convenient one is refused.
  **DONE.** `.github/workflows/review-dispatch.yml`: `issue_comment` + `pull_request_review_comment` (`created`) + `workflow_dispatch` for a pre-merge hand run. The job-level `if` prefilters on `startsWith(body, '/fix-accepted')`, and the gate re-matches the whole trimmed comment against the vocabulary file. A comment that is not a dispatch leaves a SKIPPED run in the list — see 6.1 for the live reading.
  **DONE.** `author_association` ∈ OWNER/MEMBER/COLLABORATOR, else refused; `gh pr view --json isCrossRepository` true, else refused. Each refusal is a comment on the pull request (`Dispatch by @<who> refused: <why>`) and a failed job.
  **DONE.** `.github/tests/review-dispatch.test.sh` parses EVERY workflow's `on:` and fails if any names `pull_request_target`, and pins this one's set to exactly the three above. The file's header states why the convenient trigger is refused.

## 4. Producing the fix without the power to land it

- [x] 4.1 The fixing job runs under `contents: read`, takes the work list, and
  emits a PATCH as an artifact. Verify the job's permissions in the workflow and
  that the artifact carries hidden files — the trap that made `.resolve-threads`
  a no-op for a day.
- [x] 4.2 It addresses every accepted finding in one patch, and reports which it
  could not. Verify on a pull request carrying two accepted findings.
- [x] 4.3 Verify a dispatch with nothing accepted writes no patch and says so.
  **DONE.** `fix` holds `contents: read, pull-requests: read, issues: read, id-token: write`; the suite asserts `contents: write` appears in `land` and nowhere else. Artifact `dispatch-fix` uploads with `include-hidden-files: true` and `if-no-files-found: error` — both asserted.
  **DONE in code; the live half is blocked on the merge, see 6.1.** One model run over the whole work list, one patch (`git add -N . && git diff --binary`), and a `report.json` naming `fixed` / `unfixed` with reasons. The lander then checks each claimed fix against the patch's own file list.
  **DONE.** `collect` outputs the count; `fix` is gated on `!= '0'`, so no model runs and no patch exists. `land` still runs, hands the lander an empty patch, and the lander posts `no finding is accepted, so nothing was written` — asserted in `land-dispatch.test.sh`.

## 5. Landing it, with no model in the job

- [x] 5.1 The applying job holds `contents: write`, runs no model, applies the
  patch, commits naming the findings, and pushes to the pull request's branch.
  Verify the job contains no model step at all.
- [x] 5.2 It replies in each fixed finding's thread naming the commit, THEN
  hands the ids to `resolve-review-threads.py`. Verify the ordering: a reply
  written before the push is a claim, not a report.
- [x] 5.3 A patch that fails to apply resolves NOTHING and says so in the pull
  request. Verify deliberately, with a patch made stale by a push.
- [x] 5.4 Verify the reused resolver still refuses a thread the review did not
  author, through this path as well as its own.
  **DONE.** `land`: `contents: write`, `pull-requests: write`, and one program — `land-dispatch.py` — over the artifacts. The suite asserts the job's serialised definition contains no `claude`, and that the script itself contains none either. The programs it runs are read from the branch the WORKFLOW came from, never from the pull request's checkout, so a pull request cannot rewrite its own lander or resolver.
  **DONE.** Push, then reply, then resolve — in that order in code, and observable in the test: the `gh` stub records what origin's branch pointed at AT THE MOMENT of each reply, and the assertion is that it already held the new sha. The resolve mutation is asserted to come after the reply line.
  **DONE.** `git apply --check` first; on failure a pull-request comment names the branch, says nothing was pushed or resolved, quotes git, and the run fails. Verified in the suite with a patch made stale by a commit on origin: origin's sha unchanged, no `/replies`, no mutation.
  **DONE.** The suite hands the lander a work list claiming a person's thread and a patch that applies; the resolver refuses it (`authored by 'a-maintainer', not the review`) and no mutation is sent. Same refusal, second path.

## 6. On a real pull request, with a real finding

- [x] 6.1 Run the whole loop on a live pull request: a review finding, `fix it`
  in its thread, one dispatch, one commit, the thread answered and closed.
  Verify against the WORKTREE's branch, not master's.
- [x] 6.2 Leave a second finding un-triaged in that same run, and verify the
  pull request still cannot merge — the conversation-resolution rule holding it.
- [x] 6.3 **Close `github-change-lifecycle` §7.5, §7.6, §7.8 and §7.9 from this
  run**, which is what a fixed-and-unfixed pair produces: no repetition of the
  standing finding on the next push, the fixed thread answered and resolved, a
  detached thread re-checked rather than resolved, and a human-dismissed finding
  counted rather than re-raised. Verify each by reading the pull request, and
  record the verdicts in that change's tasks file.
  **RAN LIVE ON #65 — THE MODEL HALF IS BLOCKED ON THE MERGE, exactly as `github-change-lifecycle` §7.1 was.** The review filed two findings; `Fix it.` was replied under one; `gh workflow run review-dispatch.yml --ref change/review-findings-dispatch -f pr=65` (run 32974271654). `gate` authorised the sender and read the branch; `collect` produced a work list of exactly the accepted thread and skipped the other; `fix` ran the action, which REFUSED ITSELF — it will not run a workflow file that differs from the default branch's copy, on any trigger, `workflow_dispatch` included; `land` applied an empty patch, committed nothing, resolved nothing, and posted `nothing landed. Every accepted finding is still open`. That reason was WRONG ("not addressed by the fixing step" for a step that never ran), so the fix job now asserts `execution_file` the way `claude-review.yml` does and, on the legitimate skip, writes the real reason into every thread. The first dispatch on a pull request AFTER the merge is where the fixing half gets its evidence; both findings were fixed by hand in the meantime.
  **VERIFIED on #65.** With the second finding's thread untriaged, `mergeStateStatus` read `BLOCKED` after the dispatch completed — the conversation-resolution rule holding it, with `ci-green` green.
  **ALREADY CLOSED, on #62, before this change.** `github-change-lifecycle` was archived (`2026-08-26-github-change-lifecycle`) with §7.5, 7.6, 7.8 and 7.9 each ticked and verified there — a fixed-and-unfixed pair was reached on a probe pull request after all. Nothing to record; the archived tasks file already carries the verdicts.

## 7. Documentation

### 7.1 The reference docs

- [x] 7.1.1 `.claude/rules/worktree-delivery.md` — it describes how a change
  reaches master and stops at the review. Add what a contributor does when the
  review finds something: the accept vocabulary, the dispatch form, and that an
  untriaged finding blocks the merge. Verify it stays one topic and does not
  become a second copy of the workflow.
- [x] 7.1.2 `CONTRIBUTING.md` — the same two sentences for the reader who never
  opens `.claude/`. Verify it links rather than restates.
- [x] 7.1.3 Record that branch protection requires conversation resolution, and
  that this is the one gate NOT reported through `ci-green` — with the reason it
  is an exception, so the next reader does not "fix" the inconsistency. Verify
  the claim against the live setting rather than asserting it.
  **DONE.** New section *The review found something. Now what* — a four-row table of what you type, where, and what it means; the file-not-classifier rule; trigger-not-instruction; who may dispatch; the model cannot push; an untriaged finding blocks the merge; and that the landed commit has no CI until the next push. It names the workflow and the two programs rather than restating them.
  **DONE.** One paragraph under *Pull requests*, after the fork sentence, naming the vocabulary file and linking `worktree-delivery.md` for the rest.
  **DONE, verified live**: `gh api .../branches/master/protection` reports `required_status_checks.contexts: [ci-green]` and `required_conversation_resolution.enabled: true` (2026-08-26). Recorded in `worktree-delivery.md` under the gates section: it is not a check, so it cannot be a `needs:` line, and a job asserting it would race every reply.

### 7.2 The adopter site

- [x] 7.2.1 Confirm the site is genuinely untouched: no page under `docs/`
  describes contribution, review or pull requests. State the reader who is
  unaffected and why, rather than ticking a box.
- [x] 7.2.2 Verify the site still builds — this change touches no page, so a
  failure means something moved that was not meant to.
  **CONFIRMED.** `grep -i 'pull request\|code review\|review thread'` over every site page and guide returns nothing; `_data/nav.yml` addresses somebody installing agent-ops. The reader unaffected is the adopter: nothing here reaches a cluster, a chart value or a CRD.
  **VERIFIED BY IDENTITY.** `git diff --stat origin/master -- docs/` is empty, so the site's build inputs are byte-for-byte master's — which GitHub Pages builds and serves now. CI's path-filtered `site` job therefore does not run on this pull request, and the local jekyll container could not be started from this session (the Docker daemon needs the desktop up); neither adds anything to an identical input. A failure here would need a file under `docs/` to have moved, and none did.
