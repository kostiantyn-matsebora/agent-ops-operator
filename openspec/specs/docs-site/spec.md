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

The site's deliverables SHALL be the theme, `docs/index.md` and
`docs/introduction.md`. The existing `docs/*.md` reference pages SHALL NOT be
edited for the site — no YAML front matter added, no headings changed, no links
rewritten — SHALL NOT appear in the site navigation, and SHALL NOT be treated as
published documentation by any site page, all of which link to them where they
live until a later change takes them onto the site.

Because branch deploy builds the whole source directory, those pages ARE part of
the build. Carrying no YAML front matter, they are static files to Jekyll: they
SHALL be copied to the site verbatim, unconverted and unstyled, and SHALL remain
reachable by URL only. The theme SHALL provide the layout the build assigns a
page that gains front matter without naming one; that layout carries no feature
beyond a title and the page content, and publishing a further page SHALL NOT
require editing it.

#### Scenario: A reference page is built

- **WHEN** the site is built with the untreated `docs/*.md` pages present
- **THEN** the build succeeds and each page is copied verbatim, unconverted
- **AND** no reference page has been modified in the repository
- **AND** nothing on the site links to one as though it were a site page

#### Scenario: A reader looks for the reference documentation

- **WHEN** a visitor on a site page follows a path to the CRD reference
- **THEN** the link goes to the page where it lives today
- **AND** no sidebar entry claims it as a site page

#### Scenario: A contributor is tempted to fix a page for the site

- **WHEN** a reference page would need front matter, a heading change or a
  rewritten link to look right on the site
- **THEN** the page is left alone and the work is deferred to the change that
  publishes the reference pages

#### Scenario: A site page is published

- **WHEN** a markdown file under `docs/` gains front matter and one navigation
  entry
- **THEN** it is built as a site page with the assigned layout, is reachable at
  its declared URL, and no layout, include or stylesheet was edited to publish it

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

### Requirement: The Introduction orients an adopter to the model

The site SHALL carry an Introduction page for an adopter who has read the
landing page and has not yet decided. It SHALL be reachable from the navigation
under the same group as the landing page, and SHALL declare its own URL rather
than relying on a permalink style the site does not configure.

The page SHALL carry a short opening and then exactly two sections:
**understand the concepts**, and **follow the guides**. It SHALL NOT grow further
sections; material that does not belong to one of those two belongs in a guide or
a reference page.

- The opening SHALL state what the operator is and the structural idea that
  separates it from a bot or a runbook — identity, wiring and execution are
  separate objects, so what an agent may do comes from the routing that wakes it
  and never from the agent itself.
- The concepts section SHALL present the model's parts **as cards**, one per
  part. Each card SHALL be titled with the part's NAME, SHALL carry that kind's
  glyph as the console draws it, SHALL describe it in one or two sentences, and
  SHALL link to the reference that owns its detail, with the whole card as the
  target.
- The guides section SHALL list the task-shaped walkthroughs when there are any,
  and SHALL say plainly that there are none when there are none. It SHALL NOT
  list guides that do not exist as though they were available.

#### Scenario: An adopter reads the Introduction before the reference

- **WHEN** an adopter who has read only the landing page opens the Introduction
- **THEN** they can name what wakes an agent, what decides what it may touch and
  what executes it, and can see which guide to read next

#### Scenario: A reader arrives from the console

- **WHEN** a reader who has used the console's configuration page reads the
  concept cards
- **THEN** each kind carries the glyph the console draws for it, in the theme's
  own colour

#### Scenario: An odd number of concepts

- **WHEN** the card count does not fill the last row
- **THEN** the final card keeps its normal width rather than stretching across
  the row, so it reads as one more card and not as a summary of the others

#### Scenario: A concept card is followed

- **WHEN** a reader clicks anywhere on a concept's card
- **THEN** they reach the reference section for that part, and the card carries a
  visible affordance that it leads somewhere

#### Scenario: No guides exist yet

- **WHEN** the guides section has nothing to list
- **THEN** it says so, and lists no unwritten guide as though it could be read

#### Scenario: The page is kept to its two sections

- **WHEN** a contributor has material that fits neither concepts nor guides
- **THEN** it goes in a guide or the reference page that owns it, and the
  Introduction keeps its two sections

#### Scenario: The page is current in the navigation

- **WHEN** the Introduction is open
- **THEN** its sidebar entry is marked current, both visually and with
  `aria-current="page"`

#### Scenario: The heading is not duplicated

- **WHEN** the Introduction is rendered
- **THEN** exactly one `h1` is present on the page

### Requirement: The Introduction states no reference detail

The Introduction SHALL NOT restate content another document owns. It SHALL name
no CRD field, no HTTP endpoint, no Helm values key, no install command and no
YAML example; where such detail is needed it SHALL link to the document that owns
it.

The governing test SHALL be whether a sentence would have to change if a CRD
field were renamed: a sentence that would belongs in the reference page, not
here.

#### Scenario: A field is renamed

- **WHEN** a CRD field is renamed
- **THEN** the Introduction needs no edit, because it named no field

#### Scenario: A contributor adds reference prose to the Introduction

- **WHEN** a contributor is tempted to document a field, endpoint or values key
  on the Introduction
- **THEN** it goes in the document that owns that content and the Introduction
  links to it

### Requirement: Presentation beyond prose is a named component, not page markup

A page needing a form the prose elements do not provide SHALL name a component
class the theme styles, using the markdown engine's attribute syntax. It SHALL
NOT contain HTML markup, inline styles or script, and the component's rule SHALL
carry no words.

A component SHALL be general — usable by any page that needs that form — and
SHALL NOT be styling introduced for one page. Content SHALL NOT move into
`_includes/` or `_data/` to obtain a visual form; both are theme scope and hold
no product prose.

The site SHALL provide a **card grid** for a set of peer concepts and a
**callout** for a statement a page is built around. The callout SHALL be visually
distinct from the plain blockquote, which sets a passage ASIDE: the two carry
opposite emphasis and SHALL NOT render alike.

A component no page uses SHALL be removed rather than kept for a future one.

Where a component draws a kind's glyph it SHALL use the one the console draws
for that kind (`console/ui/src/graph/shapes.tsx`), copied on the same terms as
the palette and the mark — one-directional, so that changing a glyph is a
two-file change — and rendered so it follows the theme's colour rather than
carrying a baked-in one.

#### Scenario: A glyph must follow the theme

- **WHEN** a kind's glyph is rendered in either theme
- **THEN** it takes the theme's own colour, and no colour is baked into the
  copied artwork

### Requirement: A page carries an on-this-page column, built in the theme's own assets

EVERY site page SHALL carry a navigation column listing its own headings and
marking the section being read. It SHALL be a default rather than an opt-in, so
publishing a page never depends on remembering a flag — a forgotten one is
invisible, since the page simply lacks the column.

The three-column shell is therefore the SITE's layout, not one page's.

It SHALL be built from the rendered headings by the theme's own JavaScript. No
Jekyll plugin outside the GitHub Pages set SHALL be added for it, and the build
SHALL NOT become a workflow.

It SHALL degrade rather than break: a page with too few headings to be worth
listing, and a visitor without JavaScript, SHALL see no column rather than an
empty one. Below a narrow-viewport threshold the column SHALL be dropped
entirely rather than squeezed — it is a convenience, where the left navigation is
the way between pages — and the page body SHALL NOT scroll horizontally at any
width.

#### Scenario: A reader scrolls a page

- **WHEN** a reader scrolls past a heading
- **THEN** that heading's entry becomes the marked one, both visually and for
  assistive technology

#### Scenario: The last section is reached

- **WHEN** the reader reaches the bottom of the page
- **THEN** the last section's entry is marked, even if its heading never reached
  the top of the viewport

#### Scenario: The page is too short to need one

- **WHEN** a page has fewer than two listable headings
- **THEN** no column is shown, and no empty rail occupies the width

#### Scenario: The viewport is narrow

- **WHEN** the viewport is too narrow for three columns
- **THEN** the on-this-page column is not shown, the content takes the freed
  width, and nothing scrolls horizontally

#### Scenario: Prose is held to a readable line length

- **WHEN** a page of prose is read on a wide display
- **THEN** lines run to roughly 75–80 characters, and spare width goes to the
  gutter beside the navigation rather than to longer lines

#### Scenario: The content column is the widest track

- **WHEN** the site is viewed at any width that renders three columns
- **THEN** the content column is wider than the navigation, the gutter and the
  on-this-page rail — the rail and the navigation are fixed strips and the
  gutter's maximum is below the content column's width, so the centre cannot be
  out-grown by what surrounds it

#### Scenario: A very wide display

- **WHEN** the viewport grows past what the layout needs
- **THEN** the spare width goes to the gutter until it reaches its maximum and
  then falls off the right of the rail — never into the rail itself, which is a
  fixed strip

#### Scenario: Spare width does not open a gap before the rail

- **WHEN** the viewport is wider than the content needs
- **THEN** the on-this-page rail still begins one gutter after the text ends —
  spare width is never left between the last word and the rail, which reads as
  an inflated right column rather than as a generous one

#### Scenario: The card count does not follow the window

- **WHEN** the same card section is viewed at 1280 and at 1920
- **THEN** it has the same number of columns at both

#### Scenario: Content sized against the viewport meets a new column

- **WHEN** an element is sized by subtracting known columns from the viewport
  width and a further column is added to the shell
- **THEN** it is re-sized against the column that contains it rather than against
  the viewport, so it can never be drawn underneath a sibling column

#### Scenario: A page is published with no thought given to the column

- **WHEN** a contributor adds a page with front matter and a navigation entry,
  naming nothing about the column
- **THEN** it carries one, built from its own headings

#### Scenario: A page needs a form the prose elements lack

- **WHEN** a set of peer concepts reads as an undifferentiated list
- **THEN** the page names the card component with an attribute list and the
  theme supplies the rule
- **AND** the markdown contains no `div`, inline style or script

#### Scenario: Content is tempted into the theme

- **WHEN** a contributor considers moving concept text into an include or a data
  file to render it as cards
- **THEN** the text stays in the markdown page and only the class is shared

#### Scenario: A load-bearing statement is set aside instead of emphasised

- **WHEN** a page renders a claim it is built around as a plain blockquote
- **THEN** it reads as de-emphasised, and the callout component is used instead

#### Scenario: A component is added for a single page

- **WHEN** a proposed rule could serve only the page requesting it
- **THEN** it is generalised into a component any page can name, or the page is
  rewritten in what already exists

