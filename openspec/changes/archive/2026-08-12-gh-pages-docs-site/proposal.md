## Why

The project has no adopter-facing landing page. `README.md` is the front door
on GitHub and the reference pages in `docs/` are raw markdown blobs — there is
nowhere to send someone that says what agent-ops is, in the product's own
identity, and starts them.

GitHub Pages can publish `docs/` as it stands. What is missing is a theme, and
the theme should not be a generic one: an adopter who has run the console and
then opens the site should recognise the same product — the same teal-and-violet
palette, the same Red Hat type, the same masthead-over-sidebar shell, the same
logo mark, the same light/dark/system switch. A stock Jekyll theme (Cayman,
minima, just-the-docs) would make the documentation look like somebody else's.

This change builds the **shell and the front door only**. Publishing the
existing reference pages as first-class site pages is a separate change that
inherits the theme this one lands.

## What Changes

- **A GitHub Pages site published from `master:/docs`** at
  `https://kostiantyn-matsebora.github.io/agent-ops-operator/`, using the
  branch-deploy build so no CI workflow and no local Ruby toolchain is required.
- **A hand-written Jekyll theme in `docs/`** (`_config.yml`, `_layouts/`,
  `_includes/`, `_data/nav.yml`, `assets/`), NOT a remote theme and NOT a
  PatternFly bundle — the console's component library is a React application's
  dependency and a prose site needs none of it.
- **The console's identity, carried over by its source of truth**: the `--ao-*`
  token block from `console/ui/src/theme/theme.css` is reproduced verbatim
  (both light and dark blocks), the masthead mark is the same inline SVG as
  `console/ui/src/components/Logo.tsx`, and the theme choice is the same
  three-state light/dark/system control with the same "system is the default and
  is a live subscription" behavior.
- **The Red Hat variable fonts are vendored** into `docs/assets/fonts/`
  (RedHatDisplayVF, RedHatTextVF, RedHatMonoVF — SIL OFL-1.1, the same files
  PatternFly ships), so an adopter's browser makes no third-party request and
  the page renders identically offline.
- **A site landing page** (`docs/index.md`) — the adopter hub: what the operator
  is, where to start, and grouped paths onward. Not a second copy of the README.
- **`README.md` is trimmed to what it is and how to start it** — pitch and
  diagram, the one-line-per-kind list, the demo and the install, and links
  onward with the site first. Everything that is neither identity nor quick
  start moves out or was already out.
- **The routing rule in `CLAUDE.md` gains the site**, so a future contributor
  knows the theme is not a place to put prose, prose is not a place to put
  theme, and the palette is a two-file change.

Not in scope, and deliberately left for the follow-up change: **publishing the
existing `docs/*.md` reference pages as first-class site pages** — their
navigation entries, their cross-page link behavior, their on-page contents and
the per-page constraints that go with having two consumers. They keep rendering
as GitHub blobs exactly as today. Also out: writing the new adopter guides,
search, versioned documentation, and a custom domain.

## Capabilities

### New Capabilities

- `docs-site`: the published documentation site — where it is built from and by
  what mechanism, that it carries the console's visual identity from the
  console's own tokens, its landing page, its navigation shell and its theme
  control; and the boundary that the existing reference pages are not yet site
  deliverables.

### Modified Capabilities

- `documentation-structure`: `README.md` narrows to what the operator is and how
  to start it, linking onward rather than carrying more; and the contributor
  routing rule gains the site — presentation versus prose, and the palette copy.

## Impact

- **New**: `docs/_config.yml`, `docs/_layouts/{default,home,page}.html`,
  `docs/_includes/` (masthead, sidebar, theme switcher, logo SVG, footer),
  `docs/_data/nav.yml`, `docs/assets/css/agentops.css`,
  `docs/assets/js/theme.js`, `docs/assets/fonts/*.woff2`, `docs/index.md`,
  `docs/assets/fonts/LICENSE.txt`, `docs/assets/img/agent-ops.svg` (the product
  diagram exported from `docs/diagrams/agent-ops.drawio`, icons inlined so the
  page still reaches no third party).
- **Modified**: `README.md` (trimmed to identity + quick start + links onward;
  must stay ≤150 lines — it is at exactly 150 today and should end well under),
  `CLAUDE.md` (routing rows + the note that `docs/` is now a Jekyll source and
  the palette is copied), `.gitignore` (`docs/_site/`, `docs/.jekyll-cache/`).
- **Unmodified**: every existing `docs/*.md`. No front matter, no heading
  changes, no link rewrites — they are outside this change's scope. Without front
  matter Jekyll copies them verbatim rather than rendering them, so they serve as
  raw markdown at their own URLs with nothing on the site linking to one; the
  minimal `page` layout is there for the change that publishes them.
- **Repository settings** (manual, one-time, by the maintainer): Settings →
  Pages → Deploy from a branch → `master` / `/docs`.
- **No effect on any Go module, the chart, or the console**. Nothing in `docs/`
  is compiled, embedded or shipped in an image; the font files are copied out of
  `console/ui/node_modules` once, not linked to it.
