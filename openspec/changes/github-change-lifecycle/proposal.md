# The change lifecycle runs on GitHub: a worktree, a branch, a pull request and an issue

## Why

**The repository is going live, and its development process is invisible from
outside it.** Every commit in this history landed directly on `master`. There
has never been a pull request that was not opened by Dependabot, there is not
one issue, and the only review a change has ever had is the session that wrote
it.

That was a reasonable arrangement for a private repository with one author. It
stops being one the moment a stranger can read the tree, because three things a
reader needs are simply absent: a place where work in progress is visible, a
record of why a change was accepted, and any check that ran before it landed on
the default branch.

**And the current rule actively prevents the fix.** This project's standing
instruction is to commit straight to `master` and never branch — not out of
preference, but because several sessions run at once **against one working
directory**, so they share one HEAD. On 2026-08-23 a session created a branch,
a concurrent session's commit landed on it, the two diverged, `--ff-only`
refused and the fix had to be cherry-picked back. The branch protected nothing
and caused the collision.

**A worktree per change removes that cause rather than working around it.**
Each session gets its own HEAD, so branching becomes safe for the first time —
and `session-naming.md` already names every session `<phase> <change>`. One
session, one change, one worktree, one branch, one pull request completes a
model this repository is already half inside.

## What Changes

### Delivery: a worktree, a branch, a pull request

- **Every openspec change is implemented in its own git worktree**, created at
  `../agent-ops-worktrees/<change>/` and removed when the change archives.
- **WORKTREES LIVE OUTSIDE THE REPOSITORY, and that is forced rather than
  chosen.** `.github/components.sh` derives what this project ships with
  `find . -name go.mod -mindepth 2` and `find . -name Dockerfile`, so a
  worktree under the root would report **twenty-six** components and hand CI a
  doubled matrix. `structure.md`'s standing test — `find . -type d -name docs`
  returns one line — would break in the same instant.
- **Branch `change/<name>`, one pull request per change**, its title the
  conventional-commit subject the merge will carry.
- **BREAKING for how work is done here**: the "commit directly to `master`"
  rule is **rewritten, not merely overridden**. It survives only as the record
  of why it existed, because the argument for it was correct under the
  conditions that produced it and will be re-derived by anyone who meets those
  conditions again.
- **Two prerequisites land OUTSIDE this repository's diff**, and both are
  silent failures if skipped:
  1. **The build container.** `build-test.md` starts `agentops-go` with
     `-v "$PWD":"$PWD"`, so a worktree at any other path is **invisible inside
     it** — every `go build` in a worktree fails to find the tree. The mount
     moves to the worktrees' parent directory.
  2. **helmfile.** The deploy points at a literal path,
     `chart: ../../../../../../agent-ops-operator/chart`. Verifying a branch
     against the live cluster would sync **`master`'s** chart while reporting
     success — the change would look simply not to work. The path becomes
     `{{ dig "chartPath" "…" .Values }}`, overridden per worktree with
     `helmfile --state-values-set chartPath=…`. That edit belongs to the
     `home-data-center` repository and is therefore a **precondition of this
     change, not part of it**.

### Tracking: one GitHub issue per change, and it is a POINTER

- **An issue is opened when a change is proposed and closed when it archives**,
  carrying a phase label that advances with the opsx verb driving the work:
  `proposed` → `applying` → `review` → `archived`.
- **THE ISSUE IS A POINTER AND A STATUS, NEVER A COPY.**
  `openspec/changes/<name>/` remains the single source of truth; the issue body
  is **generated from it**, links it and the pull request, and states nothing
  the change directory does not already say. A second copy of a proposal is a
  second thing to keep true, and this project already refuses that shape
  everywhere else it appears.
- **The binding is a REF, stored in the change directory**, so it survives
  archiving and needs no external index. The number is stored; the body never
  is.
- **ONE issue per change, one comment per phase transition** — not a parent
  issue with a child per task group. Changes here run 8 to 68 tasks, so child
  issues would mirror a `tasks.md` that is already the checklist, in a place
  that cannot be ticked.
- **Enforced through the two surfaces this repository already has**, never a
  third: `openspec/config.yaml` `rules:` (injected at the moment an artifact is
  WRITTEN) and a `PreToolUse` hook on the pattern `require-docs-task.sh` set —
  which **fails open** on anything it cannot read, because a hook that blocks
  work it does not understand gets disabled and then enforces nothing.

### The reverse direction: a GitHub issue becomes an openspec change

- **`/opsx:explore` accepts an issue number**, reads it with `gh issue view`,
  and seeds the exploration from what the reporter actually wrote.
- **THE INBOUND ISSUE IS PROMOTED IN PLACE** — relabelled and linked to the
  change it produced — and is never closed in favour of a tracking issue
  authored by the project. The reporter is waiting in that thread, and closing
  it discards both their words and the conversation attached to them.

### Review: Claude reviews every push, and never says the same thing twice

- **`anthropics/claude-code-action@v1` on `pull_request`**, posting inline
  comments on specific lines and one summary, using the standard mechanisms
  (`mcp__github_inline_comment__create_inline_comment`, a sticky summary
  comment).
- **ITERATION IS A CONTEXT PROBLEM, NOT A STATE PROBLEM.**
  `track_progress: true` preserves the pull request's full GitHub context — its
  existing comments included — so **the previous review reaches the next one as
  context**. No sidecar, no findings database and no label machine for review
  state: this change deliberately builds none of them.
- **What the review then does with a finding it already made**: still true →
  **say nothing**; fixed → reply and resolve the thread; no longer applicable →
  reply and resolve.
- **The resolving runs in a SECOND, MODEL-FREE JOB.** GitHub's
  `resolveReviewThread` mutation requires **`contents: write`**, not
  `pull-requests: write` — so the obvious single job would hand a
  model-driven step a token that can push to the repository. Splitting it keeps
  that token out of the model's environment entirely, which is this project's
  own rule: *the component running untrusted model output must not out-rank the
  one orchestrating it.*
- **It aims at the doctrine, not at generic bug-hunting.** The action checks
  the branch out, so `CLAUDE.md` and all seventeen `.claude/rules/` files are
  present. The question worth asking of a diff here is whether it contradicts
  an invariant, a banned term or the change's own delta specs — several rules
  in this repository record decisions that were re-broken two and three times,
  and no other check looks for that.

### Gates: the checks a pull request carries

- **`ci.yml` already runs on `pull_request`**, so its ten jobs already appear.
  That half needs nothing.
- **NEW: openspec is validated in CI.** It appears **nowhere** in any workflow
  today, and `openspec validate --all` currently reports **three failing
  changes** that nothing has ever mentioned.
- **NEW: the documentation-task gate becomes a check.**
  `require-docs-task.sh` runs only on this machine, in one session's harness,
  so the rule it enforces is unenforced for anybody else and for any session
  whose hook is not installed.
- **REQUIRING a check is explicitly OUT OF SCOPE**, and not by preference:
  branch protection and rulesets both answer **403 — "Upgrade to GitHub Pro or
  make this repository public"** on this repository today. `ci.yml`'s
  `ci-green` job was written to be exactly that required check and cannot yet
  be used as one. This change makes the checks correct and complete; requiring
  them follows the public flip or a plan upgrade, and belongs to whichever
  happens first.

## Capabilities

### New Capabilities

- `change-delivery`: how a change reaches `master` — the worktree, the branch,
  the pull request, the merge shape, and the checks that pull request carries.
- `change-issue-tracking`: the GitHub issue that tracks a change's lifecycle —
  what it is allowed to contain, how the binding is stored, how the phase label
  advances, and the reverse direction from an inbound issue to a change.
- `automated-code-review`: what the pull request review does on every push,
  what it must not repeat, what it resolves, and the privilege split that lets
  it resolve anything at all.

### Modified Capabilities

<!-- None. `continuous-integration` and `release-publishing` are declared by
     `sdlc-setup`, which has not archived, so neither exists under
     openspec/specs/ yet and neither can carry a delta. The boundary is stated
     in Impact below and reconciled in design.md. -->

## Impact

### Affected code and configuration

- **New**: `.github/workflows/claude-review.yml` (the two-job review), a
  reconcile script under `.github/scripts/`, a lifecycle hook under
  `.claude/hooks/`, an issue template for a tracked change, and a
  `.claude/rules/` file for the delivery flow.
- **Modified**: `.github/workflows/ci.yml` (the openspec validation job and the
  documentation-task check, plus `ci-green`'s `needs` list),
  `openspec/config.yaml` (the injected rules), `.claude/commands/opsx/*.md`
  (propose opens the issue, apply creates the worktree, archive closes it),
  `.claude/settings.json` (the new hook).
- **No Go code, no CRD, no chart and no API change.** Nothing an adopter
  installs is touched.
- **Repository settings, outside the diff**: the phase labels, and a secret for
  the review's model access.

### Documents this change makes untrue

**The reference docs:**

- `CONTRIBUTING.md` — "How a change is proposed here" and "Pull requests" both
  describe a process that will no longer be the process. This is the page a
  contributor is sent to, so a stale version here is the most expensive one.
- `CLAUDE.md` and `.claude/rules/` — `session-naming.md` (a session now owns a
  worktree), `build-test.md` (the container mount moves), and a new file for
  the delivery flow itself.
- `docs/CHANGELOG.md` — **no entry.** Nothing an adopter runs changes, and an
  entry describing a maintainer's git workflow in the file adopters read for
  upgrade steps would be noise in the one place that must stay signal.
- `.github/PULL_REQUEST_TEMPLATE.md` — it asks for the openspec change name by
  hand, which the branch now states.

**The adopter site:** **unaffected, and this was checked rather than assumed.**
Nothing under `docs/` describes contribution, the openspec workflow or pull
requests — `_data/nav.yml` publishes an overview, an introduction, getting
started, the console, security, installation and the guides, and every one of
them addresses somebody INSTALLING agent-ops rather than changing it. The
reader this change affects arrives at `CONTRIBUTING.md` on the forge, never at
the site.

### Relation to changes already in flight

- **`sdlc-setup` (48/51) owns how artifacts LEAVE the repository** — CI's
  existing shape, the release tags, the registry. This change owns how work
  MOVES THROUGH it. The two new checks sit on that boundary and are declared
  here only because `continuous-integration` does not exist under
  `openspec/specs/` yet; design.md records how they reconcile if `sdlc-setup`
  archives first.
- **`public-exposure` (22/31) owns the issue and pull request templates, the
  repository settings and the public flip.** This change consumes those and
  must not redefine them — and the required-checks half of the gates is left
  waiting on that change's flip.
- **Thirteen changes are in flight, several with no tasks done.** What happens
  to them under the new flow is an open decision recorded in design.md, not
  something this proposal settles by implication.

### Prior art, checked and rejected

OpenSpec's own GitHub-issues RFC (#657) proposed lifecycle hooks and an adapter
and **never shipped**. The one working extension — 104 stars — is a 541-file
parallel workflow replacing the whole `opsx` command set, carrying **no
licence**, whose actual work is done by a closed-source npm CLI. It is readable
for its mechanism and adoptable by nobody. The single borrowable detail is that
it stores its binding in a sidecar inside the change directory, which is what
this change does too.
