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

**ONE TOPIC PER FILE, under `.claude/rules/`.** `CLAUDE.md` is the index and
holds nothing else.

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
  matching file.** Most of these fail that test: they fire on a shell command,
  on a chat answer, or while writing a file that does not exist yet, and none of
  those is a read.
- **`chart.md` is the only one that passes** — a chart mistake is a chart edit.
- **A topic belonging to ONE directory lives there instead**, in that
  directory's own `CLAUDE.md` or `.claude/`, with the root only MENTIONING it.
  `docs/` is the case.
