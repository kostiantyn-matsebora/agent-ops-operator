# Design — the change lifecycle on GitHub

## Context

See `proposal.md` — Why. What matters here is the ground the design stands on,
all of it established by inspection rather than assumed:

| Fact | Consequence for this design |
|---|---|
| `.github/components.sh` derives the inventory with `find . -name go.mod -mindepth 2` and `find . -name Dockerfile` | a worktree inside the tree doubles the inventory. **Placement is forced** |
| `ci.yml` already triggers on `pull_request` and ends in an aggregating `ci-green` job | the checks half is largely built; only two are missing |
| Branch protection and rulesets answer **403 — "Upgrade to GitHub Pro or make this repository public"** | nothing can be made a REQUIRED check yet. `ci-green` was written to be one |
| `openspec` appears in no workflow; `openspec validate --all` reports **3 failing changes** | a real gap, and the cheapest check on the list |
| `.claude/hooks/require-docs-task.sh` is a local `PreToolUse` hook that FAILS OPEN | the pattern to copy, and the reason its decision must also exist in CI |
| `openspec/config.yaml` `rules:` are injected when an artifact is WRITTEN | the second enforcement surface, and it reaches the model when a context file may already be compacted away |
| `build-test.md` starts `agentops-go` with `-v "$PWD":"$PWD"` | a worktree elsewhere is invisible inside the build container |
| helmfile's chart path is the literal `../../../../../../agent-ops-operator/chart` | verifying a branch would deploy `master`'s chart and report success |

**Two of those fixes are not in this repository.** The container mount is a
documented command in `build-test.md`, and the helmfile path lives in
`home-data-center`. Both are preconditions, and both fail silently if skipped —
which is why they are tasks with verification steps rather than notes.

## Goals / Non-Goals

**Goals**

- Make a branch per change SAFE, by removing the shared HEAD that made it unsafe.
- Give work in flight one visible place per change, without creating a second
  copy of the change.
- Make every gate the project asserts visible on the pull request.
- Make the review iterative — additive on each push, never repetitive.

**Non-Goals**

- **Making any check REQUIRED.** Impossible on this plan; it follows the public
  flip (`public-exposure`) or an upgrade.
- **Redefining the issue or pull request templates**, the repository settings or
  the public flip — `public-exposure` owns those.
- **Redefining CI's existing shape or the release path** — `sdlc-setup` owns those.
- **Replacing the `opsx` workflow.** The commands gain steps; the schema,
  artifacts and vocabulary are untouched.
- Automating merge, or reviewing anything other than pull requests.

## Decisions

### D1 — Worktrees at `../agent-ops-worktrees/<change>/`, outside the repository

**Forced, not chosen.** Inside the tree, the derived inventory reports 26
components instead of 13 and `find . -type d -name docs` stops returning one
line. Both failures are silent and both look like something else: the matrix
grows, and the cause reads as a new component rather than a working copy.

*Alternatives considered.* `.worktrees/` inside the root with an ignore entry —
`.gitignore` does not affect `find`, so the inventory breaks anyway. A sibling
inside `~/.cache/` — works, and is what the existing build worktree does, but
buries active work somewhere nobody browses. A sibling of the repository keeps
worktrees next to the thing they are copies of.

### D2 — Branch `change/<name>`, and the branch is the statement of provenance

The change name is already the worktree name and the session name. Making it the
branch name too means the pull request template stops asking for it by hand:
**anything derivable is derived.** The template's "openspec change:" field
becomes a check rather than a question.

### D3 — Squash merge, one commit per change

`CONTRIBUTING.md`: *"`git log --oneline` is meant to read as an account of the
project."* One change is one line of that account. A merge commit adds a line
that describes nothing, and a rebase of a change's whole working history adds
twenty that describe the writing rather than the change.

**Consequence, and it is the reason this is a decision rather than a setting:**
the pull request TITLE becomes the commit subject, so it must satisfy the commit
convention. That is a check, listed in the tasks.

### D4 — `openspec archive` runs INSIDE the pull request

The archive folds the delta specs into `openspec/specs/`. Done inside the pull
request, **the diff shows the contract changing** — which is precisely what a
reviewer of this project should be looking at, since `openspec/specs/` is the
answer to "is this behaviour intended".

*Alternative rejected:* archiving on `master` after merge. It keeps the pull
request smaller, but the spec change then reaches the published contract having
been reviewed by nobody, and it puts direct commits back on the default branch
in the one change whose purpose is to stop them.

*Accepted cost:* an archive commit that needs to reflect review feedback needs an
amend or a follow-up commit on the branch. Cheap, and visible.

### D5 — The issue is a POINTER; the binding is a REF in the change directory

The issue body is generated and holds: what the change is (one line, from the
proposal's title), a link to the change directory, a link to the pull request,
the phase, and nothing else. **Sections are few and fixed** so a reader learns
the shape once.

The number is stored in the change directory — a sidecar file, so it survives
archiving and needs no index. This is the repository's own rule: *REFS are
snapshotted, CONTENT is not.* Storing the number means every lookup is current;
storing anything the issue SAYS means storing something that can rot with no
mechanism to notice.

*Alternative rejected:* deriving the issue by searching for the change name. No
stored state, but it is a fuzzy match that gets slower and less certain as the
issue list grows, and it fails silently by finding the wrong one.

*Alternative rejected:* parent issue plus a child per task group, which is what
the one working prior-art extension does. Changes here run 8–68 tasks. Dozens of
child issues would mirror a `tasks.md` that is already the checklist, in a place
that cannot be ticked, and which then has to be kept in step with the file that
can.

### D6 — Enforcement uses the two surfaces that already exist, and adds no third

| Surface | Enforces |
|---|---|
| `openspec/config.yaml` `rules:` | what a generated artifact must contain — reaches the model at the moment it writes the file |
| a `PreToolUse` hook, on the `require-docs-task.sh` pattern | refuses a command whose precondition is unmet — runs in the harness, not the model |
| **CI check** | the same decision, for everyone whose harness this is not |

The hook **fails open** on anything it cannot read. That is copied deliberately:
*a hook that blocks work it does not understand gets disabled, and then it
enforces nothing at all.* The CI check is what makes failing open safe, because
the decision is asserted a second time where it cannot be skipped.

### D7 — Iterative review is a CONTEXT problem, and this design builds no state

`track_progress: true` preserves the pull request's full GitHub context,
existing comments included. **The previous review therefore reaches the next one
as context.** No findings store, no sidecar, no labels standing in for review
state — and this is written down because the obvious design is to build one.

An earlier sketch in exploration did exactly that: read every review thread back
through GraphQL, classify each, and reconstruct the previous review. It works,
and it is redundant — a second record of something the pull request already
holds, wrong precisely when a run fails part-way. **It is recorded here so it is
not re-derived.**

What remains prompt-level, on top of that context: still true → say nothing;
fixed → reply and resolve; gone → reply and resolve.

### D8 — Two jobs, because resolving a thread requires `contents: write`

Verified: GitHub's `resolveReviewThread` mutation is gated behind repository
**Contents**, not Pull requests — counter-intuitive, since it changes no file.

So the single obvious job would hold a token that can push to the repository
while running generated output. Split instead:

```
review           contents: read, pull-requests: write
                 reads the diff, posts inline comments and the summary,
                 emits the thread ids it wants resolved
   │  (list of thread ids — the only thing crossing the boundary)
   ▼
reconcile        contents: write
                 NO MODEL. Resolves exactly those ids, and refuses any
                 thread the review did not author.
```

The project's own rule, one layer out: *the component running untrusted model
output must not out-rank the one orchestrating it.*

**The author check lives in `reconcile`, not in the prompt.** Resolving a human
reviewer's thread is the one failure here that destroys information rather than
adding noise — it hides a person's objection and reports it as handled — so the
constraint is placed where it cannot be talked out of.

### D9 — Detachment is not resolution

GitHub detaches an inline comment when its anchor line changes. A reformat
detaches a live finding; a fix elsewhere leaves a dead one attached. **The anchor
says nothing about the code while looking exactly as though it does**, and a
detached comment is collapsed — invisible to the reader.

So a detached thread is re-checked, re-raised at its current location if it still
holds, and the old thread closed as superseded. It is never resolved for being
detached.

### D10 — A human-resolved thread is honoured, and counted

Deferring to the person is right. Doing it silently is not: a finding resolved
rather than fixed has left the reader's view, and a summary that omits it reads
as though everything raised was dealt with. The sticky summary carries the count
— the project's *telemetry must REPORT its gaps* rule.

### D11 — The review is aimed at the doctrine

The action checks the branch out, so `CLAUDE.md` and all seventeen
`.claude/rules/` files are present in the working directory. Generic
bug-and-security review is available from many tools; **a reading against this
project's recorded invariants and retired vocabulary is available from none of
them**, and several rules here record decisions that were re-made, reverted and
re-made again — a class of regression that compiles, passes every test, renders
every chart, and is visible only to a reader who knows what was decided before.

The two mechanical guards already in CI (`publication`, `retired-vocabulary`) are
the deterministic half of the same job, which is why the review skips when either
fails: those two mean the diff contains something on its way out, and reviewing
it wastes a run on content that is about to be deleted.

### D12 — Authentication is `claude_code_oauth_token`

Uses the existing Claude subscription rather than metered API billing. Every push
to every open pull request triggers a review, so per-token billing on an
unattended trigger is the shape most likely to produce a surprise.

*Accepted cost:* the token expires and must be refreshed as a repository secret.
That is a manual step with no automated warning, so the failure mode is a review
that silently stops running — the tasks therefore include a note on where that
surfaces.

### D13 — CI ownership boundary with `sdlc-setup`

`continuous-integration` is declared by `sdlc-setup`, which has not archived, so
it does not exist under `openspec/specs/` and cannot carry a delta. The two new
checks are therefore requirements of `change-delivery`.

**If `sdlc-setup` archives first**, they stay where they are: they gate the
CHANGE LIFECYCLE (are the artifacts valid, is the documentation task done)
rather than the build, and `continuous-integration` is about what must pass for
the CODE. The boundary is *what is being judged*, not *which workflow file runs
it*.

### D14 — All thirteen in-flight changes migrate

Each existing change gets an issue and a branch when this lands.

*Alternative rejected:* new changes only. Cheaper, but it leaves thirteen pieces
of work — the majority of what is in flight — invisible in the tracking this
change exists to create, for as long as they take.

*Alternative rejected:* migrate on next touch. No big bang, but two flows run
side by side with no end date, and the one thing worse than an old process is
two.

**The migration is scripted, not hand-run thirteen times.** Thirteen repetitions
of a manual sequence is where one gets done differently and nobody notices which.

## Risks / Trade-offs

| Risk | Mitigation |
|---|---|
| **A worktree deploys `master`'s chart and reports success.** The verification step of every change silently tests the wrong tree | The helmfile path is templated and overridden per worktree, and the task VERIFIES the override by rendering from a worktree and diffing — not by reading the command |
| **The build container cannot see the worktree**, and `go build` fails in a way that reads as a broken module | The mount moves to the worktrees' parent, and `build-test.md` is corrected in the same change. This is documentation whose staleness costs a debugging session |
| **The review resolves a human's thread** | The author check is in the model-free job. Belt and braces: `reconcile` refuses any thread whose first comment is not the review's own |
| **The review becomes noise and gets ignored**, which is how review automation usually dies | Skip drafts; cancel superseded runs; skip when the two hygiene guards fail; and the no-repetition requirement is the point of the whole capability |
| **Thirteen migrated branches go stale** and diverge from `master` | The migration creates the branch and the issue, not a rebase schedule. A stale branch is rebased when its change is next worked, which is the same cost it would have had anyway |
| **The OAuth token expires and reviews stop silently** | The workflow's own failure is visible on the pull request as a failed check rather than as a missing one |
| **Checks are advisory until the public flip.** Nothing stops a merge over a red check | Stated in the proposal as out of scope rather than left as an implied promise. `ci-green` is already shaped to be the required check the day the plan permits one |
| **Two enforcement surfaces disagree** — the local hook passes and the CI check fails, or the reverse | Both read the same file and apply the same rule; the task pins them against shared fixtures so a divergence fails a test rather than a pull request |

## Migration Plan

1. **Preconditions, outside this repository's diff:** the helmfile chart path is
   templated, and the build container's mount moves to the worktrees' parent.
   Both are verified by use, not by reading.
2. **Labels and the secret** exist before anything references them.
3. **The flow lands** — commands, hook, workflows, checks — and is exercised end
   to end on ONE small change first. This change itself is the obvious
   candidate.
4. **The thirteen migrate**, scripted.
5. **The documentation follows**, last, describing what actually happened.

**Rollback** is per layer and none of it is destructive: the workflows can be
deleted, the hook removed from settings, the commands reverted. Worktrees and
branches are removed with git. The issues would remain and would be closed by
hand — the only part of this that leaves a trace, which is why the label set is
small.

## Open Questions

- **Which surface warns that the OAuth token is near expiry?** Deferrable: the
  failure is visible as a red check on a pull request, so nothing is lost while
  this is unanswered — it only decides whether the warning arrives earlier.
- **Whether the review should also run on `master` pushes** for the period before
  checks can be required. Deferrable: it changes a trigger, not the specs, the
  approach or the task breakdown.
