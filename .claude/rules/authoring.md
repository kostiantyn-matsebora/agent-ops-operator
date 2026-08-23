## Authoring rules (binding)

**They govern every `.claude/rules/*.md` file and `docs/.claude/`, not only the
pages under `docs/`.** A rule stated in prose here is as unfindable as one
stated in prose there.

**Concise and LLM-optimized.** Cut filler, marketing tone and preambles — every
sentence earns its tokens.

**Structure over prose:**

| Content | Shape |
|---|---|
| Steps | numbered list |
| Choices / mappings | table |
| "X means Y" | **X.** Y, on its own line |
| Multi-rule bullet | parent + sub-bullets, ONE rule per line |
| Prose paragraph stating > 2 rules | restructure |

**Reasoning is not filler.** A rule keeps the sentence saying what it cost —
that is what stops the next reader undoing it. What gets cut is the RESTATEMENT
of the rule, never its why.

**ONE TOPIC PER FILE, under `.claude/rules/`.** `CLAUDE.md` is the index.

**IT HOLDS A SHORT LIST OF NAMED EXCEPTIONS, AND THEY ARE NOT DRIFT.** A rule
earns a line there when being LOST AT COMPACTION would cost something the index
cannot recover — the statement itself has to be in scope, not merely findable.
Each one is a sentence or two that NAMES the rules file holding the detail:

| In `CLAUDE.md` | Detail in |
|---|---|
| a directory is a component | `structure.md` |
| documentation is part of every change | `documentation.md` |

- **DO NOT DELETE THESE AS INDEX VIOLATIONS.** They are deliberate, and this
  table is what says so — a future reader tidying `CLAUDE.md` back to pure
  index is undoing a decision, not enforcing one.
- **Adding a third needs the same test**, and the bar is high: nearly every
  rule is fine being loaded from its own file.

- **A new topic is a new file**, never a section appended to an existing one
  because it was open. Every `.md` there is discovered — there is nothing to
  wire.
- **Each file opens with its own `## ` heading** and uses `###` / `####` below
  it.
- **Cross-file references name the FILE.** "This file" survived the split into
  topics in five places and pointed at the wrong one in every case.

**`paths:` frontmatter scopes a rule to the files it names**, and it then loads
only when one of them is READ. A file without it loads at launch.

- **Scope a rule ONLY IF every way to get it wrong is preceded by reading a
  matching file.** Editing a file requires reading it first, so a rule whose
  whole harm is a BAD EDIT to named files qualifies — `chart.md` and
  `palette-and-mark.md` are the two.
- **A rule whose harm is a WRONG SENTENCE or a SHELL ACTION cannot be scoped.**
  Neither is a read: `wiring` is claimed in chat, `gotchas` fires on `helm` and
  `docker`, `build-test` on being asked to ship.
- **When a scoped rule is extracted, the always-loaded rule NAMES it.** The
  routing table carries the row, the detail loads with the files. Otherwise
  splitting a checklist deletes an item from it.
- **A scoped rule is LOST at compaction** until a matching file is read again.
  An unscoped one is re-injected from disk, exactly as the root `CLAUDE.md` is,
  which is the second reason to scope sparingly.
- **A topic belonging to ONE directory lives there instead**, in that
  directory's own `CLAUDE.md` or `.claude/`, with the root only MENTIONING it.
  `docs/` is the case.
