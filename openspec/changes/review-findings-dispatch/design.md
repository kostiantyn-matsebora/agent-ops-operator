## Context

See `proposal.md` — Why. What shapes the approach is what already exists:
`claude-review.yml` runs the review under `contents: read` and a second job,
`reconcile`, holds `contents: write`, runs no model, and resolves only the thread
ids it is handed by `resolve-review-threads.py` — which re-reads every thread and
REFUSES any whose first comment is not the review's.

That split is a published requirement (*What may close a thread holds no power
to change the code*), and it is the constraint this design has to satisfy for a
step whose whole purpose is to change code.

The other fixed point: branch protection now requires conversation resolution
before merging, so an open thread is a blocked merge.

## Goals / Non-Goals

**Goals:**

- Triage stays where the reading happens — in the thread, in the reader's words.
- One dispatch, one commit, one review re-run.
- The model never holds push access.
- Reuse `resolve-review-threads.py` rather than growing a second thing that
  closes threads.

**Non-Goals:**

- **A general "@claude do X" bot.** Anything not derived from an accepted
  finding is out of scope, deliberately: the moment the dispatch text becomes
  the instruction, the authorisation check is protecting the wrong sentence.
- **Automatic dispatch on every finding.** A review that fixed itself would push
  to a branch nobody asked it to touch, and the triage step is exactly where a
  person decides the finding is right.
- **Fixing a finding a person argued with.** The accept vocabulary is the whole
  interface; a thread that reads as agreement without saying so is not one.
- Touching `claude-review.yml`. A pull request may not rewrite the review that
  judges it, and the same holds for what acts on its findings.

## Decisions

**THE ACCEPT VOCABULARY IS A LIST, NOT A CLASSIFIER.** A small documented set —
`fix it`, `fix`, `fix this`, `agreed, fix` — matched case-insensitively against
the reply's whole text, ignoring surrounding whitespace and a trailing full stop.

*Alternative rejected:* let the model read each thread and decide what the human
meant. It reads as friendlier and it puts a judgement call in front of writing to
a branch. A reader who typed "sure, if you think so" would find code changed on
one day and not another, with nothing to point at.

**THE TRIGGER AND THE INSTRUCTION ARE DIFFERENT SENTENCES.** The dispatch comment
authorises; the thread list instructs. Keeping them apart is what stops a
maintainer's authorisation being spent on work nobody reviewed — including by a
maintainer who did not intend it.

**THE PATCH CROSSES AS AN ARTIFACT, EXACTLY AS `.resolve-threads` DOES.** The
existing pipe between a `contents: read` job and a privileged one is an uploaded
file, and it is now known to work — including the dotfile trap that made it a
no-op for a day. The applier does `git apply`, commits with a message naming the
findings, and pushes.

*Alternative rejected:* give the fixing job `contents: write` and let it commit
directly. One job instead of two, and it makes the model a committer to the
branch — which is the arrangement the existing requirement was written to
prevent one layer in.

**THE APPLIER IS THE ONE THAT REPLIES AND RESOLVES**, because it alone knows the
commit sha, and because a reply saying "fixed in <sha>" written before the push
is a claim rather than a report.

**A THREAD IS RESOLVED ONLY WHERE ITS PATCH LANDED.** If the patch fails to
apply, nothing is resolved and the dispatch says so in the thread. A resolved
thread means the code changed.

**NO PROTOTYPE.** There is no composition to settle — the surfaces are a comment
reply, a workflow log and a commit. Verification is the mechanism running on a
real pull request with a real finding, which the tasks name.

## Risks / Trade-offs

- **A patch that no longer applies** — the branch moved between review and
  dispatch → the applier fails loudly, resolves nothing, and says so in the
  pull request. Re-dispatch after a rebase is the recovery.
- **A fix that is wrong** → it arrives as an ordinary commit on the branch, the
  review re-runs on the push, and a person still merges. Nothing here bypasses
  review; it produces work for it.
- **The accept vocabulary is not what someone types** → their finding is simply
  not actioned, and the thread stays open. The failure mode is "nothing
  happened", which is visible, rather than "something happened that I did not
  mean", which is not.
- **A maintainer's dispatch acting on a finding they did not personally accept**
  — another maintainer accepted it → accepted. Write access is the boundary this
  project already trusts for merging.
- **Cost per dispatch** is a model run over the accepted findings. Bounded by
  their number, and paid only when somebody asks.
