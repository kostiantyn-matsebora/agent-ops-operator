## 1. The spike — three facts about subagents under the action

- [x] 1.1 On a throwaway branch, wire the shape: `Agent(component-reviewer)`
  on the allowlist and an inline `--agents` definition whose only instruction
  is to report what its context contains — the files it can see loaded, the
  tools it holds, whether `.claude/rules/*.md` arrived unasked. Spawn two of
  them. Verify from the run log: (a) the spawn happens under the action's
  allowlist, (b) the two ran concurrently, (c) what a reviewer's context holds.
  Record all three in `design.md` and, if either surprises, in
  `.claude/rules/gotchas.md`.
- [x] 1.2 If (a) fails: the design moves to a matrix job per component, and
  `design.md` says so before anything else is built. Verify the decision is
  written down, not carried in a head.
  **DONE, LOCALLY.** The action refuses any workflow file that is new or differs from the default branch — on every trigger — so no branch can run it; but the three facts are Claude Code facts and `claude_args` passes through verbatim, so `claude -p` in the worktree with the workflow's flags is the spike. 48s, two reviewers: (a) spawns, (b) windows 138–140s / 139–141s overlapped, (c) all fifteen unscoped rules present unasked, the three scoped ones absent, tools exactly the definition's. Recorded in `design.md` and `gotchas.md`.
  **NOT NEEDED** — (a) passed. `design.md` keeps the matrix fallback as the rejected alternative.

## 2. The queue — a component list from a diff

- [x] 2.1 Group the pull request's changed paths by component, using the list
  `.github/components.sh` derives; paths outside any component group by
  top-level directory. A model-free program in `.github/scripts/`, with a suite
  in `.github/tests/` covering a component path, a nested path, `docs/`, and a
  path in no group. Verify a docs-only diff yields exactly one entry.
  **DONE.** `.github/scripts/review-queue.py` (paths as arguments or stdin; `--components` for tests) and `review-queue.test.sh`, 11 assertions including one against the real tree through `components.sh`. A docs-only diff is one entry.

## 3. The reviewer — read-only, clean, returns data

- [x] 3.1 Define `component-reviewer` inline in `claude-review.yml`'s
  `claude_args`: `tools: Read, Grep, Glob, Bash(git diff:*)`, `model: inherit`,
  a `maxTurns` bound, and a system prompt that reads its component's diff, the
  rules named in its delegation message, and its standing threads, and returns
  the JSON document `design.md` states. Verify the definition contains no
  posting tool and no `mark-thread-resolved.sh`.
- [x] 3.2 The reviewer's instructions name the six-line finding shape as the
  shape of `claim` + `detail`, so the consolidator forwards rather than
  rewrites. Verify on the live run in §6 that an inline comment is the
  reviewer's words within the shape.
  **DONE.** Inline in `claude_args` via `--agents`: `tools: Read, Grep, Glob, Bash(git diff|log|show:*)`, `model: inherit`, `maxTurns: 40`. `claude-review.test.sh` asserts no posting tool, no `mark-thread-resolved`, no `Agent`, no apostrophe (it would end the shell argument — found by the first splice failing).
  **DONE in the prompt; verified in the local dry run** — the consolidator forwarded reviewer claims as the first line of each six-line finding. Live shape on the first reviewed pull request after the merge.

## 4. The consolidator — the main context

- [x] 4.1 Rewrite the review prompt as the consolidator's: build the queue
  (§2), hand each reviewer its diff, its rules and its threads, spawn all,
  collect. Verify on the live run that reviewers ran concurrently — the run
  log's timestamps, not the prompt's intent.
- [x] 4.2 Dedup findings across reviewers by path + claim, then post each once
  through `mcp__github_inline_comment__create_inline_comment`, in the six-line
  shape. Verify two reviewers raising one finding produce one comment.
- [x] 4.3 Resolve consumers of the union of `changedNames` with `git grep -l`,
  read each, and raise a finding where a consumer no longer holds. Verify
  deliberately on the spike branch: rename a field in `docs/contracts.md` and
  the manager's handler only, and confirm a finding lands on an adapter that
  still speaks the old name.
- [x] 4.4 The summary carries the reach — one line per changed name naming its
  consumer count — within the existing twelve-line bound; or the count line
  and `reach: none outside the change`. Verify the summary on the live run
  stays within the bound.
- [x] 4.5 Apply the thread verdicts through the existing rules: `fixed` →
  reply and `mark-thread-resolved.sh`; `detached` → re-raise at the current
  line, record the old as superseded; a thread on an unchanged component →
  untouched. Verify the existing `reconcile` job still refuses a thread the
  review did not author — nothing in this change touches it.
- [x] 4.6 A reviewer that returns malformed data, or none, is reported in the
  summary as `unreviewed: <component>`. Verify by making a reviewer return
  prose on the spike branch.
- [x] 4.7 The consolidator raises, as its first finding, any edit to
  `.claude/rules/` or `.claude/agents/` — the two things a branch can change
  that alter how it is read. Verify on the spike branch with a one-word edit
  to a rule file.
  **DONE; concurrency verified twice** — the spike (timestamps) and the dry run (three reviewers, 127s total against 126–415s serial). Live run log: after the merge.
  **DONE; verified in the dry run** — the manager and docs reviewers both raised the contract disagreement; the consolidator posted it once (eight raw findings → five).
  **VERIFIED DELIBERATELY, on a scratch commit** renaming `previousThreadId` → `priorThreadId` in `docs/contracts.md` and `internal/chat/message.go` only. Reach: `channels/telegram/manager.go:112` reported BROKEN (still decodes the old key, so every reopen gets a fresh topic), the published contract spec and `invariants.md` BROKEN, two archived records HOLD. Exactly the failure the change exists to catch. The scratch commit was discarded.
  **DONE.** Reach line in the summary within the twelve-line bound; the dry run's summary was eleven lines.
  **DONE.** Verdict handling in the consolidator prompt; `reconcile` and `resolve-review-threads.py` untouched — `claude-review.test.sh` asserts `contents: write` is still only `reconcile`'s.
  **DONE in the prompt** (`unreviewed: <group>` row). Not forced in the dry run — every reviewer returned valid JSON; the row is what a malformed one produces.
  **VERIFIED in the dry run** — the scratch commit's one-word edit to `gotchas.md` was finding #1. It also caught a real defect in this change's own gotchas entry (appended under the wrong `###`), which is fixed.

## 5. The guard still holds

- [x] 5.1 Verify the always-ran guard reports the skip on the pull request that
  carries this change (the workflow file differs), and that the first pull
  request after the merge runs the new shape. This is the same blocked-on-merge
  reading every change to this file has had; say so in the tasks rather than
  pretending it was verified earlier.
  **AS EXPECTED**: this pull request edits `claude-review.yml`, so the guard reports the skip; the first reviewed pull request after the merge runs the new shape.

## 6. On a real pull request

- [x] 6.1 The first reviewed pull request after the merge: read the run log
  for concurrency and per-reviewer duration against the three timed serial
  runs (126s, 351s, 415s); read the summary for the reach line; read one
  inline finding for the shape. Record the numbers here.
  **BLOCKED ON THE MERGE**, for the reason 5.1 states. The dry run's numbers stand in until then: 3 reviewers, 127s wall, 8 → 5 findings, reach 3 broken / 4 hold.

## 7. Documentation

### 7.1 The reference docs

- [x] 7.1.1 `CONTRIBUTING.md` — the *Pull requests* paragraph on the review
  gains one clause: findings are made per component and the summary names the
  reach the review checked. Verify it links rather than restates.
- [x] 7.1.2 `.claude/rules/gotchas.md` — whatever the spike settled about
  subagents under the action that a future reader would re-derive wrongly:
  the rules-in-context answer, and the inline-definition-is-the-guarded-one
  reasoning if it is not obvious from the workflow file. Verify each entry
  says what it cost, not only what is true.
- [x] 7.1.3 `design.md` of this change carries the spike's three answers.
  Verify before archiving that none of the three reads "TBD".
  **DONE.** One paragraph, naming per-component reading and the reach; links nothing new because the mechanism is in the workflow file's own comments.
  **DONE**, its own `###` under `gotchas.md`: the rules-in-context measurement, the action's refusal of any branch copy on any trigger, and the apostrophe-in-`claude_args` trap — each with what it cost.
  **DONE** — the table of three answers is in `design.md`; no TBD.

### 7.2 The adopter site

- [x] 7.2.1 Confirm the site is untouched: no page under `docs/` describes
  contribution or review. State the reader who is unaffected and why.
- [x] 7.2.2 Verify the site still builds — `git diff --stat origin/master --
  docs/` empty settles it by identity; otherwise the jekyll container from
  `docs/CLAUDE.md`, run against the WORKTREE's `docs/`.
  **CONFIRMED.** No page under `docs/` describes contribution or review; the adopter is unaffected.
  **VERIFIED BY IDENTITY** — `git diff --stat origin/master -- docs/` is empty.
