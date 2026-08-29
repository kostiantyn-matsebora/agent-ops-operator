## Context

- `claude-review.yml` posts findings as threads; `review-dispatch.yml` fixes
  what a person accepted per thread (`fix it`) on one `/fix-accepted` comment:
  `gate` → `collect` → `fix` (model, `contents: read`, emits a patch) → `land`
  (no model, `contents: write`, pushes, replies, resolves).
- A push with `GITHUB_TOKEN` fires no `push` / `pull_request: synchronize`
  event. `workflow_dispatch` and `repository_dispatch` ARE fired by that token
  — the documented exception. `claude-review.yml` already accepts
  `workflow_dispatch` with `number`; `ci.yml` runs on `pull_request` and
  `push: master` only.
- Branch protection requires `ci-green` on the head sha and every thread
  resolved. A check produced by a `workflow_dispatch` run is attached to the
  sha the run checked out, but whether protection counts it for the pull
  request is NOT known — it is the spike.
- `sonarcloud-analysis` (#125, PR #127, unmerged) submits one project per
  component, `sonar (<component>)` in `ci-green`'s `needs:` for submission
  only; `SONAR_TOKEN` is a repository secret; the quality-gate check is
  Sonar's own and not required. Its issues for a pull request are
  `GET /api/issues/search?componentKeys=<project>&pullRequest=<n>&resolved=false`.
- Review verdicts vary between runs (`gotchas.md`); the reviewer will review
  the fixer's commits.
- Consent today is deliberately mechanical: a list in `review-triage.json`
  matched by `accepted-findings.py`, never a classifier.

## Goals / Non-Goals

**Goals:**

- One approval per change; no per-thread typing and no hand push while the
  loop runs.
- Both reviewers' findings in one work list, one fix commit per round.
- A disagreement ends in a person's inbox, never in a silently skipped item
  and never in a change to Sonar's state.
- Bounded rounds, one summary per ending.
- The model still holds no credential and cannot push.

**Non-Goals:**

- Changing what the review finds or how it reads (`claude-review.yml`'s
  `queue`/`read`/`consolidate` are untouched).
- Fixing findings on a fork's pull request.
- Marking anything in SonarCloud (`won't fix`, `false positive`) — a person's
  verdict, done in Sonar's UI.
- A merge that happens by itself. The loop ends at "nothing left"; a person
  merges.

## Decisions

### D1 — Consent is a label, checked at every trigger

`autofix` on the pull request. `review-triage.json` gains `"approve_label":
"autofix"` beside `accept` / `dispatch`, so the vocabulary stays one file.

- **Who may set it** is the same gate as a dispatch: the `labeled` event's
  `sender` must have write access (`author_association` is not on a label
  event; `gate` asks `GET /repos/{r}/collaborators/{u}/permission`). A label
  placed by anyone else is REMOVED by `gate` with a visible comment — a
  refusal that leaves the label in place would look like a loop that broke.
- **Fork pull requests are refused** exactly as a dispatch is.
- **The session places it** with `gh pr edit <n> --add-label autofix` on the
  owner's word — the apply skill gains that step, phrased as a request the
  owner answers, never a default.
- *Alternative rejected:* a `/fix-all` comment. A comment is an event, gone
  once handled; the label is state, readable on every later round by the
  expression that decides whether to loop.

### D2 — Three triggers, one workflow, one `gate`

`review-dispatch.yml` adds:

| Trigger | Means |
|---|---|
| `pull_request: [labeled]` with `autofix` | round 1 over what is already open |
| `workflow_run: [claude-review] completed` | the review finished on a labelled pull request → next round |
| existing comment triggers | per-thread mode, unchanged |

`gate` outputs `mode: threads | all`. `workflow_run` carries the head sha and
pull request number of the reviewed run; `gate` reads the label from the pull
request at that moment, so removing it stops the loop at the next boundary
(spec: the running round completes).

- *Why `workflow_run` and not a step at the end of the review:* the review's
  jobs hold no write and must stay that way; a workflow that reacts to its
  completion keeps the review's privilege set untouched.
- The existing `concurrency` group per pull request serialises rounds.

### D3 — `collect` in `all` mode reads two sources, and the model reads none

`accepted-findings.py --mode all` lists every unresolved thread the review
authored that carries no dispute marker (D5). A new `sonar-issues.py` lists
open issues per component project for the pull request, keyed
`sonar:<issueKey>`, carrying rule, message, file, line. Both are written to
the same work-list JSON `fix` already consumes; each item carries `source:
review | sonar`.

- **Sonar not yet analysed for the head sha** (`analysisDate` older than the
  commit, or no analysis): collect proceeds without it and the round's summary
  says so. Sonar's own analysis of the landed commit runs in `ci.yml`, which
  the re-trigger (D4) starts, so the NEXT round sees it.
- The project key derivation is the one `sonar-scan/action.yml` uses
  (`<org>_agent-ops-operator_<component>`); the script imports nothing from
  the action but the same `components.sh` list.

### D4 — The re-trigger: spike first, App second

**Task 1 is a spike with a written outcome**, on a throwaway pull request:

1. `land` pushes with `GITHUB_TOKEN`, then `gh workflow run ci.yml -f pr=<n>`
   and `gh workflow run claude-review.yml -f number=<n>` (`ci.yml` gains a
   `workflow_dispatch` input and checks out `refs/pull/<n>/head`).
2. Observe whether the merge box shows `ci-green` satisfied on that head.

| Outcome | Then |
|---|---|
| protection counts the dispatched `ci-green` | ship the self-dispatch; NO App, no new secret |
| it does not | `land` pushes with a GitHub App installation token minted in the job (`actions/create-github-app-token`), so the push is an ordinary push and every existing trigger fires; `ci.yml` is untouched |

The App, if created: repository-scoped to this repository, permissions
`contents: write`, `pull-requests: write`, nothing else; app id and private
key as repository secrets; read by `land` ONLY. `fix` keeps `contents: read`
and no secret — "what writes the fix holds no power to push it" is unchanged.

- *Alternative rejected:* a personal access token. It is a person's identity
  and expires; an App's is the repository's and is scoped.
- Under either outcome the comment `land` posts on an UNLABELLED dispatch
  ("push again to get CI") stays, because that path still pushes with
  `GITHUB_TOKEN`.

### D5 — Fix or dispute, and how a dispute is remembered

`fix`'s role prompt gains the contract: for each item output `{id, action:
fixed | disputed, reason}` in the result JSON beside the patch. `land`:

- `fixed` → as today: `Fixed in <sha>`, resolve (only where the patch landed).
- `disputed`, review item → reply in the thread with a fixed marker line
  `<!-- autofix:disputed -->` followed by the reason; NOT resolved.
- `disputed`, sonar item → one comment on the pull request listing every
  disputed issue key with its reason, under the same marker.

The marker is what `collect` reads to skip a thread on later rounds (spec:
never disputed twice) and what the archive guard (D8) reads: a disputed thread
is "answered" when a comment by a human user follows the marker.

- *Why not resolve a disputed thread:* resolution is the merge gate; a
  disagreement the model closed would merge over a person's absence.
- *Why never touch Sonar's state:* the token that lists issues can also mark
  them; the script exposes no such call, and the model holds no token.

### D6 — The bound and the ending

`land` counts rounds by the `<!-- autofix:round N -->` marker on its own
landing comments — state on the pull request, derivable, no store. `MAX_ROUNDS:
3` is a workflow `env`. A round ends the loop when any of:

| Condition | Summary says |
|---|---|
| collect found nothing | clean |
| every item disputed, nothing fixed | disputes only, `@approver` |
| patch stale / applied nothing | stale, rebase and re-label |
| `N == MAX_ROUNDS` after landing | cap reached, `@approver`, what remains |

Otherwise the round re-triggers (D4) and the summary is deferred to the
ending. The approver is the user who placed the label (from the `labeled`
event, recorded in the round-1 landing comment marker so later rounds know
whom to mention).

### D7 — Sonar's gate becomes required

`ci-green`'s `needs:` gains the gate — either `sonar (<component>)` waiting
on `api/qualitygates/project_status?pullRequest=` after submission (the
spike's self-dispatch outcome, where Sonar's own check may not attach), or
Sonar's own check named in branch protection (the App outcome, where a
normal push makes it attach). The spec requires the verdict through the
always-present check; the design leaves which mechanism to the spike's
result, and the task records it.

- **Ordering:** this change's `code-quality-analysis` delta modifies a
  requirement #127 publishes. #127 archives first; if it has not by apply
  time, this change's branch rebases on it.

### D8 — The archive refusal

`.claude/hooks/require-docs-task.sh` already refuses `openspec archive`
through `docs-task-guard.py`; it gains a second check, `autofix-guard.py`,
fail-open like the first: no `gh`, no pull request, no label → allow. With
the label: refuse while a `review-dispatch` run is in progress for the pull
request, or while a thread carries the dispute marker with no later human
comment. CI's `docs-task` job calls the same script so the archive commit in
the pull request is judged where the hook can be skipped.

## Risks / Trade-offs

- **Oscillation** — the reviewer objects to the fixer's fix. Bounded by
  `MAX_ROUNDS`; the summary makes a three-round argument visible rather than
  silent. If it recurs, the answer is a reviewer prompt change, not a higher
  cap.
- **A credential that can push exists** (App outcome). Mitigated: held by the
  model-free job only, repository-scoped, two permissions, and the model's
  job is asserted to carry no secret by the workflow's own CI test.
- **Sonar lag** — round N fixes Sonar's issues, Sonar's re-analysis lands
  during round N+1's collect or after it. The "not consulted" summary line
  and the gate being required together mean the pull request is never
  reported clean while Sonar disagrees.
- **Label from a non-writer** is removed with a comment; a writer can put it
  back. A refusal that stayed would look like a broken loop.
- **`workflow_run` runs the DEFAULT branch's copy** of `review-dispatch.yml`,
  so a pull request cannot rewrite the loop that fixes it — the same property
  the review has, and the same cost: this change's own workflow edits are
  testable only by `workflow_dispatch` until merged.
- **Cost** — up to three review runs and three fix runs per pull request,
  automatically. `MAX_ROUNDS` is the knob, and a label is the opt-in.
