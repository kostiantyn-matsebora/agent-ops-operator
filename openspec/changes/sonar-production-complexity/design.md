## Context

See proposal.md - Why. The technique is already proven and recorded — see
`.claude/rules/gotchas.md`'s "WRAPPING A TEST FUNCTION'S BODY IN `t.Run` DOES
NOT LOWER SONAR'S `go:S3776`..." entry, and `sonar-ratings-baseline`'s task
2.1 for the 35 test-file findings it already fixed the same way. What's
different here is stakes: these 26 findings sit in production reconciler,
dispatch and httpapi code that runs on every cluster event, not test bodies,
so a wrong extraction changes real behavior rather than failing a test in
the same commit.

## Goals / Non-Goals

**Goals:**
- Every one of the 26 findings addressed: fixed (extracted, verified,
  reads clear on the next analysis) or explicitly left with a stated reason
  — no silent drop, matching this repo's own task-writing convention.
- Zero behavior change. A reviewer reading the diff should see the same
  logic, relocated and named, never reordered or altered conditionals.
- The branch-wide `go:S3776` backlog for `manager` re-verified against a
  live analysis after the fixes land (task 2.2's carried-forward scope).

**Non-Goals:**
- Not a broader refactor of these files — no renaming unrelated symbols, no
  restructuring beyond what each finding's own extraction requires.
- Not the org-wide enumeration beyond `manager` (`sonar-ratings-baseline`
  task 1.2) — explicitly out of scope, see proposal.md.
- Not a chance to also fix unrelated findings (duplicate literals, etc.) in
  these files if any remain — this change is scoped to `go:S3776` only,
  matching its own name.

## Decisions

**One task per FILE, not one per finding.** `conversation_controller.go`
alone carries 7 findings; extracting them one at a time across separate
commits would touch the same function bodies repeatedly and cost more review
overhead than it buys. A file's findings are read and fixed together, with
each extraction still individually named in that task's own record (matching
`sonar-ratings-baseline` task 2.1's per-finding narration style) so the task
list stays auditable finding-by-finding even though the diff lands as one
commit per file.

**Extraction over restructuring.** The working technique, already proven: a
flagged function's control-flow-heavy body moves into a new, named top-level
function, called by the original — never a closure (does not lower the
score) and never a broader reshape of the surrounding logic (raises the risk
this proposal explicitly declines). Where a function is flagged because it
does several genuinely distinct things (parse, validate, act), the split
follows that seam rather than an arbitrary line-count cut, so the extracted
name documents what it does.

**Order by risk, cheapest first.** `activity/log.go`, `mcpcompile/compile.go`
and `dispatch/dispatch.go` (1 finding each, narrower blast radius) go before
`conversation_controller.go`'s 7 findings including the complexity-61 one —
so if this change has to stop partway (the same "the hour" constraint that
deferred it the first time), what's already fixed is the safer, easier-to-
verify set, and what's left is named rather than half-attempted.

**No behavior test added just to hit a coverage number.** A new unit test is
added only where an extracted function's current behavior isn't already
exercised by the existing suite — added FOR the extraction (so the refactor
itself is verified), not retrofitted to satisfy a coverage gate this change
doesn't carry.

## Risks / Trade-offs

- **A wrong extraction silently changes reconciler behavior** →
  mitigated by requiring `go build`/`go vet` under the default, `conformance`
  and `e2e` tags plus the full `go test ./...` (envtest included) after
  EVERY file's extraction, not once at the end — the same verification
  `sonar-ratings-baseline` task 2.1 already ran per round.
- **`conversation_controller.go`'s complexity-61 function may not reduce to
  a single clean extraction** → accepted: the task for that file allows
  more than one extraction, and if a function still reads high after a
  reasonable split, that is recorded honestly (same convention as
  `sonar-ratings-baseline`'s "STILL OPEN" entries) rather than forced into
  an artificial one-shot fix.
- **This change could hit the same "the hour" constraint that deferred the
  work originally** → the risk-ordered task list (cheapest findings first)
  means a partial session still leaves a clean, verified subset landed
  rather than an all-or-nothing diff.

## Migration Plan

None — internal refactor, no schema, no data, no deploy step. Standard
build/vet/test verification per file, described above.
