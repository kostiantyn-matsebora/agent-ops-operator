## 1. The accept vocabulary, stated once

- [ ] 1.1 Write the accept vocabulary into ONE place both the program and the
  contributor read — the phrase list, the matching rule (case-insensitive, whole
  reply, trailing punctuation ignored) and what is NOT an acceptance. Verify by
  pointing the program at it rather than restating it in code.

## 2. The work list, derived from the threads

- [ ] 2.1 Add the thread-walking program that emits the accepted findings as a
  list: thread id, path, line, the finding's text and the human's reply. Verify
  against a fixture of the four cases — accepted, argued with, unanswered, and
  answered by the review itself.
- [ ] 2.2 REFUSE an acceptance written by the review, not a person. A bot that
  can accept its own findings is a bot that writes to the branch unattended.
  Verify with a fixture whose reply is authored by the review's own login.
- [ ] 2.3 Ignore threads a person already resolved, and threads whose first
  comment is not the review's. Verify both, and verify the program reports what
  it skipped rather than skipping silently.
- [ ] 2.4 Give it a test suite in `.github/tests/`, in the house style — its
  REFUSALS are the product. Verify the suite fails when the refusal is removed,
  not merely that it passes.

## 3. The dispatch, and who may send one

- [ ] 3.1 Add the comment-triggered workflow on `issue_comment` and
  `pull_request_review_comment`, matched on the dispatch form. Verify a comment
  that is not a dispatch starts nothing — reading the run list, not the intent.
- [ ] 3.2 Gate on the comment's own `author_association` (write access) and
  refuse a fork's pull request. Verify the refusal is VISIBLE — a comment or a
  check that says so — because a silent no-op reads as a broken bot.
- [ ] 3.3 Never `pull_request_target`. Verify by asserting the trigger set in
  the workflow, and state in the file why the convenient one is refused.

## 4. Producing the fix without the power to land it

- [ ] 4.1 The fixing job runs under `contents: read`, takes the work list, and
  emits a PATCH as an artifact. Verify the job's permissions in the workflow and
  that the artifact carries hidden files — the trap that made `.resolve-threads`
  a no-op for a day.
- [ ] 4.2 It addresses every accepted finding in one patch, and reports which it
  could not. Verify on a pull request carrying two accepted findings.
- [ ] 4.3 Verify a dispatch with nothing accepted writes no patch and says so.

## 5. Landing it, with no model in the job

- [ ] 5.1 The applying job holds `contents: write`, runs no model, applies the
  patch, commits naming the findings, and pushes to the pull request's branch.
  Verify the job contains no model step at all.
- [ ] 5.2 It replies in each fixed finding's thread naming the commit, THEN
  hands the ids to `resolve-review-threads.py`. Verify the ordering: a reply
  written before the push is a claim, not a report.
- [ ] 5.3 A patch that fails to apply resolves NOTHING and says so in the pull
  request. Verify deliberately, with a patch made stale by a push.
- [ ] 5.4 Verify the reused resolver still refuses a thread the review did not
  author, through this path as well as its own.

## 6. On a real pull request, with a real finding

- [ ] 6.1 Run the whole loop on a live pull request: a review finding, `fix it`
  in its thread, one dispatch, one commit, the thread answered and closed.
  Verify against the WORKTREE's branch, not master's.
- [ ] 6.2 Leave a second finding un-triaged in that same run, and verify the
  pull request still cannot merge — the conversation-resolution rule holding it.
- [ ] 6.3 **Close `github-change-lifecycle` §7.5, §7.6, §7.8 and §7.9 from this
  run**, which is what a fixed-and-unfixed pair produces: no repetition of the
  standing finding on the next push, the fixed thread answered and resolved, a
  detached thread re-checked rather than resolved, and a human-dismissed finding
  counted rather than re-raised. Verify each by reading the pull request, and
  record the verdicts in that change's tasks file.

## 7. Documentation

### 7.1 The reference docs

- [ ] 7.1.1 `.claude/rules/worktree-delivery.md` — it describes how a change
  reaches master and stops at the review. Add what a contributor does when the
  review finds something: the accept vocabulary, the dispatch form, and that an
  untriaged finding blocks the merge. Verify it stays one topic and does not
  become a second copy of the workflow.
- [ ] 7.1.2 `CONTRIBUTING.md` — the same two sentences for the reader who never
  opens `.claude/`. Verify it links rather than restates.
- [ ] 7.1.3 Record that branch protection requires conversation resolution, and
  that this is the one gate NOT reported through `ci-green` — with the reason it
  is an exception, so the next reader does not "fix" the inconsistency. Verify
  the claim against the live setting rather than asserting it.

### 7.2 The adopter site

- [ ] 7.2.1 Confirm the site is genuinely untouched: no page under `docs/`
  describes contribution, review or pull requests. State the reader who is
  unaffected and why, rather than ticking a box.
- [ ] 7.2.2 Verify the site still builds — this change touches no page, so a
  failure means something moved that was not meant to.
