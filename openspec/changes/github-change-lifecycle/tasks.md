# Tasks — the change lifecycle on GitHub

Ordered so that nothing references a thing that does not yet exist, and so that
the flow is exercised on ONE change before thirteen are migrated onto it.

**Two tasks land outside this repository** (§1). They fail silently if skipped —
a worktree that deploys `master`'s chart reports success — so each is verified by
USE rather than by reading the command back.

## 1. Preconditions, outside this repository's diff

- [ ] 1.1 Template the chart path in `home-data-center`'s
      `apps/ai/apps/agent-ops/helmfile.d/helmfile.yaml.gotmpl` — replace the
      literal `chart: ../../../../../../agent-ops-operator/chart` with a
      `dig`-defaulted value. Verify by running the existing `helmfile sync` with
      NO override and confirming the release is unchanged
- [ ] 1.2 Verify the override actually redirects: create a throwaway worktree,
      change one rendered value in ITS chart only, run
      `helmfile --state-values-set chartPath=<worktree>/chart template`, and
      confirm the rendered output carries the worktree's value and not master's.
      **This is the silent failure of the whole change — do not mark it done on
      the strength of the flag being accepted**
- [ ] 1.3 Move the `agentops-go` container's mount from `$PWD` to the worktrees'
      parent directory. Verify by running `go build ./...` inside a worktree
      through `docker exec` and getting a successful build, not a missing-module
      error

## 2. Repository setup

- [ ] 2.1 Create the four phase labels (`proposed`, `applying`, `review`,
      `archived`, under one prefix) and verify `gh label list` shows them
- [ ] 2.2 Add the `claude_code_oauth_token` repository secret and verify a
      trivial workflow-dispatch run authenticates. Record the VERDICT only —
      never the token, per `.claude/rules/publication.md`
- [ ] 2.3 Add a tracking-issue template with the fixed section set from
      design.md D5, and verify a rendered issue shows exactly those sections.
      **It must not duplicate `bug_report.yml` or `feature_request.yml`** —
      those are `public-exposure`'s and are for INBOUND reports

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
      working copy when an active change owns a branch — modelled on
      `require-docs-task.sh`, **failing open** on anything it cannot read.
      Verify both directions: it refuses the bad case, and it stays silent on a
      docs typo with no change behind it

## 4. The tracking issue

- [ ] 4.1 Extend `.claude/commands/opsx/propose.md` to open the issue with the
      generated body and the `proposed` label, and to write the issue number to
      a sidecar in the change directory. Verify on a throwaway change that the
      issue exists, the sidecar holds only the number, and the body restates no
      section of `proposal.md`
- [ ] 4.2 Advance the phase label from `apply` and `archive`, one comment per
      transition. Verify the throwaway change's issue shows exactly one comment
      per transition and no per-task noise
- [ ] 4.3 Close the issue on archive, and verify the sidecar survives into
      `openspec/changes/archive/` so an archived change is still traceable
- [ ] 4.4 Add the `openspec/config.yaml` rule that makes the binding part of
      what a generated artifact carries, and verify it is injected by generating
      one artifact and reading the instructions output

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

## 6. The pull request and its checks

- [ ] 6.1 Add the openspec validation job to `ci.yml` and add it to `ci-green`'s
      `needs`. Verify it FAILS today — `openspec validate --all` currently
      reports three failing changes, so a green first run means it is not
      actually running
- [ ] 6.2 Fix the three failing changes, or record why each is left, and verify
      the new job goes green
- [ ] 6.3 Lift `require-docs-task.sh`'s decision into a CI check over the change
      touched by the pull request. Verify it fails a branch whose documentation
      section is unticked and passes one where it is complete
- [ ] 6.4 Pin the hook and the check against shared fixtures so they cannot
      diverge, and verify a fixture that one accepts and the other rejects fails
      the test
- [ ] 6.5 Add a pull-request title check against the commit convention, since
      squash merge makes the title the commit subject (design D3). Verify it
      rejects a title with no `type(scope):` prefix
- [ ] 6.6 Update `.github/PULL_REQUEST_TEMPLATE.md`: the openspec change is
      derived from the branch, not asked for. Verify the rendered template no
      longer contains a field the branch already answers

## 7. The iterative review

- [ ] 7.1 Add `.github/workflows/claude-review.yml` with the `review` job —
      `anthropics/claude-code-action@v1`, `track_progress: true`,
      `use_sticky_comment: true`, `contents: read` + `pull-requests: write`,
      triggers `[opened, synchronize, ready_for_review]`, drafts skipped, and a
      per-PR concurrency group with `cancel-in-progress`. Verify a first run
      posts inline comments and one summary
- [ ] 7.2 Write the review prompt against the project's own doctrine (design
      D11): the invariants, the retired vocabulary and the change's delta specs,
      alongside ordinary correctness. Verify on a deliberately doctrine-breaking
      test branch that it names the rule contradicted
- [ ] 7.3 Skip the review when `publication` or `retired-vocabulary` fail, and
      verify a branch that trips one of them gets no review run
- [ ] 7.4 Verify NO REPETITION across pushes — push a second commit that leaves
      one finding unaddressed, and confirm the existing remark is untouched and
      no duplicate appears. **This is the whole capability; a passing run that
      re-posts is a failure**
- [ ] 7.5 Add the `reconcile` job — `contents: write`, NO model, resolving only
      the thread ids handed to it. Verify a fixed finding's thread is answered
      and resolved on the next push
- [ ] 7.6 Enforce in `reconcile` that a thread whose first comment is not the
      review's own is REFUSED. Verify with a human-authored thread that survives
      a run untouched — this is the failure that destroys information
- [ ] 7.7 Verify a detached thread is re-checked rather than resolved: push a
      whitespace-only edit around an open finding, and confirm the finding is
      re-raised at its new location and the old thread closed as superseded
- [ ] 7.8 Verify a human-resolved finding is not re-raised and IS counted in the
      sticky summary as dismissed

## 8. End-to-end, on one change before thirteen

- [ ] 8.1 Run this change itself through the full flow — worktree, branch,
      issue, pull request, review, archive inside the PR, squash merge — and
      verify each stage without shortcutting any
- [ ] 8.2 Verify the archive-inside-the-PR decision holds in practice (design
      D4): the pull request diff shows the delta specs folding into
      `openspec/specs/`, and `require-docs-task.sh` gates it as expected
- [ ] 8.3 Verify `.github/components.sh` still reports 13 images and 12 modules
      with the worktree present, and that `find . -type d -name docs` returns
      exactly one line

## 9. Migrating the thirteen in flight

- [ ] 9.1 Write the migration as a SCRIPT, not a hand-run sequence — thirteen
      repetitions are where one gets done differently. Verify with `--dry-run`
      that it names all thirteen and would create exactly one issue and one
      branch each
- [ ] 9.2 Run it, and verify every active change has an issue with the correct
      phase label and a branch, with no change left behind and none given two
- [ ] 9.3 Verify the sidecar binding is written for all thirteen and that
      `openspec list` is unchanged by the migration

## 10. Documentation

**Last, because it records what the change actually did — which is routinely not
what the proposal said it would.**

### 10.1 The reference docs

- [ ] 10.1.1 `CONTRIBUTING.md` — rewrite "How a change is proposed here" and
      "Pull requests" for the new flow: the worktree, the branch, one pull
      request, the issue, the review, and the checks. Verify every command it
      shows actually runs
- [ ] 10.1.2 `.claude/rules/session-naming.md` — a session now owns a worktree
      as well as a name. Verify the two rules read as one model, not two
- [ ] 10.1.3 `.claude/rules/build-test.md` — the container mount moves (§1.3).
      Verify the documented command is the one that works from a worktree
- [ ] 10.1.4 `CLAUDE.md` — one line routing to the new rules file, per the
      index rule in `.claude/rules/authoring.md`. Verify it stays an index and
      does not grow a fourth named exception
- [ ] 10.1.5 Add the retired terms to `.github/retired-vocabulary.json` for
      anything this change stops saying — "commit directly to master" chief
      among them — and verify
      `python3 .github/scripts/retired-vocabulary-guard.py` passes
- [ ] 10.1.6 `docs/CHANGELOG.md` — **NO entry**, and confirm that is still
      right: nothing an adopter installs or runs changed
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
