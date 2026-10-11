## Retired vocabulary (names this project stopped using)

**A REMOVED FIELD, A WITHDRAWN RULE OR A SUPERSEDED COMMAND MUST NEVER APPEAR
AS A CURRENT CLAIM.** May appear only as an explicit record that it was
removed. `openspec/specs/` is PUBLISHED and trusted, so a wrong entry is
worse than none.

**IT IS A DENYLIST** (unlike `publication.md`'s allowlist) — retired
vocabulary is PUBLIC BY CONSTRUCTION, so listing it is free and the list IS
the value.

| Where | Is |
|---|---|
| `.github/retired-vocabulary.json` | the terms: pattern, replacement, record-marker words |
| `.github/scripts/retired-vocabulary-guard.py` | the scan |
| `ci.yml`, job `retired-vocabulary` | the enforcement |

```sh
python3 .github/scripts/retired-vocabulary-guard.py            # file, line, term
python3 .github/scripts/retired-vocabulary-guard.py --show     # LOCAL ONLY
python3 .github/scripts/retired-vocabulary-guard.py --counts   # what a task file may record
```

- **Reports file/line/term, never the matched line** — public build logs.
- **Window: one line either side, capped at 240 chars** — line-scoped alone
  fails hard-wrapped prose, paragraph-scoped lets one "removed" far away
  launder a reintroduced term.
- **Add the term in the SAME change that retires the thing.**
- **`says` states the REPLACEMENT**, not the prohibition.
