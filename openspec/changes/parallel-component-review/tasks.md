## 1. The spike — three facts about subagents under the action

- [ ] 1.1 On a throwaway branch, wire the shape: `Agent(component-reviewer)`
  on the allowlist and an inline `--agents` definition whose only instruction
  is to report what its context contains — the files it can see loaded, the
  tools it holds, whether `.claude/rules/*.md` arrived unasked. Spawn two of
  them. Verify from the run log: (a) the spawn happens under the action's
  allowlist, (b) the two ran concurrently, (c) what a reviewer's context holds.
  Record all three in `design.md` and, if either surprises, in
  `.claude/rules/gotchas.md`.
- [ ] 1.2 If (a) fails: the design moves to a matrix job per component, and
  `design.md` says so before anything else is built. Verify the decision is
  written down, not carried in a head.

## 2. The queue — a component list from a diff

- [ ] 2.1 Group the pull request's changed paths by component, using the list
  `.github/components.sh` derives; paths outside any component group by
  top-level directory. A model-free program in `.github/scripts/`, with a suite
  in `.github/tests/` covering a component path, a nested path, `docs/`, and a
  path in no group. Verify a docs-only diff yields exactly one entry.

## 3. The reviewer — read-only, clean, returns data

- [ ] 3.1 Define `component-reviewer` inline in `claude-review.yml`'s
  `claude_args`: `tools: Read, Grep, Glob, Bash(git diff:*)`, `model: inherit`,
  a `maxTurns` bound, and a system prompt that reads its component's diff, the
  rules named in its delegation message, and its standing threads, and returns
  the JSON document `design.md` states. Verify the definition contains no
  posting tool and no `mark-thread-resolved.sh`.
- [ ] 3.2 The reviewer's instructions name the six-line finding shape as the
  shape of `claim` + `detail`, so the consolidator forwards rather than
  rewrites. Verify on the live run in §6 that an inline comment is the
  reviewer's words within the shape.

## 4. The consolidator — the main context

- [ ] 4.1 Rewrite the review prompt as the consolidator's: build the queue
  (§2), hand each reviewer its diff, its rules and its threads, spawn all,
  collect. Verify on the live run that reviewers ran concurrently — the run
  log's timestamps, not the prompt's intent.
- [ ] 4.2 Dedup findings across reviewers by path + claim, then post each once
  through `mcp__github_inline_comment__create_inline_comment`, in the six-line
  shape. Verify two reviewers raising one finding produce one comment.
- [ ] 4.3 Resolve consumers of the union of `changedNames` with `git grep -l`,
  read each, and raise a finding where a consumer no longer holds. Verify
  deliberately on the spike branch: rename a field in `docs/contracts.md` and
  the manager's handler only, and confirm a finding lands on an adapter that
  still speaks the old name.
- [ ] 4.4 The summary carries the reach — one line per changed name naming its
  consumer count — within the existing twelve-line bound; or the count line
  and `reach: none outside the change`. Verify the summary on the live run
  stays within the bound.
- [ ] 4.5 Apply the thread verdicts through the existing rules: `fixed` →
  reply and `mark-thread-resolved.sh`; `detached` → re-raise at the current
  line, record the old as superseded; a thread on an unchanged component →
  untouched. Verify the existing `reconcile` job still refuses a thread the
  review did not author — nothing in this change touches it.
- [ ] 4.6 A reviewer that returns malformed data, or none, is reported in the
  summary as `unreviewed: <component>`. Verify by making a reviewer return
  prose on the spike branch.
- [ ] 4.7 The consolidator raises, as its first finding, any edit to
  `.claude/rules/` or `.claude/agents/` — the two things a branch can change
  that alter how it is read. Verify on the spike branch with a one-word edit
  to a rule file.

## 5. The guard still holds

- [ ] 5.1 Verify the always-ran guard reports the skip on the pull request that
  carries this change (the workflow file differs), and that the first pull
  request after the merge runs the new shape. This is the same blocked-on-merge
  reading every change to this file has had; say so in the tasks rather than
  pretending it was verified earlier.

## 6. On a real pull request

- [ ] 6.1 The first reviewed pull request after the merge: read the run log
  for concurrency and per-reviewer duration against the three timed serial
  runs (126s, 351s, 415s); read the summary for the reach line; read one
  inline finding for the shape. Record the numbers here.

## 7. Documentation

### 7.1 The reference docs

- [ ] 7.1.1 `CONTRIBUTING.md` — the *Pull requests* paragraph on the review
  gains one clause: findings are made per component and the summary names the
  reach the review checked. Verify it links rather than restates.
- [ ] 7.1.2 `.claude/rules/gotchas.md` — whatever the spike settled about
  subagents under the action that a future reader would re-derive wrongly:
  the rules-in-context answer, and the inline-definition-is-the-guarded-one
  reasoning if it is not obvious from the workflow file. Verify each entry
  says what it cost, not only what is true.
- [ ] 7.1.3 `design.md` of this change carries the spike's three answers.
  Verify before archiving that none of the three reads "TBD".

### 7.2 The adopter site

- [ ] 7.2.1 Confirm the site is untouched: no page under `docs/` describes
  contribution or review. State the reader who is unaffected and why.
- [ ] 7.2.2 Verify the site still builds — `git diff --stat origin/master --
  docs/` empty settles it by identity; otherwise the jekyll container from
  `docs/CLAUDE.md`, run against the WORKTREE's `docs/`.
