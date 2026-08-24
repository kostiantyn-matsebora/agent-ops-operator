# Design — the change lifecycle on GitHub

## Context

See `proposal.md` — Why. What matters here is the ground the design stands on,
established by inspection rather than assumed. **Several of these facts changed
while this change was being planned**, which is itself the reason each is
recorded with how it was checked:

| Fact | Consequence |
|---|---|
| The repository is **public**, and branch protection **requires `ci-green`** | a new gate is a line in `ci-green`'s `needs:`, not a settings change. An earlier draft scoped required checks OUT because the API answered 403 |
| `sdlc-setup` and `public-exposure` are **archived** | CI, the release path, the templates and the community health files are done and are consumed, not redefined |
| `continuous-integration` now EXISTS under `openspec/specs/` | the two new gates are a delta on it, not requirements smuggled into a new capability |
| `.github/components.sh` derives the inventory with `find . -name go.mod -mindepth 2` and `find . -name Dockerfile` | a worktree inside the tree doubles the inventory. **Placement is forced** |
| `openspec` appears in no workflow; `openspec validate --all` reports failing changes | a real gap, and the cheapest check on the list |
| `.claude/hooks/require-docs-task.sh` is a local `PreToolUse` hook that FAILS OPEN | the pattern to copy, and the reason its decision must also exist in CI |
| `openspec/config.yaml` `rules:` are injected when an artifact is WRITTEN | the second enforcement surface, reaching the model when a context file may already be compacted away |
| `build-test.md` starts `agentops-go` with `-v "$PWD":"$PWD"` | a worktree elsewhere is invisible inside the build container |
| The deploying repository names the PUBLISHED chart | verifying a branch deploys the RELEASED chart and reports success — it never reads the tree at all |

**Two of those fixes are not in this repository**, and both fail silently — which
is why they are tasks with verification by USE rather than notes.

## Goals / Non-Goals

**Goals**

- Make a branch per change SAFE, by removing the shared working copy that made
  it unsafe.
- Give work in flight one visible place per change, without creating a second
  copy of the change.
- Close the two gates this project asserts and has never run.
- Make the review iterative — additive on each push, never repetitive.

**Non-Goals**

- **Redefining CI's existing shape, the release path, the templates, the
  repository settings or the public flip.** All of it shipped; this change
  consumes it.
- **Replacing the `opsx` workflow.** The commands gain steps; the schema,
  artifacts and vocabulary are untouched.
- **Reviewing anything other than pull requests**, and **automating merge**.
- **Making the review work on fork pull requests.** It cannot, by design, and
  the design says so rather than leaving a red check.

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
**anything derivable is derived.**

### D3 — Squash merge, one commit per change

`CONTRIBUTING.md`: *"`git log --oneline` is meant to read as an account of the
project."* One change is one line of that account. A merge commit adds a line
that describes nothing; a rebase of a change's working history adds twenty that
describe the writing rather than the change.

**Consequence, and the reason this is a decision rather than a setting:** the
pull request TITLE becomes the commit subject, so it must satisfy the commit
convention. That is a check.

### D4 — `openspec archive` runs INSIDE the pull request

The archive folds the delta specs into `openspec/specs/`. Done inside the pull
request, **the diff shows the contract changing** — precisely what a reviewer of
this project should be looking at, since `openspec/specs/` is the answer to "is
this behaviour intended".

*Alternative rejected:* archiving on `master` after merge. It keeps the pull
request smaller, but the spec change then reaches the published contract having
been reviewed by nobody, and it puts direct commits back on the default branch in
the one change whose purpose is to stop them.

*Accepted cost:* an archive commit that must reflect review feedback needs an
amend or a follow-up commit on the branch. Cheap, and visible.

### D5 — The issue is a POINTER; the binding is a REF in the change directory

The generated body holds: what the change is in one line, a link to the change
directory, a link to the pull request, the phase, and nothing else. **Sections
are few and fixed** so a reader learns the shape once.

The number is stored in the change directory — a sidecar, so it survives
archiving and needs no index. The repository's own rule: *REFS are snapshotted,
CONTENT is not.* Storing the number means every lookup is current; storing what
the issue SAYS means storing something that can rot with no mechanism to notice.

*Alternative rejected:* deriving the issue by searching for the change name. No
stored state, but a fuzzy match that gets slower and less certain as the issue
list grows, and which fails by finding the wrong one.

*Alternative rejected:* parent issue plus a child per task group — what the one
working prior-art extension does. Changes here run 8–68 tasks. Dozens of child
issues would mirror a `tasks.md` that is already the checklist, in a place that
cannot be ticked, and which must then be kept in step with the file that can.

### D6 — Enforcement uses the surfaces that already exist, and adds no new kind

| Surface | Enforces | Runs |
|---|---|---|
| `openspec/config.yaml` `rules:` | what a generated artifact must contain | in the model, as it writes the file |
| a `PreToolUse` hook | refuses a command whose precondition is unmet | in the harness |
| a CI check | the same decision, for everyone whose harness this is not | on the pull request |

The hook **fails open** on anything it cannot read. Copied deliberately: *a hook
that blocks work it does not understand gets disabled, and then it enforces
nothing at all.* The CI check is what makes failing open safe, because the
decision is asserted a second time where it cannot be skipped.

**The two must not drift**, so they share fixtures and a divergence fails a test.

### D7 — Iterative review is a CONTEXT problem, and this design builds no state

`track_progress: true` preserves the pull request's full GitHub context,
existing comments included. **The previous review therefore reaches the next one
as context.** No findings store, no sidecar, no labels standing in for review
state — and this is written down because the obvious design is to build one.

An earlier sketch did exactly that: read every review thread back through
GraphQL, classify each, reconstruct the previous review. It works, and it is
redundant — a second record of something the pull request already holds, wrong
precisely when a run fails part-way. **It is recorded here so it is not
re-derived.**

What remains prompt-level: still true → say nothing; fixed → reply and resolve;
gone → reply and resolve.

### D8 — Two jobs, because resolving a thread requires `contents: write`

Verified: GitHub's `resolveReviewThread` mutation is gated behind repository
**Contents**, not Pull requests — counter-intuitive, since it changes no file.

So the single obvious job would hold a token that can push while running
generated output. Split instead:

```
review           contents: read, pull-requests: write
                 reads the diff, posts inline comments and the summary,
                 emits the thread ids it wants resolved
   │  (a list of thread ids — the only thing crossing the boundary)
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
holds, and the old thread closed as superseded. Never resolved for being
detached.

### D10 — A human-resolved thread is honoured, and counted

Deferring to the person is right. Doing it silently is not: a finding resolved
rather than fixed has left the reader's view, and a summary that omits it reads
as though everything raised was dealt with. The sticky summary carries the count
— the project's *telemetry must REPORT its gaps* rule.

### D11 — The review is aimed at the doctrine

The action checks the branch out, so `CLAUDE.md` and every `.claude/rules/` file
is present. Generic bug-and-security review is available from many tools; **a
reading against this project's recorded invariants and retired vocabulary is
available from none of them**, and several rules here record decisions that were
re-made, reverted and re-made again — a class of regression that compiles,
passes every test, renders every chart, and is visible only to a reader who
knows what was decided before.

The two mechanical guards already in CI (`publication`, `retired-vocabulary`) are
the deterministic half of the same job, which is why the review skips when either
fails: those two mean the diff contains something on its way out, and reviewing
it wastes a run on content about to be deleted.

### D12 — Authentication is `claude_code_oauth_token`

Uses the existing Claude subscription rather than metered API billing. Every push
to every open pull request triggers a review, so per-token billing on an
unattended trigger is the shape most likely to produce a surprise.

*Accepted cost:* the token expires and must be refreshed as a repository secret,
a manual step with no automated warning.

### D13 — Fork pull requests are SKIPPED, explicitly

The repository is public, so pull requests arrive from forks — where
`GITHUB_TOKEN` is read-only and repository secrets are withheld by design. The
review cannot run there, and neither can `reconcile`.

**Skipped, not failed.** A red check an outside contributor cannot turn green
reads as "your change is broken" when nothing about their change is. The
condition is explicit (`pull_request.head.repo.fork`) rather than left to the
job failing on a missing secret, because those two look identical in the checks
list and mean opposite things.

### D14 — The new gates report through `ci-green`, not as new required checks

`continuous-integration` already requires exactly one always-present check, and
branch protection already names it. Adding the two gates to `ci-green`'s `needs:`
makes them required **by construction** — no protection edit, and nothing for
anybody to remember when the next gate is added.

This is also why they belong to `continuous-integration` rather than to
`change-delivery`: the boundary is *what is being judged*, and both judge whether
a change may land.

### D15 — All eleven in-flight changes migrate

Each existing change gets an issue and a branch when this lands.

*Alternative rejected:* new changes only — cheaper, but it leaves the majority of
what is in flight invisible in the tracking this change exists to create.

*Alternative rejected:* migrate on next touch — no big bang, but two flows run
side by side with no end date, and the one thing worse than an old process is
two.

**Scripted, not hand-run eleven times.** Eleven repetitions of a manual sequence
is where one gets done differently and nobody notices which.

## Risks / Trade-offs

| Risk | Mitigation |
|---|---|
| **A worktree deploys `master`'s chart and reports success**, so every change's verification silently tests the wrong tree | The chart path is templated and overridden per worktree, and the task VERIFIES the override by rendering from a worktree and diffing — not by reading the command back |
| **The build container cannot see the worktree**, and `go build` fails in a way that reads as a broken module | The mount moves to the worktrees' parent, and `build-test.md` is corrected in the same change |
| **The commit hook blocks other sessions.** Once migration gives every change a branch, a session still working on `master` starts being refused | The hook is narrow — it refuses only a commit touching a change that owns a branch — and fails open otherwise. The coordination cost is real and is called out in the migration task rather than discovered |
| **The review resolves a human's thread** | The author check is in the model-free job, which refuses any thread whose first comment is not the review's own |
| **The review becomes noise and gets ignored**, which is how review automation usually dies | Skip drafts and forks; cancel superseded runs; skip when the hygiene guards fail; and the no-repetition requirement is the point of the whole capability |
| **Eleven migrated branches go stale** | The migration creates a branch and an issue, not a rebase schedule. A stale branch is rebased when its change is next worked — the same cost it would have had anyway |
| **The OAuth token expires and reviews stop** | The workflow fails visibly on the pull request rather than silently not running. Which surface warns EARLIER is an open question below |
| **A fork contributor sees no review at all** | Skipped rather than failed, and visibly so, so a maintainer can tell an unreviewed pull request from a reviewed one |

## Migration Plan

1. **Preconditions, outside this repository's diff**: the deploying repository's
   chart reference is templated, and the build container mounts the worktrees'
   root beside the repository. Both verified by use, never by reading the
   command back.
2. **Labels and the secret** exist before anything references them.
3. **The flow lands** — commands, hook, workflows, checks — and is exercised end
   to end on ONE change first. This change is that change.
4. **The eleven migrate**, scripted.
5. **The documentation follows**, last, describing what actually happened.

**Rollback** is per layer and none of it destructive: workflows deleted, the hook
unwired, the commands reverted, worktrees and branches removed with git. The
issues would remain and be closed by hand — the only part that leaves a trace,
which is why the label set is small.

## Open Questions

- **Which surface warns that the OAuth token is near expiry?** Deferrable: the
  failure is already visible as a red check, so nothing is lost while this is
  unanswered — it only decides whether the warning arrives earlier.
- **Whether a fork's pull request should get a review through a
  `pull_request_target` variant.** Deferrable and deliberately not taken now:
  that trigger runs workflow code from the base branch with a writable token
  against untrusted head code, which is a security decision deserving its own
  change rather than a footnote in this one.
