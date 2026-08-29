## Why

The review spends model tokens twice where it should spend none: it reads a
component whose code does not compile — findings against code the author is
about to rewrite anyway, and a reader spending turns on a build error a
compiler already stated — and on every push it re-reads EVERY changed file of
the pull request, including the ones no commit has touched since they were
last reviewed. A ten-file change pushed five times is read fifty times to find
what five readings would have found. The review already refuses to REPEAT a
finding; it does not yet refuse to re-derive one.

## What Changes

- **A component is read only after it builds.** Each `read` job runs the
  component's own build — the recipe CI uses for it, derived from the tree —
  before any model starts. A failed build produces no reading: the component
  is reported `unbuilt` in the summary with the tail of the build's output,
  its files are recorded as NOT reviewed, and no token is spent on it. A
  directory group (`docs/`, `openspec/`, `.github/`, `.claude/`, root) has
  no build and is always read.
- **A push is reviewed as its DELTA since the last review.** The summary the
  review posts now carries a hidden coverage marker — the head sha and the
  paths that were actually read at it, written by the posting program, never
  by the model. The next run reads only the changed paths that are new,
  changed since the sha they were last read at, or whose coverage no longer
  holds (the sha is not an ancestor of the head — a rebase or force-push — or
  what a file is judged against changed: a rule file or the change's delta
  specs). Everything else is CARRIED: its unresolved threads stand, its
  resolved ones count as before, and it is still reachable by the
  coordinator's cross-check.
- **The cross-check keeps its whole reach.** A reader's diff runs from the
  sha its file was last read at; the coordinator's reach search excludes
  only the paths read THIS run, so a carried file is a consumer it can find,
  and every thread on a read file — the previous reviews — is still handed
  to its reader for a verdict.
- **The summary says what was read**: a third fixed line — how many of the
  pull request's changed files were read this run, how many were carried
  unchanged since their last review, how many sit in unbuilt components.
- **A hand dispatch may ask for a full review** (`-f full=true`), for when a
  rebase or a rule change should be re-read as one.

No new store. The marker is a line in a comment that is already posted, on the
pull request that is already the review's memory — the same rule the existing
"continuity is context, not stored state" requirement states.

## Capabilities

### New Capabilities

_None._

### Modified Capabilities

- `automated-code-review`: a component that does not build is not read and is
  reported as unbuilt; a push is reviewed as the delta since each file's last
  review, with coverage recorded on the pull request itself and invalidated
  by a rebase or by a change to what a file is judged against; the summary
  states what was read and what was carried.

## Impact

**Code:** `.github/workflows/claude-review.yml` (a build step before the
model in `read`, the `full` dispatch input, the coverage file crossing from
`consolidate` to the posting program); `.github/scripts/review-input.py`
(reads the coverage markers, decides per path, carries the rest);
`.github/scripts/review-prompt.py` (per-file `since`, the carried paths in
the coordinator's message, the coverage list from the readings);
`.github/scripts/review-post.py` (appends the marker); a new
`.github/scripts/review-build.sh` (the recipe per group, and the unbuilt
reading on failure); `.github/scripts/review-reading-check.py` (accepts the
unbuilt shape); `.github/scripts/review-context.py` (the diff it measures
starts at `since`); `.claude/workflows/review-component.js` and
`.claude/agents/file-reviewer.md` (diff from `since`);
`.claude/agents/review-coordinator.md` (carried paths, the third summary
line, the `unbuilt` row). Every new or edited script joins the
restored-from-base lists in the workflow. Tests under `.github/tests/`.

**Reference docs made untrue:** `openspec/specs/automated-code-review/spec.md`
(the delta spec folds in at archive); `.claude/rules/worktree-delivery.md`,
"The review found something" (the four jobs are described there, and neither
the build gate nor the delta is); `.claude/rules/gotchas.md`, "A review
subagent under the action" (measurements of cost per reading are stated as
if every file is read every run). `docs/CHANGELOG.md` is NOT touched: nothing
here ships in the chart or an image.

**Adopter site:** nothing — the review is a contributor-facing mechanism, and
no page under `docs/` describes it. `CONTRIBUTING.md`'s "Claude reviews the
pull request" paragraph is the contributor's page and is updated: it says the
review reads every changed component and does not say it builds first or
reads a delta.
