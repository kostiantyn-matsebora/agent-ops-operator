# Tasks — the change lifecycle on GitHub

Ordered so nothing references a thing that does not yet exist, and so the flow is
exercised on ONE change before eleven are migrated onto it.

**Two tasks land outside this repository** (§1). Both fail silently if skipped —
a worktree that deploys `master`'s chart reports success — so each is verified by
USE rather than by reading the command back.

## 1. Preconditions, outside this repository's diff

- [ ] 1.1 In the repository that deploys this chart, replace the literal
      relative chart path with a defaulted, overridable value. Verify by running
      the existing deploy with NO override and confirming the release is
      unchanged
- [ ] 1.2 Verify the override actually redirects: create a throwaway worktree,
      change one rendered value in ITS chart only, render with the override
      pointed at that worktree, and confirm the output carries the worktree's
      value and not master's. **This is the silent failure of the whole change —
      do not mark it done on the strength of the flag being accepted**
- [ ] 1.3 Move the `agentops-go` container's mount from `$PWD` to the worktrees'
      parent directory. Verify by running `go build ./...` inside a worktree
      through `docker exec` and getting a successful build, not a missing-module
      error

## 2. Repository setup

- [ ] 2.1 Create the four phase labels under one prefix and verify
      `gh label list` shows them
- [ ] 2.2 Add the `CLAUDE_CODE_OAUTH_TOKEN` repository secret. **Needs an
      interactive login, so it cannot be scripted.** Verify by a
      workflow-dispatch run that authenticates. Record the VERDICT only — never
      the token, per `.claude/rules/publication.md`
- [ ] 2.3 Add a tracking-issue template with the fixed section set from design
      D5, and verify a rendered issue shows exactly those sections. **It must not
      duplicate `bug_report.yml` or `feature_request.yml`** — those are for
      INBOUND reports and are already shipped

## 3. The worktree and branch flow

- [ ] 3.1 Write `.claude/rules/worktree-delivery.md` — placement, branch naming,
      the one-PR rule, and WHY worktrees cannot live inside the tree. Verify by
      running `.github/components.sh images | jq length` with a worktree present
      and getting 13, not 26
- [ ] 3.2 Extend `.claude/commands/opsx/apply.md` to create
      `../agent-ops-worktrees/<change>/` on branch `change/<name>` if absent, and
      verify a fresh `/opsx:apply` lands in the worktree with its own HEAD
- [ ] 3.3 Extend `.claude/commands/opsx/archive.md` to remove the worktree after
      a successful archive, and verify `git worktree list` no longer names it
- [ ] 3.4 Add a `PreToolUse` hook refusing a commit to `master` from the main
      working copy when the files being committed belong to a change that owns a
      branch — modelled on `require-docs-task.sh`, **failing open** on anything
      it cannot read. Verify both directions: it refuses the bad case, and stays
      silent on a docs typo with no change behind it
- [ ] 3.5 Wire the hook into `.claude/settings.json` beside the existing one, and
      verify both still fire — a second matcher on the same tool must not
      displace the first

## 4. The tracking issue

- [ ] 4.1 Extend `.claude/commands/opsx/propose.md` to open the issue with the
      generated body and the `proposed` label, and to write the issue number to a
      sidecar in the change directory. Verify on a throwaway change that the
      issue exists, the sidecar holds only the number, and the body restates no
      section of `proposal.md`
- [ ] 4.2 Advance the phase label from `apply` and `archive`, one comment per
      transition. Verify the throwaway change's issue shows exactly one comment
      per transition and no per-task noise
- [ ] 4.3 Close the issue on archive, and verify the sidecar survives into
      `openspec/changes/archive/` so an archived change stays traceable
- [ ] 4.4 Add the `openspec/config.yaml` rule that makes the binding part of what
      a generated artifact carries, and verify it is injected by generating one
      artifact and reading the instructions output

## 5. The reverse direction: an inbound issue becomes a change

- [ ] 5.1 Extend `.claude/commands/opsx/explore.md` to accept an issue number,
      read it with `gh issue view --json`, and seed the exploration from the
      reporter's own words. Verify against a throwaway issue that the exploration
      opens from what was filed
- [ ] 5.2 Promote IN PLACE on propose — the inbound issue becomes the tracking
      issue, keeping its title, body and comments, gaining the link and the
      label. Verify no second issue is created and the original thread is intact
- [ ] 5.3 Verify the archive path closes a promoted issue exactly as it closes a
      project-authored one, with the reporter's thread still readable

## 6. The two gates

- [ ] 6.1 Add the openspec validation job to `ci.yml`. Verify it FAILS on the
      current tree — `openspec validate --all` reports failing changes today, so
      a green first run means it is not actually running
- [ ] 6.2 Fix the failing changes, or record why each is left, and verify the job
      goes green
- [ ] 6.3 Write `.github/scripts/docs-task-guard.py` — the documentation-task
      decision, over the change a pull request touches. Verify it fails a branch
      whose documentation section is unticked and passes one where it is complete
- [ ] 6.4 Add it as a job in `ci.yml`, and add BOTH new jobs to `ci-green`'s
      `needs:` list. Verify `ci-green` reports them and that no branch-protection
      change was needed (design D14)
- [ ] 6.5 Pin the hook and the guard against SHARED fixtures so they cannot
      diverge, and verify a fixture that one accepts and the other rejects fails
      the test
- [ ] 6.6 Add a pull-request title check against the commit convention, since
      squash merge makes the title the commit subject (design D3). Verify it
      rejects a title with no `type(scope):` prefix
- [ ] 6.7 Update `.github/PULL_REQUEST_TEMPLATE.md`: the openspec change is
      derived from the branch, not asked for. Verify the rendered template no
      longer contains a field the branch already answers

## 7. The iterative review

- [ ] 7.1 Add `.github/workflows/claude-review.yml` with the `review` job —
      `anthropics/claude-code-action@v1`, `track_progress: true`,
      `use_sticky_comment: true`, `contents: read` + `pull-requests: write`,
      triggers `[opened, synchronize, ready_for_review]`, drafts skipped, and a
      per-PR concurrency group with `cancel-in-progress`. Verify a first run
      posts inline comments and one summary
- [ ] 7.2 Skip forks explicitly on `pull_request.head.repo.fork` (design D13) and
      verify the check reports SKIPPED rather than failing on a missing secret —
      the two look identical in the checks list and mean opposite things
- [ ] 7.3 Write the review prompt against the project's own doctrine (design
      D11): the invariants, the retired vocabulary and the change's delta specs,
      alongside ordinary correctness. Verify on a deliberately doctrine-breaking
      test branch that it names the rule contradicted
- [ ] 7.4 Skip the review when `publication` or `retired-vocabulary` fail, and
      verify a branch tripping one of them gets no review run
- [ ] 7.5 Verify NO REPETITION across pushes — push a second commit leaving one
      finding unaddressed, and confirm the existing remark is untouched and no
      duplicate appears. **This is the whole capability; a passing run that
      re-posts is a failure**
- [ ] 7.6 Add `.github/scripts/resolve-review-threads.py` and the `reconcile`
      job — `contents: write`, NO model, resolving only the thread ids handed to
      it. Verify a fixed finding's thread is answered and resolved on the next
      push
- [ ] 7.7 Enforce in the script that a thread whose first comment is not the
      review's own is REFUSED. Verify with a human-authored thread that survives
      a run untouched — this is the failure that destroys information
- [ ] 7.8 Verify a detached thread is re-checked rather than resolved: push a
      whitespace-only edit around an open finding, and confirm it is re-raised at
      its new location and the old thread closed as superseded
- [ ] 7.9 Verify a human-resolved finding is not re-raised and IS counted in the
      sticky summary as dismissed

## 8. End-to-end, on one change before eleven

- [ ] 8.1 Run this change itself through the full flow — worktree, branch, issue,
      pull request, review, archive inside the PR, squash merge — without
      shortcutting any stage
- [ ] 8.2 Verify the archive-inside-the-PR decision holds in practice (design
      D4): the pull request diff shows the delta specs folding into
      `openspec/specs/`, and the documentation gate passes as expected
- [ ] 8.3 Verify `.github/components.sh` still reports 13 images and 12 modules
      with the worktree present, and that `find . -type d -name docs` returns
      exactly one line

## 9. Migrating the eleven in flight

- [ ] 9.1 Write the migration as a SCRIPT, not a hand-run sequence — eleven
      repetitions are where one gets done differently. Verify with `--dry-run`
      that it names all eleven and would create exactly one issue and one branch
      each
- [ ] 9.2 Run it, and verify every active change has an issue with the correct
      phase label and a branch, with no change left behind and none given two
- [ ] 9.3 Verify the sidecar binding is written for all eleven and that
      `openspec list` is unchanged by the migration
- [ ] 9.4 **Tell the other sessions.** Once every change owns a branch, the §3.4
      hook starts refusing commits to `master` that were fine an hour earlier.
      Verify by making the intended refusal happen once, deliberately, and
      reading the message it gives

## 10. Documentation

**Last, because it records what the change actually did — which is routinely not
what the proposal said it would.** This one already diverged twice: required
checks moved from out-of-scope to free, and thirteen in-flight changes became
eleven.

### 10.1 The reference docs

- [ ] 10.1.1 `CONTRIBUTING.md` — rewrite "How a change is proposed here" and
      "Pull requests" for the new flow: the worktree, the branch, one pull
      request, the issue, the review, and the gates. Verify every command it
      shows actually runs
- [ ] 10.1.2 `.claude/rules/session-naming.md` — a session now owns a worktree as
      well as a name. Verify the two rules read as one model, not two
- [ ] 10.1.3 `.claude/rules/build-test.md` — the container mount moves (§1.3).
      Verify the documented command is the one that works from a worktree
- [ ] 10.1.4 `CLAUDE.md` — one line routing to the new rules file, per the index
      rule in `.claude/rules/authoring.md`. Verify it stays an index and does not
      grow a fourth named exception
- [ ] 10.1.5 Add the retired terms to `.github/retired-vocabulary.json` — "commit
      directly to master" chief among them — and verify
      `python3 .github/scripts/retired-vocabulary-guard.py` passes
- [ ] 10.1.6 `docs/CHANGELOG.md` — **NO entry**, and confirm that is still right:
      nothing an adopter installs or runs changed
- [ ] 10.1.7 Confirm no generated block is stale: no CRD field, api doc comment
      or chart value moved, so `python3 .github/scripts/docs-generate.py --check`
      must pass unchanged. Run it rather than reasoning about it

### 10.2 The adopter site

- [ ] 10.2.1 Confirm the site is genuinely unaffected — re-run the check the
      proposal recorded: nothing under `docs/` describes contribution, the
      openspec workflow or pull requests, and every page in `_data/nav.yml`
      addresses somebody INSTALLING agent-ops. **State the reader who is
      unaffected and why**, rather than ticking a box
- [ ] 10.2.2 Verify the site still builds — the change touches no page, so a
      failure here means something moved that was not meant to
