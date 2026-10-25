# Design — crd-spec-pages

## Context

Four site surfaces change at once: the landing opening, the Introduction's
concepts section, a new page kind, and the sidebar. The constraints that
shape the approach:

- GitHub Pages branch deploy — no build workflow, every generated asset a
  committed file with a CI check behind it (`docs-generate.py --check`).
- An `<img>`-loaded SVG is inert — links inside it do not work, so a
  clickable diagram must be inlined into the page.
- `chart/crds/` holds fourteen kinds. `ControlPlaneProbe` declares itself
  disposable scaffolding, so the pages, the diagram and the nav carry the
  thirteen adopter-facing kinds, and the generator excludes the fourteenth
  by name — with a check that the exclusion dies with the kind.

**The approved mockup is committed at `mockups/index.html`** in this change
(also published in-session at
<https://claude.ai/artifact/Bqb9C818Feq52BtziKN3bz>). Tab 1 is the landing
opening, tab 2 the domain-model diagram, tab 3 the spec-page shape, tab 4 the
collapsible nav.

**The mockup is the reference for COMPOSITION, the specs for BEHAVIOUR.**

- The mockup settles geometry, badge/text alignment, cluster layout,
  connector routing, the spec-page section order and the YAML comment style.
  Those numbers are written down nowhere else.
- The specs alone say what a page must do — link, generate, fail CI, degrade
  without script.

Where the mockup is deliberately NOT the target:

- its colors and fonts approximate the theme — the site's own `--ao-*` tokens
  and self-hosted Red Hat families are authoritative
- its diagram links jump to a tab — the site links to real `/crds/<kind>/`
  pages
- its field table shows `AgentProfile` only — the site generates one per kind
- its nav tab shows static open/closed states — the site persists the
  reader's choice
- its hand-set SVG sizes (`max-width: 36rem`) yield to the site's measure
  rules

## Goals / Non-Goals

**Goals:**

- One concise published spec page per CRD, generated fields, hand prose.
- The Introduction's diagram as the model's map, every kind clickable.
- The landing opening static, the presentation machinery gone.
- Collapsible nav groups that degrade to fully open.

**Non-Goals:**

- No change to `docs/concepts.md` or `docs/contracts.md` — they stay
  reference pages on the forge.
- No change to the three adapter/runtime contract pages.
- No redesign of the guides or integration pages.
- No console changes — the glyphs are copied from `shapes.tsx`, not moved.

## Decisions

**The diagram is an inline SVG fragment, not a drawio export.** Clickable
kinds are the point, and an `<img>`-loaded export cannot carry links.

The fragment lives under `docs/_includes/` and the Introduction inlines it
with `{% raw %}{% include %}{% endraw %}`. It themes itself with
`currentColor` and the `--ao-*` tokens, so one file serves both themes and
`themed.js` is not involved.

Alternative considered: a new `agent-ops.drawio` page via `export.py` —
rejected because exported links die inside `<img>`, and an `<object>` embed
breaks the theme-token inheritance.

**Glyphs are copied from the console's `shapes.tsx`, verbatim.** The path
data in `platform/console/ui/src/graph/shapes.tsx` is clearance-tested and
already the product's visual vocabulary. The SVG carries them as plain
`<path>` elements. A comment in the fragment names the source file so a
glyph change there prompts a copy here.

**Edge labels are checked, not trusted.** The diagram fragment marks each
labelled edge with a `data-field` attribute, and `docs-generate.py --check`
fails when a named field is absent from `chart/crds/`.

This is what lets the Introduction keep its "no reference detail" rule while
the diagram names `signalSourceRefs`.

**Per-CRD pages are marker-generated like the guides.** Each
`docs/crds/<kind>.md` carries front matter, hand prose, and two generated
blocks: `crd-example` (the commented YAML instance) and `crd-fields` (the
field table).

`docs-generate.py` grows the two block kinds and a set check — page set and
CRD set must match, both directions. The `cr-reference` generation is
deleted in the same edit.

Alternative considered: keeping `cr-reference.md` alongside — rejected, two
statements of every field.

**Example instances come from the same source the guides' do** — the
resource templates `docs-generate.py` already derives from the CRDs, trimmed
to one realistic instance per kind with one comment per top-level field.

**Collapsible nav is `<details>`/`<summary>` per group.** The sidebar
include wraps each group, rendered `open` for every group.

A small script then closes the groups the reader closed before (persisted in
`localStorage`) and always opens the current page's group. No script:
everything stays open, matching the spec.

`<summary>` is natively keyboard-operable and states its expanded state, so
no ARIA wiring is hand-built. Alternative considered: class-toggling
buttons — rejected, re-implements what `<details>` ships.

**The landing picture reuses `readme-flow-{light,dark}.svg` as-is.** Named
once by the page, resolved by `themed.js`, exactly as other themed images.
`readme-flow.py` is untouched. The presentation's CSS, JS and layout hooks
are deleted, `.ao-presentation` goes into `.github/retired-vocabulary.json`
in the same change, and `docs/CLAUDE.md` plus `docs/.claude/site.md` drop
their presentation sections.

## Risks / Trade-offs

- [The inlined SVG bloats the Introduction's HTML] → one diagram of ~100
  elements, well under a page of prose in bytes. Acceptable.
- [Glyph drift between console and diagram] → the fragment names its source
  file. Accepted as a copy, same as the `.ao-icon-*` glyphs the site already
  copies.
- [Thirteen new nav entries crowd the sidebar] → that is what the collapsible
  groups part of this change exists to absorb — the CRDs group ships
  collapsed unless current.
- [Generated YAML examples read generic] → the generator emits the skeleton,
  the page's hand prose carries meaning, and comments come from a per-kind
  table in the generator script, editable as code.
- [Deleting `cr-reference.md` breaks inbound links] → every in-repo pointer
  is updated in this change. External deep links to a raw GitHub page were
  never a stable URL.

## Open Questions

None.
