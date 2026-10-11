## Publication hygiene (this repository is read by strangers)

**NO FILE AND NO COMMIT MESSAGE MAY SAY WHO RUNS THE AUTHOR'S CLUSTER, WHAT
IT IS CALLED, OR HOW TO REACH IT.** `.github/scripts/publication-guard.py`
enforces it in CI over the whole tree plus commit messages.

- **A shipped example carries a PLACEHOLDER, never a real value** — one per
  kind, reused. A fixture PERSON is named for the ROLE it exercises. No
  placeholder yet → add the allowlist entry FIRST, never loosen the regex.
- **Record the VERDICT, never the TEXT** — "the guard passes" or a count per
  rule, in a task/chat/commit/PR. A pasted grep result reintroduces exactly
  what was removed, and archiving republishes it. A findings list is a local
  artifact, never committed.

```sh
python3 .github/scripts/publication-guard.py            # verdict, positions, rules
python3 .github/scripts/publication-guard.py --counts   # what a task file may record
python3 .github/scripts/publication-guard.py --show     # LOCAL ONLY
```

- **IT IS AN ALLOWLIST** (`.github/publication-allowlist.json`) — reserved
  domains, cluster-internal names, loopback, this repo's clone url,
  placeholders, linked upstream projects. A denylist would publish what it
  protects. Nothing in the allowlist may name a real person/host/identifier.
- **The guard reports file/line/rule, never the matched text.**
- **The guard runs BEFORE any cleanup** — an artifact naming a literal FAILS
  THE BUILD instead of reaching the archive.
