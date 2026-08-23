## After changes

**DOCUMENTATION IS PART OF THE CHANGE, NOT A FOLLOW-UP. THIS IS NOT
NEGOTIABLE.**

### EVERY OPENSPEC CHANGE ENDS WITH A DOCUMENTATION TASK

**A dedicated task, its own section, and the LAST one before the change is
finished.**

- **Not a bullet inside another task.** A docs line appended to "implement X" is
  the line that gets ticked with X and never done.
- **Not first, and not in the middle.** Written last, it documents what the
  change ACTUALLY did — which is routinely not what the proposal said it would.
- **BOTH HALVES, listed separately in the task**, because they are skipped
  independently:
  1. the reference docs (`docs/concepts.md`, `docs/contracts.md`, a bundle page)
  2. **the ADOPTER SITE** — the landing page, Introduction, Getting started,
     Installation, the guides
- **An `/opsx:archive` with the task unticked is a change reported as finished
  while half of what a reader meets is stale.** Archiving is what makes it
  permanent.

**The archive is the deadline, and by then it is already too late to notice.**

**IT IS ENFORCED IN TWO PLACES, NEITHER OF WHICH IS THIS FILE.** A rule stated
in prose is followed until the evening somebody is tired.

| Where | Does |
|---|---|
| `openspec/config.yaml` — `rules.tasks` / `rules.proposal` | injected into the instructions each time a tasks or proposal file is GENERATED, so the section is written in the first place |
| `.claude/hooks/require-docs-task.sh` — a `PreToolUse` hook on Bash | **REFUSES `openspec archive`** when the last section is not documentation, or when its tasks are unticked |

- **The config half reaches the model at the moment it writes the file**, where
  a context file may already have been compacted away.
- **The hook half does not depend on the model at all.** The harness runs it.
- **ARCHIVING IS THE GATE** because it is the point of no return: the deltas
  fold into the specs and nobody revisits the change.
- **The hook FAILS OPEN** on a store-backed change, an unreadable path or a
  missing tasks file. One that blocks work it does not understand gets disabled,
  and then it enforces nothing.
- **This is not the session-title hook.** That one was deleted for competing
  with Claude Code over the terminal title, a race it could not win. This one
  validates a file and blocks one command, and nothing else writes that
  decision.



Before a change is committed, and certainly before it is ARCHIVED, every
document the change made untrue is already updated — in the same commit, not a
later one.

Archiving a change whose docs still describe the old behaviour records the work
as finished when the half a reader meets is not.

**That explicitly INCLUDES the adopter pages on the site.** It is the half most
often skipped, because a behaviour change feels done once `concepts.md` is right
— and the adopter never reads `concepts.md`.

Ask of every change: does the landing page, the Introduction, Getting started or
Installation now say something that is no longer true, or fail to mention
something an adopter must now decide?

**A page promising a step the chart just automated is as wrong as a stale field
name.**

### Where documentation goes

Keep commits scoped to this directory, and write documentation to the file that
OWNS that kind of content.

"Update README.md" is what grew it to 969 lines — three documents wearing one
filename — so the routing is explicit:

| What changed | Where it goes |
|---|---|
| CRD fields, semantics, how capabilities resolve | `docs/concepts.md` |
| Work contract, adapter contracts, HTTP endpoints | `docs/contracts.md` |
| A subchart's components or values | `docs/<bundle>.md` |
| The PARENT chart's values, install, upgrade, uninstall | `docs/installation.md` |
| Breaking change + upgrade steps | `docs/CHANGELOG.md`, newest first |
| Terminology | `.claude/rules/terminology.md`, `wiring.md`, `adapters.md` |
| Invariants | `.claude/rules/invariants.md` |
| Hard-won gotchas | `.claude/rules/gotchas.md` |
| What the console is FOR — its views, what each answers, the authentication decision | `docs/console-guide.md` |
| What the console IS — endpoints, RBAC grant, values reference, internals | `docs/console.md` |
| A change to the console's UI | re-run BOTH `npm run screenshots` and `npm run demo` in `platform/console/ui` — the site's screenshots and its landing recording are build output, and the change is not done until both match |
| A CRD field, an api doc comment, or a chart value a guide shows | re-run `python3 .github/scripts/docs-generate.py` — every CR template, every worked example and `docs/cr-reference.md` are build output, and CI fails on a stale one |
| A guide under `docs/guides/` | the page's prose is hand-written, its `yaml` blocks are NOT — edit the `<!-- generated: … -->` marker and re-run the generator |
| The pitch, the kind list, the demo, the install command | `README.md` |
| The site's SHELL — Jekyll source, diagrams, what each page owes beyond its row | `docs/.claude/site.md` |
| A colour token, the theme choice, or the mark | `.claude/rules/palette-and-mark.md` — always a TWO-FILE change |
| How the site LOOKS or navigates | `docs/_layouts/`, `_includes/`, `_data/nav.yml`, `assets/` |
| What the site SAYS to an adopter | a markdown page under `docs/` |
| How a page READS — structure, tabs, components, tables, the lint | `docs/CLAUDE.md` |

**A GENERATED BLOCK IS NEVER EDITED IN PLACE.** The three generated rows above
are the same rule as the screenshots: the source is elsewhere, and a hand-edit
is reverted by the next run — silently, because the run reports success.

- **The marker is the interface.** A page declares `kind=`, `name=` and
  `fields=`, and the generator fills the block from the CRDs or from a chart
  render. Change what a page teaches by changing the marker.
- **A field the CRDs no longer have FAILS the generator**, naming the file, the
  marker and the command. That is the point of generating rather than typing.

**Both value rows are "values", so the split is stated.** The PARENT chart's
belong to `docs/installation.md`, a SUBCHART's to that bundle's own page, and
neither restates the other.

**`installation.md` carries the values an operator must DECIDE**, grouped by the
decision they serve. `helm show values` is the exhaustive list, and a
hand-copied inventory rots.

**The last three rows are one rule read three ways: the theme holds no prose,
the pages hold no theme, and neither holds the rules.**

- A layout or include that starts explaining a CRD is in the wrong file.
- So is a markdown page that opens with a `<div>` or an inline style.
- Adding a page to the site is a page plus one line in `_data/nav.yml`, never
  navigation markup written a second time.

### README.md has a budget: 150 lines

`wc -l README.md`.

**It holds** the pitch and diagram, one line per CRD kind, the behaviors that
matter, the demo, install, the Documentation index (the site first), development
and status. **Nothing else.**

- **A distinguishing behavior is named in a LINE**, and the document that owns
  it is linked.
- **Reference material and migration guides do not belong in it.**
- **If it is over budget, something is in the wrong file.**
