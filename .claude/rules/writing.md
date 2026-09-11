## Writing (every markdown file)

Everything in a repository is written to be SCANNED: a rule, a page, a README,
an agent's instructions, a pull request. The markdown is the structure.

- **Structure first.** A procedure is NUMBERED STEPS. A mapping is a TABLE. A
  set is BULLETS. The claim a page rests on is a callout. *A paragraph that
  enumerates is a list that has not been written yet.*
- **Short sentences, one idea each.** Three clauses is three sentences. If it
  has to be read twice, it is wrong.
- **NO SEMICOLONS.** A `;` is a full stop that lost its nerve. It is the tell
  of a sentence that should have been two, and it is forbidden whatever the
  grammar allows.
  - **THE SEMICOLON IS THE SYMPTOM, AND THE FULL STOP IS NOT THE CURE.**
    Swapping one for the other keeps the two crammed thoughts and just makes
    them choppier. Ask which structure the sentence wanted, usually a list or
    a table, and write that.
  - **ONE THOUGHT PER SENTENCE, NOT ONE CLAUSE.** Pulling a single thought in
    half reads as badly as cramming two together. "Removes the workloads. It
    leaves the CRDs" hides the contrast that "removes the workloads but leaves
    the CRDs" states. A sentence carrying one idea in three clauses is finished
    as it is.
  - **The tell of overcorrecting is a verbless fragment.** "Two volumes." and
    "It is below." were both shipped by applying the rule above mechanically,
    and both had to be written again.
- **Small paragraphs.** Past about three lines it stops being read.
- **Emphasise the load-bearing phrase**, not the sentence around it.
  Everything bold means nothing bold.
- **Cut what earns nothing.** Reasoning belongs in the reference page that
  owns it, in a rules file, or in the commit message.

**The failure mode is recognisable and has been shipped more than once:** long
compound sentences, every point explained twice, nothing scannable. Prose
doing a table's job.

Reference pages may be dense. A page a reader meets first may not.

### Tables

- **Code never wraps in a table.** A stylesheet that wraps a long key across
  lines invents a key that does not exist. Widen the column instead.
- **Watch the last column.** Two long code values in a three-column table
  crush the description to two words a line. When that happens the table is
  the wrong shape. Use two columns, or a snippet.
- **Give every table a header row.** A headerless table renders as an empty
  strip and reads as a rendering accident.

### Checked, not remembered

These rules were written and then broken on the very next page, twice, and
caught each time by the reader rather than the writer. So they are a program:

- **`.claude/scripts/rules_compliance.py`** reports `file:line rule` for a
  semicolon in prose, a paragraph over 45 words, and a rules file that does
  not open with one `## ` heading. Never the text.
- **The `rules_compliance` hook** runs it after every markdown `Write` or
  `Edit` and hands the findings back. Fix them, then continue.
- **Over a whole tree**, by hand:

```sh
python3 .claude/scripts/rules_compliance.py $(git ls-files '*.md')
```

### The binding statement

The shape rules, stated once and quoted, never paraphrased:

    Concise + LLM-optimized. Cut filler, marketing tone, preambles. Every sentence earns its tokens.
    Structure over prose:
    Steps → numbered list.
    Choices / mappings → table.
    "X means Y" → **X.** Y on its own line.
    Multi-rule bullet → parent + sub-bullets, one rule per line.
    Prose paragraph stating > 2 rules → restructure.
