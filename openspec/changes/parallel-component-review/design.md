## Context

`claude-review.yml` runs one `anthropics/claude-code-action@v1` step whose
prompt reads the rules, the whole diff and every standing thread in one
context. Measured: the model step is 97–99% of a run, 126–415s on the three
runs that were timed. The action is Claude Code in `-p` mode, so its subagent
mechanics apply: clean context, own system prompt, tools allowlist, up to 20
concurrent, `Agent` stripped from subagents by default, only the final summary
returned to the parent. A definition can be passed inline with `--agents`, for
that session only.

The fixed constraint from `automated-code-review`: the review keeps no record
of its own state — findings live on the pull request and are handed to the next
run as context. The fixed constraint from `structure.md`: modules are
self-contained and import nothing from each other, so a contract change is
invisible to every compiler and test suite.

## Goals / Non-Goals

**Goals:**

- The model step's wall clock is bounded by the largest changed component.
- A reviewer reads the rules for its path and no others.
- A change to a contract is checked against the consumers of what it changed.
- One place posts, dedups and reconciles threads.
- A pull request cannot rewrite its own reviewer.

**Non-Goals:**

- Incremental review (a last-reviewed marker). Adds state the spec forbids;
  deferred until the parallel shape has been measured.
- Reviewing at `ready_for_review` only. Changes when the review runs, not how;
  a separate decision.
- A matrix job per component. Same picture one level up, at ~40s of action
  setup per job. The fallback if the spike says subagents do not spawn there.

## Decisions

**THE UNIT IS A COMPONENT, NOT A FILE.** `components.sh` already derives the
list; a component is what shares rules and context, and two files in one
adapter are one change. Files outside any component (`docs/`, `.github/`,
`openspec/`, `.claude/`) group by top-level directory.

*Alternative rejected:* one reviewer per file. More parallel, but two halves of
one edit reviewed apart lose the fact that they are one edit, and the
consolidator would be re-deriving that.

**THE REVIEWER IS DEFINED INLINE IN THE WORKFLOW FILE.** `--agents` in
`claude_args`, so the definition sits inside `claude-review.yml` and inherits
its guard: the action refuses to run a copy that differs from the default
branch's.

*Alternative rejected:* `.claude/agents/component-reviewer.md`, the documented
team practice. Here it is the wrong side of the guard — a pull request could
weaken the reviewer that reads it. The same exposure already exists for
`.claude/rules/` and is named in the consolidator's prompt as a finding to
raise, not fixed here.

**REVIEWERS ARE READ-ONLY AND RETURN STRUCTURED DATA.** `tools: Read, Grep,
Glob, Bash(git diff:*)`. No posting tool, no `mark-thread-resolved.sh`. The
return is one JSON document:

```
{ "component": "signals/cron",
  "findings":     [{ "path", "line", "claim", "detail", "rule" }],
  "changedNames": ["/signal/inbound", "LISTEN_ADDR", "spec.port"],
  "threads":      [{ "id", "verdict": "fixed|standing|gone|detached" }] }
```

The consolidator writes every comment, in the six-line and table shapes the
prompt already states. A reviewer that wrote prose for a human would be a
second author of the wall of text.

**CONSUMERS ARE RESOLVED MECHANICALLY.** The consolidator runs `git grep -l`
over the union of `changedNames` and hands itself the file list. A program
decides what is read; the model decides what it means — the same split as
`accepted-findings.py`. The list is posted in the summary as the reach the
review checked, so a reader sees the blast radius rather than trusting it.

*Alternative rejected:* the consolidator guessing which components care. That
is the failure the change exists to fix.

**STANDING THREADS ARE HANDED TO THEIR COMPONENT'S REVIEWER.** Each reviewer
gets the open threads on its paths and returns a verdict per thread; a thread
on a component with no diff is carried over unread. The consolidator applies
the existing rules — reply and record `fixed`, re-raise `detached` at its
current line, never touch a person's thread.

**THE MODEL IS A KNOB, NOT A DECISION MADE HERE.** The definition carries
`model: inherit` initially. A reviewer's job is bounded enough that a smaller
model may serve; that is measured after the shape works, not guessed before.

**NO PROTOTYPE, ONE SPIKE — AND IT RAN LOCALLY, NOT ON A BRANCH.** The action
refuses to run any workflow file that is new or differs from the default
branch's, so a throwaway branch cannot exercise it at all. The three facts are
Claude Code facts, and the action passes `claude_args` through verbatim, so
`claude -p` in the worktree with exactly the workflow's flags answers them.
Run on 2026-08-26, 48s wall, two reviewers:

| Fact | Answer |
|---|---|
| (a) `Agent(component-reviewer)` on the allowlist spawns an inline `--agents` definition | YES |
| (b) two reviewers spawned in one message run concurrently | YES — windows 138–140s and 139–141s overlapped |
| (c) what a reviewer's context holds before it reads anything | `CLAUDE.md` and EVERY UNSCOPED rule file (15); the three `paths:`-scoped ones (`chart.md`, `signal-rules.md`, `palette-and-mark.md`) load only on reading a matching file; the git status snapshot; the tools named in its definition and nothing else |

**Consequence of (c): rule routing is free for scoped rules and impossible for
the rest.** A reviewer for `chart/` gets `chart.md` the moment it reads a
template, and no other reviewer pays for it. The 15 unscoped files are in
every reviewer's context regardless — the fixed cost is paid N times, in
parallel, and the delegation message need not name any of them. The lever on
that cost is scoping more rules with `paths:`, which is a decision about the
rules and belongs to a change of its own.

## Risks / Trade-offs

- **Subagents do not spawn under the action** → the matrix-job fallback, same
  design one level up, accepted setup cost. The spike decides in one run.
- **`.claude/rules/` is auto-loaded into every subagent** → the fixed cost
  multiplies by N instead of routing; wall clock still drops. Mitigation: the
  delegation message names the rules that apply and the reviewer is told to
  read those; if the rules are in context regardless, routing is a future
  `paths:` question for the rules themselves.
- **Tokens go up.** N contexts read N sets of rules. Accepted: the goal is
  time, and the reviewer count is the changed-component count, so a docs-only
  push spawns one.
- **A finding that only exists across two components** — an edit in A that is
  wrong because of an edit in B, neither wrong alone → the consolidator's
  reading, on the changed names. What it cannot see is a cross-component
  interaction with no shared name; that class was missed by the serial reading
  too.
- **A reviewer returns malformed data** → the consolidator reports the
  component as unreviewed in the summary rather than silently dropping it. A
  gap that is visible, the same rule as the dismissed count.
