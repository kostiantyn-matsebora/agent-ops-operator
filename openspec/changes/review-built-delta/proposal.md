## Why

The review spends model tokens where it should spend none, and re-derives
what it has already found: it reads a component whose code does not compile —
findings against code the author is about to rewrite anyway — and on every
push it re-reads EVERY changed file of the pull request, in a reader that is
handed its own previous findings before it reads. Two things are measured
about that reader. A component's files are read by two QUEUE READERS, each
one context that reads its rules once and then keeps every file it has read,
so the last file of a long queue is read on top of a hundred thousand tokens
of files before it. And a reading is not a function of the file: the same
file read again yields findings the first read did not, which is why a pull
request here takes several review rounds before a read adds nothing — and
the reader that is handed the threads tends to agree with them, so the rounds
are less independent than they look. The review already refuses to REPEAT a
finding; it does not yet know when a file is finished, and it pays the rules
again for every reader that starts.

## What Changes

- **A component is read only after it builds.** Each `read` job runs the
  component's own build — the recipe CI uses for it, derived from the tree —
  before any model starts. A failed build produces no reading: the component
  is reported `unbuilt` in the summary with the tail of the build's output,
  its files are recorded as NOT reviewed, and no token is spent on it. A
  directory group (`docs/`, `openspec/`, `.github/`, `.claude/`, root) has
  no build and is always read.
- **A reading is one file in one process, and it is BLIND.** The queue
  readers are gone. The `read` job runs one `claude -p` per changed file,
  several at once from the shell, each holding the file reader's role and
  the RULE TEXT routed to its path as its system prompt — the same bytes for
  every file sharing a rule set, so the API's prompt cache pays the rules
  once per job and every further reader reads them from cache — plus the
  file, its diff and the change's delta specs. It holds NO thread and no
  previous finding: every read of a file is an independent sample, whether
  the first or the fourth. A process that returns nothing usable names its
  file `unread`, as a lost reading does today.
- **Threads are judged apart from reading.** A file with unresolved threads
  gets a second, separate process handed the threads and the file, which
  returns each thread's verdict — `fixed`, `standing`, `gone`, `detached`.
  This one is meant to be primed; its whole job is the thread.
- **"Not made again" moves to the coordinator.** It already holds every
  thread and is the only writer; a blind finding matching an open or
  dismissed thread is folded into that thread's count instead of posted.
- **A file is carried once its reads have gone quiet.** The summary carries
  a hidden coverage marker — the head sha and, per path read, how many
  consecutive reads added no new finding — written by the posting program,
  never by the model. The next run reads a changed path unless it is
  unchanged since the sha it was last read at AND its quiet streak has
  reached K (a workflow variable, default 1). Everything else is CARRIED:
  its unresolved threads stand, its resolved ones count as before, and it
  is still reachable by the coordinator's cross-check. The record is
  invalidated whole by a rebase or force-push, by a change to a rule file
  or the change's delta specs, and by a hand dispatch with `-f full=true`.
- **The cross-check keeps its whole reach.** A reader's diff runs from the
  sha its file was last read at; the coordinator's reach search excludes
  only the paths read THIS run, so a carried file is a consumer it can find.
- **The summary says what was read**: a third fixed line — how many of the
  pull request's changed files were read this run, how many were carried
  quiet, how many sit in unbuilt components.

No new store. The marker is a line in a comment that is already posted, on the
pull request that is already the review's memory — the same rule the existing
"continuity is context, not stored state" requirement states. No claim that
one read finds everything: the review stops reading a file when independent
reads stop finding anything, which is the rule a person applies today, made
cheap and stated.

## Capabilities

### New Capabilities

_None._

### Modified Capabilities

- `automated-code-review`: a component that does not build is not read and is
  reported as unbuilt; a file is read blind in its own process with the rules
  as shared, cached context, and its threads are judged by a separate pass;
  duplicate suppression is the consolidation's, not the reader's; a push is
  reviewed as the delta since each file's last read, a file is carried once
  its reads have gone quiet, with coverage recorded on the pull request and
  invalidated by a rebase or by a change to what a file is judged against;
  the summary states what was read and what was carried.

## Impact

**Code:** `.github/workflows/claude-review.yml` (a build step before the
model in `read`; the per-file process loop replacing the component session;
the `full` dispatch input and the `REVIEW_QUIET_READS` variable; the coverage
file crossing from `consolidate` to the posting program);
`.github/scripts/review-input.py` (reads the coverage markers, decides per
path, carries the rest); `.github/scripts/review-prompt.py` (`reader` emits a
system prompt — role plus routed rule text — and a per-file prompt with
`since`; a new `verdict` message; `coordinator` gains carried paths, the
counts and the coverage list from the readings); `.github/scripts/review-post.py`
(computes each read path's quiet streak from what it posted and appends the
marker); a new `.github/scripts/review-build.sh` (the recipe per group, the
unbuilt reading on failure); `.github/scripts/review-reading-check.py`
(merges per-file readings and verdicts into the component's, accepts the
unbuilt shape); `.github/scripts/review-context.py` (measures one process's
system prompt and prompt, diff from `since`); `.claude/agents/file-reviewer.md`
(a blind single-file role, no threads); a new `.claude/agents/thread-verdict.md`;
`.claude/agents/review-coordinator.md` (dedup against threads, carried
paths, the third summary line, the `unbuilt` row);
`.claude/workflows/review-component.js` is DELETED — the loop is the job's.
Every new or edited script joins the restored-from-base lists in the
workflow. `.github/retired-vocabulary.json` gains `queue reader` /
`QUEUE_READERS`. Tests under `.github/tests/`.

**Reference docs made untrue:** `openspec/specs/automated-code-review/spec.md`
(the delta spec folds in at archive); `.claude/rules/worktree-delivery.md`,
"The review found something" (the four jobs are described there with two
`file-reviewer` subagents two at a time, and neither the build gate, the
per-file process nor the delta is); `.claude/rules/gotchas.md`, "A review
subagent under the action" (the pool measurement and the per-reading cost are
stated for queue readers inside one session; the unit is a process now and the
pool no longer applies); `.claude/rules/retired-vocabulary.md` (the retired
term is recorded). `docs/CHANGELOG.md` is NOT touched: nothing here ships in
the chart or an image.

**Adopter site:** nothing — the review is a contributor-facing mechanism, and
no page under `docs/` describes it. `CONTRIBUTING.md`'s "Claude reviews the
pull request" paragraph is the contributor's page and is updated: it says the
review reads every changed component and does not say it builds first, reads
blind, or reads a delta and carries what has gone quiet.
