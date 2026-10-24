## Why

The review loop is half built and stops twice by design: every finding needs a
person to type `fix it` in its thread, and the commit `review-dispatch.yml` lands
gets neither CI nor a second review, because a push made with the workflow
token starts no workflow. A change whose owner has already approved it still
costs a triage pass per round and a hand push per round — and SonarCloud's
findings, live on every pull request since `sonarcloud-analysis` (#125), reach
nothing that fixes them at all. Close the loop: approve the change once, and CI
iterates over both sources of findings until none is left that a person has not
been told about.

## What Changes

- **A pull request label, `autofix`, is change-level consent.** Set on the
  owner's word by the session that owns the change (`gh pr edit --add-label`),
  so the write-access gate the dispatch already enforces holds. Without the
  label nothing changes: per-thread `fix it` and `/fix-accepted` stay exactly as
  they are.
- **The dispatch reads TWO sources on a labelled pull request:** every open
  review thread, and SonarCloud's open issues for the pull request
  (`api/issues/search?pullRequest=<n>`, one query per component project, the
  token the repository already holds). Both are collected by a program; the
  model reads neither API.
- **Every finding is FIXED or DISPUTED, never ignored.** A fix lands as today
  (`Fixed in <sha>`, thread resolved; a Sonar issue is closed by Sonar's own
  next analysis, never marked in Sonar by the fixer). A dispute is a reply
  stating why — in the thread, or on the pull request naming the Sonar issue
  key — the thread STAYS OPEN, and the round's summary `@mentions` the owner.
  A disagreement is a decision still owed, and an open thread already blocks
  the merge.
- **The landed commit re-triggers the review and CI**, so the next round runs
  without a hand push. The `land` job pushes with a GitHub App installation
  token held by that model-free job only; the first task spikes whether a
  `workflow_dispatch`ed `ci.yml` run satisfies branch protection on the head
  sha, and if it does the App is dropped before it is ever created.
- **The loop is BOUNDED.** At most N rounds per pull request (N a workflow
  constant, initially 3); a round that changed nothing — every finding
  disputed, or a stale patch — ends it early. Every ending posts ONE summary
  comment: fixed, disputed, rounds used, what is left, `@owner`.
- **SonarCloud's quality gate becomes a required check** — the deferral
  `sonarcloud-analysis` wrote ("measure first, gate in a later change"). It is
  the loop's termination signal for the Sonar half, so it reports through
  `ci-green`.
- **The opsx lifecycle gains the step**: after `/opsx:apply` opens the pull
  request, the owner approves and the session labels; `/opsx:archive` is
  refused while the loop is running or a dispute is unanswered.

## Capabilities

### New Capabilities

_None._ Everything here is a change to how the existing review, delivery and
analysis capabilities behave.

### Modified Capabilities

- `automated-code-review`: consent is per PULL REQUEST under `autofix`, not
  only per thread; the dispatch's work list includes SonarCloud issues; a
  finding may be DISPUTED and a dispute keeps its thread open and notifies the
  owner; a landed fix re-triggers the review; rounds are bounded and every
  ending is summarised.
- `code-quality-analysis`: the quality gate's verdict IS a required check —
  reverses "a failed quality gate does not fail the pull request". **Ordering:**
  that spec is published by `sonarcloud-analysis` (#125, PR #127), which MUST
  archive before this change's delta can fold; the delta is written against
  its text as it stands on that branch.
- `change-delivery`: the pull request lifecycle carries the approval step and
  the archive refusal while a loop is open or a dispute unanswered.

## Impact

- **Workflows and scripts:** `.github/workflows/review-dispatch.yml` (a
  label-triggered run, the two-source collector, the fix/dispute contract, the
  re-trigger and round counter in `land`), `.github/workflows/claude-review.yml`
  (dispatchable on a sha it did not see pushed), `ci.yml` (Sonar's gate in
  `ci-green`'s `needs:` or as the App-pushed run — decided by the spike),
  `.github/scripts/accepted-findings.py` (label mode: everything open),
  `land-dispatch.py` (dispute replies, summary, re-trigger), a new Sonar
  collector script, `.github/review-triage.json` (the label named beside the
  vocabulary).
- **Credential:** one GitHub App and its private key as a repository secret,
  held by `land` only — or none, if the spike succeeds.
- **opsx:** `.claude/skills/openspec-apply-change` (label on approval),
  `openspec-archive-change` and `.claude/hooks/require-docs-task.sh` (refuse
  while the loop is open).
- **Reference docs:** `.claude/rules/worktree-delivery.md` ("The review found
  something. Now what" — the label row, the dispute row, the re-trigger no
  longer needs a hand push), `.claude/rules/gotchas.md` (the token-push caveat
  is bounded to unlabelled pull requests), `CONTRIBUTING.md` (the review
  section and the SonarCloud gate), `docs/CHANGELOG.md` (the gate becomes
  required), `docs/security.md` (a credential that can push: what holds it and
  what cannot).
- **Adopter site:** nothing an adopter of the OPERATOR reads changes — the loop
  is this repository's contributor process. The documentation task states that
  explicitly rather than leaving the half unlisted.
- **Not changed:** the per-thread vocabulary, the model's inability to push,
  the review's refusal to run on a fork, thread resolution as the merge gate.
