## Retired vocabulary (names this project stopped using)

**A REMOVED FIELD, A WITHDRAWN RULE OR A SUPERSEDED COMMAND MUST NEVER APPEAR AS
A CURRENT CLAIM.** It may appear as an explicit record that the thing was
removed, and the difference between those two is the whole of this rule.

`openspec/specs/` is PUBLISHED and read as the contract. A spec is trusted, so a
wrong one is worth less than none.

### IT IS A DENYLIST, AND THAT FOLLOWS FROM THE SUBJECT

`publication.md` argues an ALLOWLIST, because naming a private thing in a guard
publishes it. Here the opposite holds:

- **Retired vocabulary is PUBLIC BY CONSTRUCTION.** Listing it is free.
- **The list IS the value** — the record of what this project stopped doing, in
  the one place that fails a build when someone brings it back.

**Two guards, opposite shapes, each following from what its subject is. NEITHER
IS A TEMPLATE FOR THE OTHER**, and "make them consistent" is a change that
breaks one of them.

### The mechanism

| Where | Is |
|---|---|
| `.github/retired-vocabulary.json` | the terms: pattern, what to write instead, and the words that mark a sentence as a RECORD |
| `.github/scripts/retired-vocabulary-guard.py` | the scan |
| `ci.yml`, job `retired-vocabulary` | the enforcement |

```sh
python3 .github/scripts/retired-vocabulary-guard.py            # file, line, term
python3 .github/scripts/retired-vocabulary-guard.py --show     # LOCAL ONLY
python3 .github/scripts/retired-vocabulary-guard.py --counts   # what a task file may record
```

**THE REPORT NAMES FILE, LINE AND TERM AND NEVER THE MATCHED LINE**, on the same
grounds `publication.md` gives: public build logs. `--show` is for local fixing.

### THE WINDOW IS ONE LINE EITHER SIDE, BOUNDED TO 240 CHARACTERS

Both halves were paid for:

- **LINE-scoped fails on correct text.** The prose is hard-wrapped, so the word
  marking a sentence as a record lands on the line above about half the time. A
  guard that fails for a reason invisible from its own message gets turned off.
- **PARAGRAPH-scoped launders.** One "removed" anywhere in a paragraph passes an
  assertion at its other end — and the paragraph listing every removed `Channel`
  field is exactly where a reintroduced `spec.type` would be written. Verified:
  it passed.

### ADDING A TERM IS THE NORMAL WAY TO RETIRE SOMETHING

When a change removes a field, a rule or a command, the entry lands **in the
same change**. Afterwards nobody remembers the name, which is the point — and
also why the guard has to.

**`says` is not decoration.** It is what the next reader sees instead of the
retired name, so it states the REPLACEMENT rather than the prohibition.
