# The change lifecycle runs on GitHub: a worktree, a branch, a pull request and an issue

## Why

**The repository is public now, and its development process is the last thing
that has not caught up.** `public-exposure` flipped the switch, `sdlc-setup`
gave it CI and a release path, and branch protection already requires
`ci-green`. What none of that settled is how work gets from an idea to that
pull request — and today the answer is still "in one shared working copy, on
`master`, alone".

**Sessions share one working copy, and it destroys work.** Several sessions run
this repository at once. They share one checkout and therefore one HEAD, which
is why this project's standing instruction has been to commit straight to
`master` and never branch: on 2026-08-23 a session created a branch, a
concurrent session's commit landed on it, the branches diverged, `--ff-only`
refused, and the fix had to be cherry-picked back.

**The rule contained the symptom and left the cause, and the cause is still
biting.** While this very proposal was being written, a concurrent session
cleaned the tree and **deleted the whole change directory** — four unstaged
artifacts, gone, with no branch and no stash to recover them from. Nothing
warned either session. That is the same defect as the diverged branch wearing
different clothes: two sessions, one working copy.

**A worktree per change removes the cause rather than working around it.** Each
session gets its own HEAD and its own files, so branching becomes safe for the
first time — and `session-naming.md` already names every session
`<phase> <change>`. One session, one change, one worktree, one branch, one pull
request completes a model this repository is already half inside.

The rest follows from being able to branch at all: work in flight becomes
visible as an issue, a change becomes reviewable as a pull request, and the
gates this project already asserts get somewhere to report.

## What Changes

### Delivery: a worktree, a branch, a pull request

- **Every openspec change is implemented in its own git worktree**, created at
  `../agent-ops-worktrees/<change>/` and removed when the change is archived.
- **WORKTREES LIVE OUTSIDE THE REPOSITORY, and that is forced rather than
  chosen.** `.github/components.sh` derives what this project ships with
  `find . -name go.mod -mindepth 2` and `find . -name Dockerfile`, so a
  worktree under the root would report **twenty-six** components and hand CI a
  doubled matrix. `structure.md`'s standing test — `find . -type d -name docs`
  returns one line — would break in the same instant.
- **Branch `change/<name>`, one pull request per change**, its title the
  conventional-commit subject the squash merge will carry.
- **BREAKING for how work is done here**: the "commit directly to `master`"
  rule is **rewritten, not merely overridden**. It survives only as the record
  of why it existed, because the argument for it was correct under the
  conditions that produced it and will be re-derived by anyone who meets those
  conditions again.
- **Two prerequisites land OUTSIDE this repository's diff**, and both fail
  silently:
  1. **The build container.** `build-test.md` starts `agentops-go` with
     `-v "$PWD":"$PWD"`, so a worktree at any other path is **invisible inside
     it** — every `go build` in a worktree fails to find the tree. The mount
     moves to the worktrees' parent directory.
  2. **The GitOps deploy.** The repository that deploys this chart names the
     PUBLISHED one, so verifying a branch against a live cluster syncs the
     released chart and reports success — the change looks simply not to work,
     and nothing says the tree was never read. The reference becomes a defaulted
     value overridable per worktree. That edit belongs to the deploying
     repository and is therefore a **precondition of this change, not part of
     it**.

### Tracking: one GitHub issue per change, and it is a POINTER

- **An issue is opened when a change is proposed and closed when it archives**,
  carrying a phase label that advances with the opsx verb driving the work:
  `proposed` → `applying` → `review` → `archived`.
- **THE ISSUE IS A POINTER AND A STATUS, NEVER A COPY.**
  `openspec/changes/<name>/` remains the single source of truth; the issue body
  is **generated from it**, links it and the pull request, and states nothing
  the change directory does not already say. A second copy of a proposal is a
  second thing to keep true, and this project refuses that shape everywhere
  else it appears.
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
  it discards both their words and the conversation attached to them. **The
  repository is public, so this stopped being hypothetical the day it flipped.**

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
  `pull-requests: write` — so the obvious single job would hand a model-driven
  step a token that can push to the repository. Splitting it keeps that token
  out of the model's environment, which is this project's own rule: *the
  component running untrusted model output must not out-rank the one
  orchestrating it.*
- **A FORK'S PULL REQUEST GETS NO REVIEW, and that is stated rather than
  discovered.** A public repository takes contributions from forks, where
  `GITHUB_TOKEN` is read-only and repository secrets are unavailable by design.
  The review is skipped there explicitly, so a contributor sees a skipped check
  rather than a red one they cannot fix.
- **It aims at the doctrine, not at generic bug-hunting.** The action checks the
  branch out, so `CLAUDE.md` and every `.claude/rules/` file is present. The
  question worth asking of a diff here is whether it contradicts an invariant, a
  retired term or the change's own delta specs — several rules in this
  repository record decisions that were re-broken two and three times, and no
  other check looks for that.

### Gates: two checks the project asserts and never ran

- **NEW: openspec is validated in CI.** It appears **nowhere** in any workflow,
  and `openspec validate --all` reports failing changes that nothing has ever
  mentioned. `openspec/specs/` is the published answer to "is this behaviour
  intended" and nothing checks that it parses.
- **NEW: the documentation-task gate becomes a check.**
  `require-docs-task.sh` runs only in one contributor's local harness, so the
  rule it enforces is unenforced for everybody else — and the repository is now
  open to everybody else.
- **BOTH JOIN `ci-green`'s `needs`, and are therefore REQUIRED without touching
  branch protection.** That is the whole point of the always-present gate
  `sdlc-setup` built: a protection rule names one check, and everything real
  reports through it. Adding a required gate is a line in a `needs:` list, not a
  settings change somebody has to remember.

## Capabilities

### New Capabilities

- `change-delivery`: how a change reaches `master` — the worktree, the branch,
  the pull request and the merge shape.
- `change-issue-tracking`: the GitHub issue that tracks a change's lifecycle —
  what it may contain, how the binding is stored, how the phase advances, and
  the reverse direction from an inbound issue to a change.
- `automated-code-review`: what the pull request review does on every push,
  what it must not repeat, what it resolves, and the privilege split that lets
  it resolve anything at all.

### Modified Capabilities

- `continuous-integration`: two gates are added — the openspec artifacts are
  validated, and a change's documentation task must be complete — both reporting
  through the existing always-present gate rather than as new required checks.

## Impact

### Affected code and configuration

- **New**: `.github/workflows/claude-review.yml` (the two-job review), a
  reconcile script and a documentation-task check under `.github/scripts/`, a
  lifecycle hook under `.claude/hooks/`, a tracking-issue template, and a
  `.claude/rules/` file for the delivery flow.
- **Modified**: `.github/workflows/ci.yml` (two jobs plus `ci-green`'s `needs`),
  `openspec/config.yaml`, `.claude/commands/opsx/*.md` (propose opens the issue,
  apply creates the worktree, explore reads an issue, archive closes it),
  `.claude/settings.json`.
- **No Go code, no CRD, no chart and no API change.** Nothing an adopter
  installs is touched.
- **Repository settings, outside the diff**: the phase labels, and a secret for
  the review's model access.

### Documents this change makes untrue

**The reference docs:**

- `CONTRIBUTING.md` — "How a change is proposed here" and "Pull requests" both
  describe a process that will no longer be the process. **This is now the page
  a stranger is sent to**, so a stale version here is the most expensive one in
  the repository.
- `CLAUDE.md` and `.claude/rules/` — `session-naming.md` (a session now owns a
  worktree), `build-test.md` (the container mount moves), and a new file for the
  delivery flow.
- `.github/PULL_REQUEST_TEMPLATE.md` — it asks for the openspec change name by
  hand, which the branch now states.
- `.github/retired-vocabulary.json` — "commit directly to master" is retired by
  this change and must fail the build if it returns as a current claim.
- `docs/CHANGELOG.md` — **no entry.** Nothing an adopter installs or runs
  changes, and a maintainer's git workflow in the file adopters read for upgrade
  steps is noise in the one place that must stay signal.

**The adopter site:** **unaffected, and this was checked rather than assumed.**
Nothing under `docs/` describes contribution, the openspec workflow or pull
requests — `_data/nav.yml` publishes an overview, an introduction, getting
started, the console, security, installation and the guides, and every one of
them addresses somebody INSTALLING agent-ops rather than changing it. The reader
this change affects arrives at `CONTRIBUTING.md` on the forge, never at the site.

### What has already shipped, and is therefore NOT in scope

- **`sdlc-setup` and `public-exposure` are archived.** CI, the release path, the
  templates, the community health files and the public flip are done. This
  change consumes all of it.
- **Branch protection exists and requires `ci-green`.** An earlier draft of this
  proposal scoped required checks OUT, because the API answered *403 — upgrade
  to GitHub Pro or make this repository public*. The repository is public now
  and the rule is in place, so the scope shrinks: the work is to make the new
  gates report through the existing required check, not to argue for one.
- **Eleven changes remain in flight**, and each gets an issue and a branch.

### Prior art, checked and rejected

OpenSpec's own GitHub-issues RFC proposed lifecycle hooks and an adapter and
**never shipped**. The one working extension is a 541-file parallel workflow
replacing the whole `opsx` command set, carrying **no licence**, whose actual
work is done by a closed-source npm CLI. It is readable for its mechanism and
adoptable by nobody. The single borrowable detail is that it stores its binding
in a sidecar inside the change directory, which is what this change does too.
