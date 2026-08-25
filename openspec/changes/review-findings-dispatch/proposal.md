## Why

**The review posts findings and nothing can act on them from the pull request.**
It reads, it comments, and there the loop ends: every fix is somebody opening an
editor, and every thread is somebody clicking resolve. The reviewer that took a
day to build is, from the pull request's point of view, write-only.

Two things have just made that gap load-bearing rather than merely
inconvenient:

- **Branch protection now requires conversation resolution before merging.** An
  unresolved thread blocks the merge, so the cost of every finding is now paid
  by hand at exactly the moment somebody wants to land the change.
- **The review is finally running for real**, so findings arrive on ordinary
  pull requests rather than in test branches.

**AND FIVE VERIFICATION TASKS CANNOT BE FINISHED WITHOUT IT.**
`github-change-lifecycle` §7.5, 7.6, 7.8 and 7.9 each need a pull request where
one finding is addressed and another is deliberately left — the state a triage
loop produces as a matter of course, and which was otherwise reachable only by
writing a defect into the repository on purpose. That was attempted in this
session and refused, correctly, by the harness.

## What Changes

- **TRIAGE HAPPENS IN THE THREAD, IN THE READER'S OWN WORDS.** A maintainer
  replies `fix it` under a finding they accept. Anything else — a question, an
  argument, silence — is not an acceptance, and the thread is left exactly as it
  is.
- **THE ACCEPT VOCABULARY IS MECHANICAL AND WRITTEN DOWN**, never interpreted. A
  fixed set of phrases, matched by a program. What decides that code will be
  written to a branch may not be a judgement call.
- **ONE DISPATCH, NOT ONE PER FINDING.** A single pull-request comment starts
  the work, and everything accepted is fixed in one commit — rather than N
  mention-triggered runs racing each other on one branch.
- **THE WORK LIST IS DERIVED FROM THE THREADS, NEVER FROM THE DISPATCH
  COMMENT.** A program walks the review threads and collects the accepted ones,
  carrying each finding, its file and line, and the human's reply. The comment
  is a trigger, not an instruction.
- **THE FIXING STEP CANNOT PUSH.** It runs under `contents: read` and emits a
  PATCH as an artifact. A separate step, holding the write privilege and running
  no model, applies it, pushes it, replies in each thread naming the commit, and
  hands the thread ids to the resolver that already exists.
- **A DISPATCH IS AUTHORISED BY WHO SENT IT** — write access, established from
  the comment's author association. Forks are refused, and no path uses
  `pull_request_target`.
- **A FINDING NOBODY ACCEPTED KEEPS ITS THREAD OPEN**, and therefore keeps the
  merge blocked until a person deals with it. That is the feature.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `automated-code-review`: gains the triage-and-dispatch loop — what counts as
  an acceptance, what a dispatch is authorised by, that the work list comes from
  the threads, and that the step which writes code is not the step which pushes
  it. Its existing requirement *What may close a thread holds no power to change
  the code* is EXTENDED rather than weakened: the same split now covers the step
  that produces the fix.

**NOTE ON ORDERING.** `automated-code-review` is introduced by
`github-change-lifecycle`, which is merged but NOT YET ARCHIVED, so the
capability is not in `openspec/specs/` yet. This change's delta is written
against it and lands after it. If the two are archived out of order the delta
must be re-checked rather than assumed.

## Impact

**Code**

- `.github/workflows/` — a new comment-triggered workflow with the two-step
  split. `claude-review.yml` is untouched: a pull request may not rewrite the
  review that judges it, and the same argument applies to the thing that fixes
  its findings.
- `.github/scripts/` — the thread-walking program that produces the work list,
  and the applier. `resolve-review-threads.py` is REUSED, not replaced: it
  already refuses any thread the review did not author.
- `.github/tests/` — both new programs get suites, in the style of the existing
  ones: the refusals are the product.

**Reference docs**

- `.claude/rules/` — the triage vocabulary and the dispatch form are things a
  contributor types, so they belong in a rule rather than only in a workflow
  comment. `worktree-delivery.md` describes how a change reaches master and now
  gains the answer to "the review found something, now what".
- `CONTRIBUTING.md` — same reason, for the reader who never opens `.claude/`.

**The adopter site**

- Nothing. No page under `docs/` describes contribution, review or pull
  requests, and every page in `_data/nav.yml` addresses somebody INSTALLING
  agent-ops. The reader who is unaffected is the adopter: none of this reaches a
  cluster, a chart value or a CRD.

**Not affected**

- `docs/CHANGELOG.md` — nothing an adopter installs or runs changes.
- Every Go module, the chart, and the published images.
