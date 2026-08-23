## After changes

**DOCUMENTATION IS PART OF THE CHANGE, NOT A FOLLOW-UP.**

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
