## Context

`README.md` is 514 lines and carries four distinct audiences at once:

| Lines | Section | Audience |
|---|---|---|
| 1–59 | pitch, diagram, CRD table, behaviors | first-time reader |
| 61–107 | tool access is wiring | operator wiring a pipeline |
| 109–310 | channel / signal / work contracts | adapter or runtime author |
| 207–295 | VictoriaMetrics bundle | VM user only |
| 312–365 | demo, install, HTTP API | evaluator / operator |
| 367–503 | eight `## Migrating to chart …` guides | upgrader, once |
| 504–514 | development, status | contributor |

The migration guides are 137 lines — 27% of the file — and are ordered
1.0, 1.12, 1.8, 1.7, 1.6, 1.4, 1.3, 1.1, which is neither chronological nor
reverse-chronological. Every feature change appends another one plus a
reference section, so the file grows monotonically; `CLAUDE.md`'s current
"Update README.md when concepts/behavior change" is what makes that the path
of least resistance.

Constraint that shapes everything below: the prose is dense, load-bearing, and
correct. Several sections state invariants that exist nowhere else in
human-readable form (credential projection, token derivation contexts,
merge/overwrite semantics, the `continue: true` Alertmanager sharp edge).
Rewriting them risks introducing doc bugs that outlive this change. So the
move is mechanical: **cut and paste, adjust links and section levels, nothing
else**.

## Goals / Non-Goals

**Goals:**
- README readable end to end in one sitting (~120 lines) and answering only:
  what is this, what are the pieces, how do I try it, where is everything else.
- Every migration guide in one `CHANGELOG.md`, newest first, and an obvious
  home for the next one.
- Reference material preserved verbatim under `docs/`, reachable in one hop
  from the README.
- A convention recorded in `CLAUDE.md` so the README does not regrow.
- Zero content loss: every current README line lands in exactly one file.

**Non-Goals:**
- Rewriting, condensing, or fact-checking the moved prose (a separate pass if
  wanted).
- A docs site, generated API reference, or per-directory READMEs.
- Retrofitting in-flight changes (`visualize-agent-ops`, `k8s-bundle`,
  `ha-bundle`, `add-web-chat-channel`) whose tasks say "update README.md".
- Any code, CRD, chart, or test change.
- Backfilling changelog entries for versions that never had a migration note.

## Decisions

### D1. Three reference pages, split by audience — not one `docs/reference.md`, not one file per section

`docs/concepts.md` (what the CRDs are + tool access), `docs/contracts.md` (work,
channel, signal, HTTP API), `docs/vm-bundle.md` (the VM subchart). The split
follows who reads it: operator, integrator, VM user. One big file just moves the
scrolling problem; one file per section (six or seven) makes the README's
Documentation index as long as the content it replaced. VM bundle earns its own
page because it is optional, off by default, and irrelevant to everyone not
running VictoriaMetrics.

Tool access lives in `concepts.md` rather than its own page: it is 47 lines and
it explains what a `Pipeline`/`MCPToolset`/`MCPConfig` *means*, which is what
that page is for.

### D2. Moved sections are re-leveled, not re-written

Each moved `##` section becomes the `#` title of its page (or a `##` within it),
its subsections shift one level, and every relative link is re-resolved from
`docs/`. Since `docs/` is one level down, `channel-telegram/` becomes
`../channel-telegram/`. No sentence is edited except in-page anchors that no
longer resolve. This keeps the diff reviewable as a move and keeps `git log -M`
able to follow it.

### D3. The README's CRD table collapses to one line per kind, with the long cells moving to `docs/concepts.md`

The table is the single most useful thing in the README and the single biggest
one (16 lines, some cells 8 lines of rendered text). Keeping the kinds and
losing the detail preserves the "what are the pieces" answer at a glance; the
full cell text becomes a `### <Kind>` section in `docs/concepts.md`, so nothing
is lost and the detail gets *more* room than a table cell allows.

Alternative rejected: drop the table entirely and link out. The list of kinds
is the mental model — a reader who never opens `docs/` should still leave
knowing there are nine CRDs and roughly what each does.

### D4. `CHANGELOG.md` is migration-guide-shaped, not Keep-a-Changelog-shaped

Entries are `## chart <version> — <title>` newest first, each holding its
existing guide verbatim, with `— BREAKING` retained where the README had it.
The repo has no per-release Added/Fixed/Changed history to backfill and
inventing one would be fiction. A `## Unreleased` heading at the top gives the
next breaking change a place to land. The file is versioned by *chart* version
because that is what every existing guide is keyed to, and the manager image
tag moves independently.

### D5. `CLAUDE.md` gains a routing rule, replacing the line that caused the sprawl

"Update README.md when concepts/behavior change" becomes an explicit table:
concepts → `docs/concepts.md`, contracts → `docs/contracts.md`, VM bundle →
`docs/vm-bundle.md`, breaking upgrade steps → `CHANGELOG.md`, README only when
the pitch/kind list/demo/install changes — with the one-page budget stated as a
number so a future reader knows when they have overrun it. Stating the budget
is what makes the rule enforceable; "keep it concise" is not.

### D6. No `docs/README.md` index

The README's Documentation section is the index. A second index means two
places to forget to update, for three files.

### D7. Link integrity is verified mechanically, not by eye

A one-off script (not committed) extracts every relative markdown link and
anchor from `README.md`, `CLAUDE.md`, `CHANGELOG.md`, and `docs/*.md`, and
checks that each target path exists and each `#anchor` matches a heading in the
target file. Cross-file anchors are the failure mode this change creates most
of, and there are ~10 of them.

## Risks / Trade-offs

- [Detail becomes less discoverable — one Ctrl+F over README no longer finds
  everything] → Accepted and partly intended. Mitigated by the Documentation
  index naming what each page holds, and by GitHub's repo-wide search.
- [Content silently lost or duplicated during the cut-and-paste] → Verify by
  line accounting: current README is 514 lines; the sum of the new README plus
  the moved sections must reconcile, and a `git diff` review confirms moved
  blocks are byte-identical apart from heading level and links.
- [Broken links, especially anchors that pointed inside the README] → D7's
  mechanical check, run before the change is done.
- [In-flight changes merge README additions into sections that no longer exist]
  → Their tasks say "update README.md"; the D5 rule in `CLAUDE.md` redirects
  whoever applies them. A merge conflict here is loud, not silent.
- [The one-page budget is a number nobody enforces] → It is written in
  `CLAUDE.md` next to the routing table and stated as a line count, so it is
  checkable in one command. Not enforced in CI; a docs lint job would be
  disproportionate for a repo this size.
- [`CHANGELOG.md` implies a release cadence the project does not have] → The
  file documents chart-version migrations only, and says so in its first line.

## Migration Plan

Not applicable — documentation-only, no deployed artifact changes. Rollback is
`git revert` of a single commit.

## Open Questions

None blocking. Two deferred by choice: whether the moved reference prose should
later be condensed on its own merits (D2 says not here), and whether `docs/`
eventually becomes a published site (out of scope, and this layout is
site-generator-friendly either way).
