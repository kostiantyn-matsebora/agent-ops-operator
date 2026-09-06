## Why

Every change here is worked on one workstation, in a worktree beside twelve
others, with the cluster, the build container and the owner's attention all on
that machine — so a change waits for a free window there, and an issue somebody
files waits for a person to sit down and start it. A Claude Code cloud session
already gives each piece of work its own machine, its own clone and its own
branch, which is the isolation the worktree rule was written to fake; what it
does not have is anything this repository's process needs — helm, openspec, the
envtest assets — and it reads rules that state this workstation as fact. And
the two halves of the loop that already run unattended, the review and its
fixing rounds, start only once a pull request exists: nothing yet turns a filed
issue into one.

## What Changes

- **A cloud environment for this repository, defined by the tree.** The
  environment's setup field is one line handing off to a checked-in, idempotent
  bootstrap (`.github/scripts/cloud-bootstrap.sh`) that exits at once on a
  workstation and, in the cloud, installs what the tree's own checks need —
  helm (the chart render tests SKIP silently without it), openspec at the
  version CI pins, pyyaml, the envtest assets — and verifies the Go floor. A
  verify-only run of the same script is a `SessionStart` hook, so a remote
  session is told what it lacks instead of discovering it at a silent skip.
- **The rules stop stating the workstation as fact.** `build-test.md` says a
  Go toolchain is absent and a docker container is the way to build; both are
  false in the cloud and the first is false locally now. A new unscoped rule,
  `remote-session.md`, records the environment's one line, what the bootstrap
  installs, that a remote session's clone IS its working copy (check out
  `change/<name>` in place, never add a worktree), what stays workstation-only
  (the cluster, the deploy, the visual check), and that the environment's
  variables are public — a secret goes under API credentials and is attached by
  the proxy.
- **`.mcp.json` works in both places.** The stdio servers run directly where
  their binary exists and wait-then-install in the cloud; nothing exits 0 on a
  workstation, which is what the sibling repository's wrappers do and what
  would kill the local servers here.
- **A label on an issue starts a remote session that implements it.** A second
  consent label beside `autofix` — `autoimplement`, in `review-triage.json` —
  placed on an ISSUE by a person with write access. A workflow on the label
  event checks who placed it (the collaborators API, as the fixing loop does),
  refuses visibly otherwise, fires the repository's routine with the issue
  number as payload, and records the session link on the issue once. The
  routine runs in the environment above; its saved prompt is a POINTER to a
  committed instruction file (`.github/routines/implement-issue.md`), which
  promotes the issue in place, proposes, implements on `change/<name>`, runs
  the unit and chart tiers itself and dispatches the smoke e2e workflow for
  the rest, and opens the pull request saying `Closes #<n>`.
- **The issue label is the owner's word for the fixing loop too, and the loop
  fixes EVERYTHING that holds the merge.** The pull request such a session
  opens carries `autofix` from creation. Under that label the loop's work list
  is today the review's findings and the analysis service's open issues; it
  gains every FAILED REQUIRED CHECK on the head — a red test, a lint, the docs
  generator's check, a quality gate reported as a failed job — collected by a
  program from the checks API with the failed step's log tail, and fixed or
  disputed like any other item. A red `ci-green` starts a round, beside the
  review's completion. Nothing about the loop's bound, its disputes or its one
  summary changes; what it cannot settle waits for a person, exactly as today.
  "Implement this issue" means a pull request that is green and reviewed, not
  one that compiles.
- **The routine's identity stays out of the tree.** The fire URL is a repository
  variable, the token a repository secret, the environment and routine are named
  in the rule and identified nowhere in the repository.

## Capabilities

### New Capabilities
- `remote-change-sessions`: the cloud environment a change is worked in — how
  it is defined from the repository, what a remote session finds and is told,
  what a label on an issue starts, and what consent it carries.

### Modified Capabilities
- `change-delivery`: "A change is implemented in its own working copy" gains
  the remote clone as a working copy in its own right; "A change is approved
  for automatic fixing by its owner, once" gains the issue label as the owner's
  word, so the pull request a remote session opens is labelled at creation.
- `change-issue-tracking`: "An inbound issue is promoted in place" gains the
  remote session as the promoter — the session started by a label on an issue
  promotes that issue, and the fire is recorded on it once, as a transition.
- `automated-code-review`: "The work list of an approved pull request includes
  the analysis service's issues" widens to the head's failed required checks,
  each carrying its job name and log tail; "A landed fix on an approved pull
  request starts the next round" gains a failed required check as a round's
  start, under the same bound and the same summary.

## Impact

**Code and configuration**

- `.github/scripts/cloud-bootstrap.sh` (new), `.github/scripts/remote-implement.py`
  (new), `.github/workflows/remote-implement.yml` (new),
  `.github/routines/implement-issue.md` (new), `.github/review-triage.json`
  (`implement_label`), `.github/tests/` (three new suites, `run.sh`).
- `.github/workflows/review-dispatch.yml` (a `ci` failure as a round's start;
  `collect` gathers failed checks under `actions: read`), `.github/scripts/failed-checks.py`
  (new), `.github/scripts/land-dispatch.py` (checks in the summary), the fixer's
  role file (a check item is reproduced, then fixed or disputed).
- `.claude/settings.json` (a `SessionStart` verify hook), `.mcp.json` (the
  wrappers), `.claude/rules/remote-session.md` (new).
- Outside the tree, once: the `agent-ops-operator` cloud environment, its
  Sonar credential and `SONAR_ORG` variable; the routine with its API trigger;
  the `ROUTINE_FIRE_URL` variable and `ROUTINE_FIRE_TOKEN` secret; the
  `autoimplement` label.

**Documents the change makes untrue — reference half**

- `.claude/rules/build-test.md`: "this workstation has no Go toolchain" and the
  container as the only way to build.
- `.claude/rules/worktree-delivery.md`: the consent table gains the
  `autoimplement` row and its `autofix` row says the work list includes failed
  required checks; "THE REVIEW FOUND SOMETHING" names a red `ci-green` as a
  round's start; the lifecycle gains the remote session as a working copy; the
  archive rule is unchanged.
- `.claude/rules/gotchas.md`: the routine push rules and the public-variables
  fact, measured, so nobody re-derives them.
- `openspec/config.yaml` (`rules.tasks`) and `.claude/commands/opsx/apply.md`
  step 1.5: "from its own git worktree" becomes "or a remote session's clone".
- `.claude/skills/openspec-apply-change`: where the `autofix` label is placed
  on the owner's word, the issue label now counts as that word.
- `CONTRIBUTING.md`: "How a change is proposed here" and "The issue that
  tracks it" gain the label and the remote path; "Pull requests" says the
  labelled loop fixes failed checks beside findings and analysis issues.
- `docs/CHANGELOG.md`: nothing — no chart, CRD or image changes.

**Documents the change makes untrue — adopter site**

- None. The landing page, Introduction, Getting started, Installation, the
  integration pages and the guides describe the operator an adopter installs;
  this change alters how the project is DEVELOPED and touches no shipped
  behaviour. Stated so the absence is a claim a reviewer can dispute, not an
  omission.
