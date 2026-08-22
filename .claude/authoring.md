## Authoring rules (binding)

**They govern every `.claude/*.md` file, not only the pages under `docs/`.** A
rule stated in prose here is as unfindable as one stated in prose there.

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

**ONE TOPIC PER FILE, and `CLAUDE.md` is only the index.** The root holds the
title, one orientation line and the `@.claude/<topic>.md` imports — nothing
else.

- **A new topic is a new file plus one import line**, never a section appended
  to an existing file because it was open.
- **Each file opens with its own `## ` heading** and uses `###` / `####` below
  it.
- **Import order is reading order.** Preserve it when adding one.
- **Cross-file references name the FILE.** "This file" survived the split in
  five places and pointed at the wrong one in every case.
