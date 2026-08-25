# Tasks — the change lifecycle on GitHub

Ordered so nothing references a thing that does not yet exist, and so the flow is
exercised on ONE change before eleven are migrated onto it.

**Two tasks land outside this repository** (§1). Both fail silently if skipped —
a worktree that deploys `master`'s chart reports success — so each is verified by
USE rather than by reading the command back.

## 1. Preconditions, outside this repository's diff

- [x] 1.1 In the repository that deploys this chart, replace the fixed chart
      reference with a defaulted, overridable value. Verify by rendering with NO
      override and confirming the release still names the published chart at its
      pinned version — done: `oci://…/agent-ops-operator` at `13.0.1`, unchanged
- [x] 1.2 Verify the override actually redirects, by reading the RESOLVED
      release rather than the exit code. **Not done on the strength of the flag
      being accepted** — the rendered release was extracted both ways: default
      gives the published chart at `13.0.1`, the override gives the worktree's
      own `chart/` path with no version, which is what helm requires of a local
      chart
- [x] 1.3 Mount the worktrees' root into `agentops-go` BESIDE the repository —
      two explicit mounts, not their common parent, which would hand the
      container every other project on this machine. Both are needed: a
      worktree's `.git` is a file pointing into the main repository's
      `.git/worktrees/`. Verified with `go build ./...` in the worktree
      (`signals/cron` and `platform/manager`) and in the main checkout

## 2. Repository setup

- [x] 2.1 Create the four phase labels under one prefix and verify
      `gh label list` shows them
- [x] 2.2 **No secret, and that is the resolution.** The repository is connected
      to the Claude GitHub integration, so the action authenticates by exchanging
      the workflow's OIDC token — `id-token: write` is what the job needs, and
      nothing is stored. The empty `claude_code_oauth_token` input was REMOVED
      rather than left blank: a static credential takes precedence over
      federation, so an input resolving to nothing can silently win
- [x] 2.3 **NO tracking-issue template, and that is the resolution rather than a
      skip.** The spec requires the body to be GENERATED rather than written by
      hand; a template is an invitation to hand-write it, and would be a second
      way to create the same object that could drift from the first.
      `.github/scripts/opsx-issue.sh` is the generator, and the two inbound
      templates keep their job unchanged

## 3. The worktree and branch flow

- [x] 3.1 Write `.claude/rules/worktree-delivery.md` — placement, branch naming,
      the one-PR rule, and WHY worktrees cannot live inside the tree. Verify by
      running `.github/components.sh images | jq length` with a worktree present
      and getting 13, not 26
- [x] 3.2 Extend `.claude/commands/opsx/apply.md` to create
      `../agent-ops-worktrees/<change>/` on branch `change/<name>` if absent, and
      verify a fresh `/opsx:apply` lands in the worktree with its own HEAD
- [x] 3.3 Extend `.claude/commands/opsx/archive.md` to remove the worktree after
      a successful archive, and verify `git worktree list` no longer names it
- [x] 3.4 Add a `PreToolUse` hook refusing a commit to `master` from the main
      working copy when the files being committed belong to a change that owns a
      branch — modelled on `require-docs-task.sh`, **failing open** on anything
      it cannot read. Verify both directions: it refuses the bad case, and stays
      silent on a docs typo with no change behind it
- [x] 3.5 Wire the hook into `.claude/settings.json` beside the existing one, and
      verify both still fire — a second matcher on the same tool must not
      displace the first

## 4. The tracking issue

- [x] 4.1 Extend `.claude/commands/opsx/propose.md` to open the issue with the
      generated body and the `proposed` label, and to write the issue number to a
      sidecar in the change directory. Verify on a throwaway change that the
      issue exists, the sidecar holds only the number, and the body restates no
      section of `proposal.md`
- [x] 4.2 Advance the phase label from `apply` and `archive`, one comment per
      transition. Verify the throwaway change's issue shows exactly one comment
      per transition and no per-task noise
- [x] 4.3 Close the issue on archive, and verify the sidecar survives into
      `openspec/changes/archive/` so an archived change stays traceable
      **VERIFIED 2026-08-25, and it FAILED FIRST.** The sidecar does survive into
      `openspec/changes/archive/<date>-<name>/` — but `opsx-issue.sh` resolved only the
      LIVE path, so `phase archived` and `close` reported "no tracking issue" at exactly
      the point /opsx:archive calls them. #33 was closed by hand. Fixed in #37; the
      lookup now falls back to the archive, matched on the date prefix so `auth` cannot
      collect `2026-01-01-oauth`'s number. Confirmed against the real archived change:
      `number presentation-reduced-motion-opt-in` -> 33.
- [x] 4.4 Add the `openspec/config.yaml` rule that makes the binding part of what
      a generated artifact carries, and verify it is injected by generating one
      artifact and reading the instructions output

## 5. The reverse direction: an inbound issue becomes a change

- [x] 5.1 `/opsx:explore <number>` reads the issue with `gh issue view --json`
      and explores what was filed — including the comments, which routinely carry
      the constraint that makes the obvious answer wrong. It also states that
      exploring is not accepting: an issue may end in "this does not fit", with
      the reasoning in the thread
- [x] 5.2 Promotion in place, **exercised against a real filed issue rather than
      asserted**: an issue was filed as a reporter would write it, given a
      comment, then promoted. It kept its number, title, body and comment,
      gained `opsx:proposed` beside the reporter's own `enhancement` label, and
      the repository's open-issue count did not rise — no second issue anywhere.
      The probe was then closed with an explanation of what it was
- [ ] 5.3 Verify the archive path closes a promoted issue exactly as it closes a
      project-authored one, with the reporter's thread still readable

## 6. The two gates

- [x] 6.1 Add the openspec validation job to `ci.yml`. Verify it FAILS on the
      current tree — `openspec validate --all` reports failing changes today, so
      a green first run means it is not actually running
- [x] 6.2 **Left, and recorded.** `on-demand-adapters`,
      `one-page-per-integration` and `signal-triage` each carry a `MODIFIED`
      block that omits scenarios the current spec still has — so archiving them
      would silently DROP those scenarios. They are three other changes' work and
      do not belong in this diff. The gate is scoped to what a pull request
      touches, so it catches each the next time one is worked on; the job is
      green here because this change's own artifacts are valid
- [x] 6.3 Write `.github/scripts/docs-task-guard.py` — the documentation-task
      decision, over the change a pull request touches. Verify it fails a branch
      whose documentation section is unticked and passes one where it is complete
- [x] 6.4 Add it as a job in `ci.yml`, and add BOTH new jobs to `ci-green`'s
      `needs:` list. Verify `ci-green` reports them and that no branch-protection
      change was needed (design D14)
- [x] 6.5 Pin the hook and the guard against SHARED fixtures so they cannot
      diverge, and verify a fixture that one accepts and the other rejects fails
      the test
- [x] 6.8 **Test every script this change adds, not just the one with fixtures.**
      Six scripts and a hook were each verified ONCE, by hand, in directories
      that no longer exist — which proves they worked that afternoon and catches
      nothing after. `.github/tests/run.sh` now runs **58 assertions** across all
      of them, with `gh` and `openspec` stubbed so the suite reaches no network
      and touches no real repository, and it runs in CI.
      **It found a real defect on its first run:** `mark-thread-resolved.sh`
      validated thread ids with the `case` glob `[A-Za-z0-9_=-]*`, which matches
      a first character followed by ANYTHING — it accepted `PRRT_x; rm -rf /`
      unchanged while reading as validation in review. Anchored, and pinned
- [x] 6.6 Add a pull-request title check against the commit convention, since
      squash merge makes the title the commit subject (design D3). Verify it
      rejects a title with no `type(scope):` prefix
- [x] 6.7 Update `.github/PULL_REQUEST_TEMPLATE.md`: the openspec change is
      derived from the branch, not asked for. Verify the rendered template no
      longer contains a field the branch already answers

## 7. The iterative review

- [x] 7.1 **BLOCKED ON THE MERGE, not on anything buildable.** The action refuses
      to run when this workflow file differs from the default branch's copy — a
      pull request may not rewrite the review that judges it — so its first real
      run is the NEXT pull request. Found the only way it could be: the job went
      green in 17s having posted nothing, which is now asserted against (design
      D12a). Remaining: Add `.github/workflows/claude-review.yml` with the `review` job —
      `anthropics/claude-code-action@v1`, `track_progress: true`,
      `use_sticky_comment: true`, `contents: read` + `pull-requests: write`,
      triggers `[opened, synchronize, ready_for_review]`, drafts skipped, and a
      per-PR concurrency group with `cancel-in-progress`. Verify a first run
      posts inline comments and one summary
      **VERIFIED on #34**, the first pull request after the merge. It also found what
      the merge could not: the action had NO MODEL CREDENTIAL — the GitHub App
      authorises reading and commenting, not the Anthropic call — so the first run
      failed on `Either ANTHROPIC_API_KEY, CLAUDE_CODE_OAUTH_TOKEN, or workload identity
      federation ... is required`, and the always-ran guard refused to call that green.
      Fixed in #35. The review then ran for 2m39s, posting ONE sticky summary and no
      inline comments (there were no findings). Inline comments confirmed separately on
      #37 and #40, which each carried one.
- [ ] 7.2 Skip forks explicitly on `pull_request.head.repo.fork` (design D13) and
      verify the check reports SKIPPED rather than failing on a missing secret —
      the two look identical in the checks list and mean opposite things
- [ ] 7.3 Write the review prompt against the project's own doctrine (design
      D11): the invariants, the retired vocabulary and the change's delta specs,
      alongside ordinary correctness. Verify on a deliberately doctrine-breaking
      test branch that it names the rule contradicted
- [x] 7.4 The review runs both hygiene guards first and skips itself when either
      fails, with a `::notice::` saying why — a failing guard means the diff
      carries content on its way out, and reviewing it spends a model call on
      text about to be deleted. Confirmed in the workflow log: the step ran and
      reported `ok=true` on this branch
- [x] 7.5 Verify NO REPETITION across pushes — push a second commit leaving one
      finding unaddressed, and confirm the existing remark is untouched and no
      duplicate appears. **This is the whole capability; a passing run that
      re-posts is a failure**
      **VERIFIED on #37 and #40.** A finding was left standing across a push on #37:
      the second run added a reply in the existing thread and did NOT re-post the
      finding — three comments on that thread in total, one of them the review's
      "Fixed in 293cc78". The sticky summary is edited in place rather than re-posted:
      one PR-level comment from the review on each pull request.
- [x] 7.6 Add `.github/scripts/resolve-review-threads.py` and the `reconcile`
      job — `contents: write`, NO model, resolving only the thread ids handed to
      it. Verify a fixed finding's thread is answered and resolved on the next
      push
      **FALSIFIED, THEN VERIFIED.** On #37 the review replied "Fixed in <sha>" and the
      thread stayed open: `reconcile` refused it — `authored by 'claude', not the
      review`. REST and GraphQL spell a bot differently (`claude[bot]` against
      `claude`), so the allowlist could never match. Fixed in #39, which also made the
      ACCOUNT TYPE decide, since `claude` is a login a person can hold. **On #40 both
      threads came back `isResolved=true` with no human click.** Found by 7.7's
      diagnostic, exactly as that task predicted.
- [x] 7.7 The script refuses any thread whose first comment is not the review's,
      **verified against a REAL human-authored thread** rather than a fixture: a
      review comment was posted on this pull request as the maintainer, handed to
      `resolve-review-threads.py`, and refused — `authored by
      'kostiantyn-matsebora', not the review` — with the thread still
      `isResolved: false` afterwards. It also PRINTS every author it saw, so the
      first real review run reports the login to pin rather than silently
      resolving nothing
- [ ] 7.8 Verify a detached thread is re-checked rather than resolved: push a
      whitespace-only edit around an open finding, and confirm it is re-raised at
      its new location and the old thread closed as superseded
- [ ] 7.9 Verify a human-resolved finding is not re-raised and IS counted in the
      sticky summary as dismissed

## 8. End-to-end, on one change before eleven

- [x] 8.1 Run this change itself through the full flow. **Done up to the merge**:
      worktree at `../agent-ops-worktrees/github-change-lifecycle`, branch
      `change/github-change-lifecycle`, issue #29 advancing `proposed` →
      `applying` → `review`, pull request #30 with the new checks reporting.
      Outstanding: the review's first real run (needs §2.2), the archive, and the
      squash merge — **left deliberately for a person**, since the merge is the
      one step here that cannot be undone
      **VERIFIED end to end on `presentation-reduced-motion-opt-in`**, a change proposed,
      applied, archived and merged after this flow landed: issue #33 advancing
      proposed -> applying -> review -> archived, worktree at
      `../agent-ops-worktrees/presentation-reduced-motion-opt-in`, branch
      `change/presentation-reduced-motion-opt-in`, pull request #34 squash-merged, and
      the worktree removed afterwards. THIS change's own merge remains a person's, as
      written.
- [x] 8.2 Verify the archive-inside-the-PR decision holds in practice (design
      D4): the pull request diff shows the delta specs folding into
      `openspec/specs/`, and the documentation gate passes as expected
      **VERIFIED on #34.** `openspec archive` ran on the branch, and the pull request's
      diff carried `openspec/specs/landing-presentation/spec.md` (+1 requirement, ~1
      modified) beside the implementation. The `docs-task` gate passed on the same run,
      and `openspec` validated the change while it was still in `openspec/changes/`.
- [x] 8.3 Verify `.github/components.sh` still reports 13 images and 12 modules
      with the worktree present, and that `find . -type d -name docs` returns
      exactly one line

## 9. Migrating the eleven in flight

- [x] 9.1 `.github/scripts/opsx-migrate.sh`, verified with `--dry-run`: it names
      twelve active changes, would create exactly one branch and one issue for
      each of the ELEVEN that lack them, and correctly KEEPS this change's
      existing branch and issue #29 rather than duplicating them. An existing
      branch is never reset to master — a branch that has moved on is somebody's
      work in progress
- [x] 9.2 Run it — **AFTER the merge, not before.** Until the flow is on the
      default branch the commit hook is not active, and eleven branches nothing
      yet expects are litter. Then verify every active change has one issue with
      the right phase label and one branch, none left behind and none given two
      **RUN 2026-08-25**: 11 branches and 10 issues created (#42-#51), with #29, #38
      and #41 KEPT rather than duplicated. Thirteen changes, each with exactly one
      issue and one branch — listed and checked, none left behind and none given two.
      
      **AND IT FOUND A HAZARD THE DRY-RUN OF 9.1 COULD NOT.** Idempotence rests on the
      sidecar, and a sidecar written by a CONCURRENT SESSION lives in ITS worktree,
      uncommitted and invisible from the shared checkout. The first dry-run therefore
      read `one-page-per-integration issue:would open` while #41 already existed. It
      was caught by dry-running before the real run, and fixed by writing that
      session's number into this checkout first — but a future run has the same
      exposure, and the script cannot see what it cannot read. **Dry-run first is not
      advice, it is the procedure.**
- [x] 9.3 Verify the sidecar binding is written for all eleven and that
      `openspec list` is unchanged by the migration
      **VERIFIED.** Every one of the thirteen carries its binding — the eleven the
      migration wrote, plus #29 and #41 kept. `openspec list` reports 13 changes,
      unchanged by the migration: a sidecar is not an artifact openspec reads.
      
      **Committed onto each change's OWN branch**, never the default one — which the
      §3.4 hook now enforces, so this is not merely a convention.
- [x] 9.4 **Tell the other sessions.** Once every change owns a branch, the §3.4
      hook starts refusing commits to `master` that were fine an hour earlier.
      Verify by making the intended refusal happen once, deliberately, and
      reading the message it gives
      **THE REFUSAL WAS MADE TO HAPPEN, DELIBERATELY.** A change's sidecar was staged on
      `master` in the shared checkout and a commit attempted: the hook exited 2 and
      named the change, the branch it owns, and the worktree command to use instead.
      
      **AND THE HAZARD IT GUARDS IS REAL, PROVEN THE HARD WAY IN THE SAME SESSION.**
      Committing the eleven sidecars onto their branches with plumbing moved
      `change/one-page-per-integration` while a CONCURRENT SESSION had it checked out —
      its worktree's HEAD followed the ref, its index did not, and its `git status`
      showed a staged DELETION of a file it had never touched. A `git commit -a` there
      would have made that real. Reverted with `update-ref` before any harm; the
      lesson is that `git worktree list` must be READ and ACTED ON, not merely run.

## 10. Documentation

**Last, because it records what the change actually did — which is routinely not
what the proposal said it would.** This one already diverged twice: required
checks moved from out-of-scope to free, and thirteen in-flight changes became
eleven.

### 10.1 The reference docs

- [x] 10.1.1 `CONTRIBUTING.md` — rewrite "How a change is proposed here" and
      "Pull requests" for the new flow: the worktree, the branch, one pull
      request, the issue, the review, and the gates. Verify every command it
      shows actually runs
- [x] 10.1.2 `.claude/rules/session-naming.md` — a session now owns a worktree as
      well as a name. Verify the two rules read as one model, not two
- [x] 10.1.3 `.claude/rules/build-test.md` — the container mount moves (§1.3).
      Verify the documented command is the one that works from a worktree
- [x] 10.1.4 `CLAUDE.md` — one line routing to the new rules file, per the index
      rule in `.claude/rules/authoring.md`. Verify it stays an index and does not
      grow a fourth named exception
- [x] 10.1.5 Add the retired terms to `.github/retired-vocabulary.json` — "commit
      directly to master" chief among them — and verify
      `python3 .github/scripts/retired-vocabulary-guard.py` passes
- [x] 10.1.6 `docs/CHANGELOG.md` — **NO entry**, and confirm that is still right:
      nothing an adopter installs or runs changed
- [x] 10.1.7 Confirm no generated block is stale: no CRD field, api doc comment
      or chart value moved, so `python3 .github/scripts/docs-generate.py --check`
      must pass unchanged. Run it rather than reasoning about it

### 10.2 The adopter site

- [x] 10.2.1 Confirm the site is genuinely unaffected — re-run the check the
      proposal recorded: nothing under `docs/` describes contribution, the
      openspec workflow or pull requests, and every page in `_data/nav.yml`
      addresses somebody INSTALLING agent-ops. **State the reader who is
      unaffected and why**, rather than ticking a box
- [x] 10.2.2 Verify the site still builds — the change touches no page, so a
      failure here means something moved that was not meant to
