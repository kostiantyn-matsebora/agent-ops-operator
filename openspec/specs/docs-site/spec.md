# docs-site Specification

## Purpose
TBD - created by archiving change gh-pages-docs-site. Update Purpose after archive.
## Requirements
### Requirement: The site is built from docs/ by GitHub Pages branch deploy

The repository SHALL publish its documentation site from the `docs/` directory
of the default branch, built by GitHub's own Jekyll build ("Deploy from a
branch", folder `/docs`). The site SHALL NOT require a CI workflow, a committed
`Gemfile.lock`, or any locally installed Ruby, Node or Jekyll toolchain to
publish.

`docs/_config.yml` SHALL set `baseurl` to the repository path
(`/agent-ops-operator`) and `url` to the Pages origin, and every internal link
the theme emits SHALL be prefixed with `site.baseurl` so the site is correct at
a project-pages sub-path rather than only at a domain root.

Plugins SHALL be limited to those GitHub Pages enables by default. A feature
that needs a plugin outside that set SHALL be dropped or implemented in the
theme's own CSS/JS rather than switching the build to a workflow.

#### Scenario: Maintainer enables Pages

- **WHEN** the maintainer sets Settings → Pages → Deploy from a branch →
  `master` / `/docs` and pushes
- **THEN** the site builds with no workflow file present in the repository
- **AND** the landing page is reachable at
  `https://<owner>.github.io/agent-ops-operator/`

#### Scenario: A link is followed at the project sub-path

- **WHEN** a visitor follows any navigation link, stylesheet, font or script
  reference the theme emits
- **THEN** the URL includes the `/agent-ops-operator` base path
- **AND** no request 404s because it was resolved against the domain root

#### Scenario: A contributor wants to add a Jekyll plugin

- **WHEN** a desired feature would require a plugin GitHub Pages does not enable
- **THEN** it is implemented in the theme's own assets or dropped
- **AND** the branch-deploy build is not replaced by an Actions workflow

### Requirement: The site delivers the theme and the landing page only

The site's deliverables SHALL be the theme and `docs/index.md`. The existing
`docs/*.md` reference pages SHALL NOT be edited for the site — no YAML front
matter added, no headings changed, no links rewritten — SHALL NOT appear in the
site navigation, and SHALL NOT be treated as published documentation by the
landing page, which links to them where they live until a later change takes
them onto the site.

Because branch deploy builds the whole source directory, those pages ARE part of
the build. Carrying no YAML front matter, they are static files to Jekyll: they
SHALL be copied to the site verbatim, unconverted and unstyled, and SHALL remain
reachable by URL only. The theme SHALL provide the layout the build would assign
a page that gains front matter without naming one, so that no page can ever fail
the build for want of a layout; that layout carries no feature beyond a title
and the page content.

#### Scenario: A reference page is built

- **WHEN** the site is built with the untreated `docs/*.md` pages present
- **THEN** the build succeeds and each page is copied verbatim, unconverted
- **AND** no reference page has been modified in the repository
- **AND** nothing on the site links to one

#### Scenario: A reader looks for the reference documentation

- **WHEN** a visitor on the landing page follows a path to the CRD reference
- **THEN** the link goes to the page where it lives today
- **AND** no sidebar entry claims it as a site page

#### Scenario: A contributor is tempted to fix a page for the site

- **WHEN** a reference page would need front matter, a heading change or a
  rewritten link to look right on the site
- **THEN** the page is left alone and the work is deferred to the change that
  publishes the reference pages

### Requirement: The site is recognisably the same product as the console

The site SHALL carry the console's visual identity so that an adopter who has
used the console recognises the documentation as the same product. Specifically
it SHALL reproduce:

- **The palette**, as the `--ao-*` custom-property block copied verbatim from
  `console/ui/src/theme/theme.css` — both the light block and the dark block,
  token names unchanged. The site's own CSS SHALL be written against those
  tokens and SHALL NOT hard-code a hex colour outside the token block.
- **The mark**, as the same inline SVG the console's masthead renders
  (`console/ui/src/components/Logo.tsx`), drawn with `var(--ao-brand)` and
  `var(--ao-surface)` so it follows the theme.
- **The shell**: a fixed masthead carrying the mark and the product word, over a
  left navigation sidebar and a content column — the console's arrangement, not
  a centred blog column.
- **The type**: Red Hat Display for headings, Red Hat Text for body, Red Hat
  Mono for code — the same families PatternFly gives the console.

The site SHALL NOT load PatternFly's component CSS or JavaScript. Identity is
carried by tokens, mark, shell and type; a prose site needs none of the
component library.

#### Scenario: An adopter arrives from the console

- **WHEN** an adopter who has used the console opens the documentation site
- **THEN** the brand teal, the accent violet, the mark and the masthead-over-
  sidebar arrangement are the ones the console showed
- **AND** no PatternFly stylesheet or bundle is requested

#### Scenario: The console palette changes

- **WHEN** a token value in `console/ui/src/theme/theme.css` is changed
- **THEN** the corresponding value in the site's token block is the only site
  edit required, because no other rule states a colour literally

### Requirement: Fonts are self-hosted with no third-party requests

The Red Hat variable fonts SHALL be vendored into the repository under
`docs/assets/fonts/` and declared with `@font-face` rules served from the site's
own origin. The site SHALL make NO request to any third-party host at
runtime — no font CDN, no analytics, no external stylesheet or script — and
SHALL render identically with no network access beyond its own origin.

The fonts' SIL OFL-1.1 licence text SHALL be committed alongside them, and
`font-display: swap` SHALL be set so text is readable before the font loads.

#### Scenario: A page is loaded with third-party hosts blocked

- **WHEN** a visitor loads the site with all origins except the site's own
  blocked
- **THEN** the page renders with the Red Hat families, the full palette and
  working navigation

#### Scenario: The font licence is checked

- **WHEN** the vendored font files are inspected
- **THEN** the OFL-1.1 licence text is present in the same directory

### Requirement: Theme choice is light, dark or system, and system follows the OS live

The site SHALL offer the console's three-state theme control — `light`, `dark`,
`system` — defaulting to `system`. The control SHALL store the CHOICE, not the
resolved theme, so that a visitor who asked to follow the OS keeps following it.
A `system` choice SHALL be a live subscription to
`prefers-color-scheme`: a reader whose OS flips to dark while the page is open
SHALL see the page follow without reloading.

The choice SHALL persist across page loads and be applied BEFORE first paint, so
no page flashes the wrong theme. The applied theme SHALL be exposed on the root
element in the same two ways the console exposes it — the PatternFly theme class
(`pf-v6-theme-light` / `pf-v6-theme-dark`) that the copied token block keys off,
and a `data-theme` attribute — and SHALL set `color-scheme` so browser chrome,
scrollbars and form controls match.

#### Scenario: First visit

- **WHEN** a visitor with no stored choice opens the site while their OS is in
  dark mode
- **THEN** the page renders in the dark palette
- **AND** the control shows `system` as the current choice

#### Scenario: The OS flips while the page is open

- **WHEN** a visitor whose choice is `system` has their OS switch to dark
- **THEN** the open page switches to the dark palette without a reload

#### Scenario: A stored choice is restored

- **WHEN** a visitor who chose `dark` navigates to another page on the site
- **THEN** the page paints dark on first paint, with no light flash
- **AND** the choice survives a browser restart

#### Scenario: The visitor returns to following the OS

- **WHEN** a visitor who chose `dark` selects `system` while the OS is light
- **THEN** the page becomes light, and later OS changes are followed again

### Requirement: Navigation is declared once, in one data file

Site navigation SHALL be declared in `docs/_data/nav.yml` — groups of entries,
each a title and a path — and rendered by the sidebar include. The entry for the
page currently being viewed SHALL be marked as current, both visually and for
assistive technology.

Adding a page to the site SHALL require editing exactly one navigation source.
A navigation entry pointing at a file that does not exist SHALL be treated as a
defect. In this change the file SHALL list only the landing page, since nothing
else is a site deliverable yet.

#### Scenario: The current page is shown

- **WHEN** any page in the navigation is open
- **THEN** its sidebar entry is visually distinguished and carries
  `aria-current="page"`

#### Scenario: A page is added to the site later

- **WHEN** a contributor publishes a further page on the site
- **THEN** one entry in `_data/nav.yml` is the only theme-side change needed
- **AND** no navigation markup is written a second time

### Requirement: The landing page is an adopter hub, not a second README

`docs/index.md` SHALL be the site's landing page: a short orientation for an
adopter — what the operator is, what "takes care of it" means, where it is
pluggable, and the paths onward grouped by what the reader is trying to do.

It SHALL lead with the product diagram exported from `docs/diagrams/`, and its
prose SHALL be that diagram's own copy rather than a second telling — the
diagram and the page must not disagree about what the product claims. Nothing
SHALL be said twice: the drawio source therefore carries a SITE page distinct
from the standalone poster, with the poster's own masthead removed, and the
figures the page states as tiles removed as well.

Words the page can say SHALL be said by the page, not by the drawing — the
eyebrow pills, the headline and the standfirst are page text, because text in
an exported image cannot be selected, translated, searched, or read except
through alt text.

The exported asset SHALL be self-contained (no third-party request) and SHALL
ship as one variant per theme, selected by the applied theme, with the unused
variant not fetched. Both SHALL be produced by a committed export script rather
than by remembered commands, since the dark variant requires a correction the
exporter cannot make itself. The diagram SHALL use the product's own palette.

Headline figures SHALL be presented as a row of stat tiles whose words come
from the page and whose markup comes from the layout. They are a KPI row, not
hero figures: values wear ink, identity is carried by a mark beside them, and
the number is never the only thing distinguishing one tile from another.

It SHALL NOT duplicate `README.md`'s CRD table, demo transcript, install
commands or status; those stay in the README, which the landing page links to.
Reference prose SHALL NOT be written into the landing page — it links to the
page that owns that content.

#### Scenario: The landing page and the diagram disagree

- **WHEN** the product pitch changes in `docs/diagrams/`
- **THEN** the landing page's prose is updated from the diagram's copy and the
  asset is re-exported, so the page never states a claim the diagram contradicts

#### Scenario: The diagram is viewed in the dark theme

- **WHEN** a reader with the dark theme opens the landing page
- **THEN** the dark variant is shown, legible down to its icons, on the page's
  own canvas
- **AND** the light variant is not fetched

#### Scenario: The diagram is re-exported

- **WHEN** the drawio source changes and the diagram is exported again
- **THEN** one command produces both variants with the dark one's icons already
  corrected
- **AND** no manual repair step stands between the export and the site

#### Scenario: An adopter opens the site root

- **WHEN** a first-time visitor opens the site root
- **THEN** they see what the project is and a grouped set of paths onward
- **AND** every path is a link to the document that owns that content

#### Scenario: Content would be duplicated

- **WHEN** a contributor is tempted to restate the install command or a CRD
  description on the landing page
- **THEN** the landing page links to the owning document instead

### Requirement: The site is readable and operable on any width

The layout SHALL be responsive: below a narrow-viewport threshold the sidebar
collapses behind a toggle and the content column takes the full width. Content
that cannot reflow — wide tables, code blocks, diagrams — SHALL scroll inside
its own container, and the page body SHALL NEVER scroll horizontally. Code
blocks SHALL be syntax-highlighted in a scheme built from the `--ao-*` tokens so
one scheme serves both themes.

A keyboard visitor SHALL be able to skip the masthead and sidebar and land on
the content, and SHALL be able to operate the theme control and the navigation.

#### Scenario: The site is read on a phone

- **WHEN** the landing page is opened at a phone viewport width
- **THEN** the sidebar is collapsed behind a toggle and the content is full width
- **AND** the page does not scroll horizontally, though a wide table may scroll
  inside its own frame

#### Scenario: A keyboard visitor arrives

- **WHEN** a visitor tabs into the page
- **THEN** the first stop is a skip link that moves focus to the content
- **AND** the theme control and every navigation entry are reachable and operable

### Requirement: The theme holds no prose and the pages hold no theme

Files under `docs/_layouts/`, `docs/_includes/`, `docs/_data/` and
`docs/assets/` SHALL contain presentation only — no product documentation, no
CRD descriptions, no install instructions. Published markdown pages SHALL
contain content only — no HTML shell, no inline styles, no script tags.

The site build's outputs (`docs/_site/`, `docs/.jekyll-cache/`) SHALL be
ignored by git.

#### Scenario: Product prose is added to the site

- **WHEN** a contributor documents a new behavior
- **THEN** it goes in a markdown page under `docs/`, never in a layout or include

#### Scenario: A build is run locally

- **WHEN** a contributor previews the site locally
- **THEN** the generated `_site/` and cache directories are not offered as
  changes to commit

