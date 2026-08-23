## Publication hygiene (this repository is read by strangers)

**NO FILE AND NO COMMIT MESSAGE MAY SAY WHO RUNS THE AUTHOR'S CLUSTER, WHAT IT
IS CALLED, OR HOW TO REACH IT.**

`.github/scripts/publication-guard.py` enforces it in CI, over the WHOLE tree —
`openspec/`, `docs/`, `chart/`, every module, `.claude/` included — plus the
commit MESSAGES of the range under review.

### A SHIPPED EXAMPLE CARRIES A PLACEHOLDER, NEVER A REAL VALUE

Documentation, chart values, sample manifests and test fixtures are COPIED by
adopters and READ by strangers. Every identifier in one — a hostname, a clone
url, a chat or group id, an address literal, a person — is a documented
placeholder the allowlist permits by name.

- **This holds even when the real value is more convincing.** An example that
  works when pasted unchanged is a leak that looks like a courtesy. One shipped
  for months as *the value to copy*, sitting beside the obviously-fake
  placeholder that was there first.
- **ONE placeholder per kind.** Where one already exists, it is reused rather
  than a second invented beside it — two placeholders for one kind is how a real
  value gets chosen as the "better" example.
- **A fixture PERSON is named for the ROLE the fixture exercises**, never for
  anybody. The test then says what it is testing rather than who was typing.
- **No placeholder for that kind yet? The allowlist entry lands FIRST**, and the
  real value is not committed in the meantime. A legitimate new reference is an
  allowlist ENTRY, never a loosened regex.

### THE ONE RULE FOR EVERY CHANGE: RECORD THE VERDICT, NEVER THE TEXT

**A verification is written down as "the guard passes", or as a COUNT PER
RULE. Never as the value it matched.**

- **Pasting a grep result into a ticked task is the most natural way to
  reintroduce exactly what was removed.** A change that cleans an identifier out
  of the tree, and names that identifier in its own tasks file, has moved it —
  and archiving republishes it permanently.
- **The same applies to a chat answer, a commit message and a PR description.**
  The guard cannot read a sentence somebody typed about it.
- **A findings LIST is a local artifact.** It is the input to a cleanup, and it
  is never committed.

```sh
python3 .github/scripts/publication-guard.py            # verdict, positions and rules
python3 .github/scripts/publication-guard.py --counts   # what a task file may record
python3 .github/scripts/publication-guard.py --show     # LOCAL ONLY — prints matches
```

### IT IS AN ALLOWLIST, AND INVERTING IT IS THE REGRESSION

`.github/publication-allowlist.json` declares the SHAPES that may be published —
reserved example domains, cluster-internal names, loopback and the documentation
address ranges, this repository's own clone url, the documented placeholder
identifiers, and the upstream projects the docs link.

- **A denylist has to spell out the thing it protects**, which publishes it in
  the guard itself, and it catches only what somebody already thought of.
- **Nothing in the allowlist names a person, a private host or a real
  identifier.** That property is checkable by reading the file, and it is worth
  checking on every edit to it.
- **A legitimate new reference is an ALLOWLIST entry**, never a loosened regex.

### THE REPORT DOES NOT REPUBLISH WHAT IT CAUGHT

**File, line and rule. Never the matched text.** A public repository has public
build logs, so a guard that quoted its findings would leak them to the same
audience it protects the tree from. `--show` exists for local fixing and is
never used in CI.

### THE GUARD LANDS BEFORE ANY CLEANUP, AND THAT ORDERING IS THE POINT

A change that removes identifying content has to NAME what it removes. Archived,
that naming is republished by the very repository it was cleaning. With the
guard already in CI, an artifact that names a literal FAILS THE BUILD instead of
reaching the archive.
