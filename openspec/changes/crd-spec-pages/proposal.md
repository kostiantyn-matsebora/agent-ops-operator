# crd-spec-pages

## Why

The adopter site explains three extension-point contracts in full but not one
of the thirteen CRDs an adopter actually writes. The only field reference is
`cr-reference.md`, a generated, nav-less dump read on GitHub.

Three smaller debts sit beside that gap:

- the Introduction describes the model as nine static cards
- the landing page's "How it works" tab runs an animated presentation that
  restates what `README.md` already shows as one still picture
- the sidebar has grown past comfortable scanning

Mockups for all four parts were built and approved in-session:
<https://claude.ai/artifact/Bqb9C818Feq52BtziKN3bz> (copy committed at
`mockups/index.html` in this change).

## What Changes

- **A concise spec page per CRD** on the adopter site, one per kind, each the
  same promotion the three adapter contracts received: front matter, its own
  permalink, a nav entry. Shape per the approved mockup (tab 3): a commented
  "spec, at a glance" YAML instance, an "Every field" table, a closing stat
  line. Field tables are generated from `chart/crds/`, prose is hand-written.
- **`cr-reference.md` is replaced by those pages** and deleted, with its
  generator section in `docs-generate.py` retargeted at the per-kind tables.
- **The Introduction's nine concept cards become the domain-model diagram**
  from the mockup (tab 2): thirteen kinds in four clusters, console glyphs,
  one connector per relationship that is really unidirectional, every shape a
  link to that kind's spec page. A short description per kind stays beside it.
- **The landing page's "How it works" tab shows the static flow picture**
  (`readme-flow-{light,dark}.svg`, already generated for `README.md`) instead
  of the animated presentation. **BREAKING for the theme**: the
  `.ao-presentation` component is deleted — the landing page was its only
  user.
- **Sidebar nav groups become expandable/collapsible** (mockup tab 4), the
  current page's group open, state kept per-reader.

## Capabilities

### New Capabilities

- `crd-spec-pages`: a published, concise specification page per CRD — its
  shape, its generated field table, its nav group, and the links that reach it
  from the Introduction diagram.

### Modified Capabilities

- `docs-site`: the Introduction orients with the linked domain-model diagram
  instead of the card set. The landing page's "How it works" is the static
  picture. Nav groups are collapsible.
- `landing-presentation`: retired. The landing page no longer declares a
  presentation and the theme no longer ships the component.

## Impact

Reference docs:

- `docs/cr-reference.md` — deleted, replaced by the per-CRD pages
- `.github/scripts/docs-generate.py` — cr-reference generation becomes
  per-kind field-table generation, CI check follows
- `docs/CHANGELOG.md` — entry for the removed page and its replacement

Adopter site:

- `docs/index.md` — presentation beats replaced by the picture
- `docs/introduction.md` — cards replaced by the diagram plus short
  descriptions
- `docs/crds/*.md` — thirteen new pages
- `docs/_data/nav.yml` — new group for the CRD pages
- `docs/_layouts/`, `docs/_includes/`, `docs/assets/` — presentation
  component deleted, collapsible sidebar added, diagram asset added
- `README.md` — "The reference" pointer moves from GitHub links to the site's
  CRD pages

Context docs:

- `docs/CLAUDE.md` — page table, `.ao-presentation` component row
- `docs/.claude/site.md` — landing-presentation sections
- `.github/retired-vocabulary.json` — `.ao-presentation` retired in the same
  change that removes it
