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
| Terminology | `.claude/terminology.md`, `.claude/wiring.md`, `.claude/adapters.md` |
| Invariants | `.claude/invariants.md` |
| Hard-won gotchas | `.claude/gotchas.md` |
| What the console is FOR — its views, what each answers, the authentication decision | `docs/console-guide.md` |
| What the console IS — endpoints, RBAC grant, values reference, internals | `docs/console.md` |
| A change to the console's UI | re-run BOTH `npm run screenshots` and `npm run demo` in `console/ui` — the site's screenshots and its landing recording are build output, and the change is not done until both match |
| The pitch, the kind list, the demo, the install command | `README.md` |
| The site's SHELL — Jekyll source, diagrams, what each page owes beyond its row | `docs/.claude/site.md` |
| How the site LOOKS or navigates | `docs/_layouts/`, `_includes/`, `_data/nav.yml`, `assets/` |
| What the site SAYS to an adopter | a markdown page under `docs/` |
| How a page READS — structure, tabs, components, tables, the lint | `docs/CLAUDE.md` |

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

### The palette and the mark are COPIED, and the copy is one block

**The site's `--ao-*` palette is copied from `console/ui/src/theme/theme.css`**
into the token blocks at the head of `docs/assets/css/agentops.css`. Changing a
token is a TWO-FILE change.

**The copy is deliberate and one-directional** — a Jekyll site must not need a
Node build to publish a paragraph.

**What makes it survivable is that no colour is stated literally anywhere else
in the site CSS**, so the sync is one block, not a hunt:

```sh
grep -n '#[0-9a-fA-F]\{3,6\}' docs/assets/css/agentops.css
```

That must return hits only inside those blocks.

**The theme-choice semantics are copied on the same terms** —
`assets/js/theme.js` from `theme/useTheme.ts`.

**The MARK is copied on those terms across THREE files:**

| File | Is |
|---|---|
| `console/ui/src/components/Logo.tsx` | the source |
| `docs/_includes/logo.svg` | the masthead's theme-driven copy |
| `docs/assets/img/logos/agent-ops.svg` | the standalone one an `<img>` can load |

The standalone one states its colours literally, because an `<img>` is its own
document and inherits no custom properties.

**Integration marks sit beside it**, committed unaltered from each project's own
source with their terms in that directory's README, and the PAGE names each
file. A vendor list in the stylesheet would be product knowledge in the theme.

### README.md has a budget: 150 lines

`wc -l README.md`.

**It holds** the pitch and diagram, one line per CRD kind, the behaviors that
matter, the demo, install, the Documentation index (the site first), development
and status. **Nothing else.**

- **A distinguishing behavior is named in a LINE**, and the document that owns
  it is linked.
- **Reference material and migration guides do not belong in it.**
- **If it is over budget, something is in the wrong file.**
