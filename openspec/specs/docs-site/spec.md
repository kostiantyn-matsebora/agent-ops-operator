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

The site's deliverables SHALL be the theme, `docs/index.md`,
`docs/introduction.md`, `docs/getting-started.md`, `docs/installation.md` and
`docs/console-guide.md`. The existing `docs/*.md` reference pages SHALL NOT be
edited for the site — no YAML front matter added, no headings changed, no links
rewritten — SHALL NOT appear in the site navigation, and SHALL NOT be treated as
published documentation by any site page, all of which link to them where they
live until a later change takes them onto the site.

A site page SHALL be free to declare a permalink that a reference page's own
filename resembles. `docs/console-guide.md` at `/console/` and the untreated
`docs/console.md` served verbatim at its own path are two different documents at
two different URLs, and neither SHALL be renamed or edited to accommodate the
other.

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

#### Scenario: A site page and a reference page name the same subject

- **WHEN** the site publishes a page about the console and an untreated
  reference page about the console is present in the same directory
- **THEN** both build, each is reachable at its own URL, and neither was renamed
  or edited to free a filename

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
  `platform/console/ui/src/theme/theme.css` — both the light block and the dark block,
  token names unchanged. The site's own CSS SHALL be written against those
  tokens and SHALL NOT hard-code a hex colour outside the token block.
- **The mark**, as the same inline SVG the console's masthead renders
  (`platform/console/ui/src/components/Logo.tsx`), drawn with `var(--ao-brand)` and
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

- **WHEN** a token value in `platform/console/ui/src/theme/theme.css` is changed
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

### Requirement: The landing page opens with what it is, then how it works

`docs/index.md` SHALL be the site's landing page: a short orientation for an
adopter — what the operator is, where it earns its keep, and the paths onward
grouped by what the reader is trying to do.

Its opening SHALL be, in this order: the **project's name**, **one sentence**
saying what it is, the **claim chips**, and then a **tabbed panel set**. Nothing
SHALL stand between the name and the panel set except those two lines and the
chips.

**The one sentence is the whole of the standfirst.** A second explanatory
paragraph SHALL NOT be added beneath it: the panel set immediately below states
the model in full, and a page that explains itself twice before showing anything
has buried what it is showing.

The panel set's panels SHALL be, in order:

1. the **presentation**, which states the model one beat at a time;
2. the **recording of the product working**, carrying one piece of work from the
   event that starts it to the answer a person replies to;
3. a real `Pipeline` manifest, written in the page as a fenced code block.

The presentation SHALL come first because it answers the question a first-time
reader actually has — what is this and how does it fit together — and the
recording answers the next one.

**No exported drawing SHALL be shown in the opening.** The presentation carries
the model, and a still restating it would be the same claim in two forms, one of
which cannot be selected, translated or searched.

**The landing page SHALL NOT tour the console's views.** They belong to the
Console page, which takes each in turn with the question it answers, and
publishing them in both places is one tour at two altitudes. The landing page
SHALL instead link the Console page for them.

The manifest panel SHALL be page text rather than an exported image, so it can be
selected, copied and searched, and every field name on it SHALL exist on the
`Pipeline` CRD.

Words the page can say SHALL be said by the page. The name, the sentence and the
chips are page text, because text in an exported image cannot be selected,
translated, searched, or read except through alt text.

**What the install reaches SHALL be named once, as a single labelled group**
below the panel set, rather than as several labelled rows above it. A reader
decides whether this fits their stack after they know what it is, not before.
Anything named there SHALL ship in the release being described.

The page SHALL carry a section headed by a **question the reader is asking** —
where this earns its keep — introduced by one sentence and answered by a
**two-column table** naming each area of use and what happens there. Each row MAY
carry the mark of the system it names.

**A table SHALL be used rather than a grid of tiles.** The rows are read against
each other, and a tile grid spends most of its area on ornament for six lines of
text.

**The viewer SHALL be given its own full-width strip** beneath that table rather
than a row within it. It is not one more area of use — it is where every area is
watched and answered — and a row would state that it is a peer.

**Headline figures SHALL NOT be presented.** A count of resource kinds, contracts
or bundles answers a question a first-time reader has not yet asked, and it
occupies the position where the reader is deciding whether the product is for
them at all. Such counts belong on the reference pages that own them.

The layout SHALL place the page's sections **in the order the page writes them**
and SHALL NOT split the page's content to insert anything of its own.

It SHALL NOT duplicate `README.md`'s CRD table, demo transcript, install
commands or status; those stay in the README, which the landing page links to.
Reference prose SHALL NOT be written into the landing page — it links to the
page that owns that content.

#### Scenario: An adopter opens the site root

- **WHEN** a first-time visitor opens the site root
- **THEN** they see the project's name, one sentence saying what it is, and the
  presentation, before anything else

#### Scenario: A reader asks whether it is for them

- **WHEN** the landing page is read past its opening
- **THEN** a question-headed section answers where the product earns its keep, as
  a table of areas of use rather than a grid of figures

#### Scenario: A reader asks whether it fits their stack

- **WHEN** the landing page is read
- **THEN** what the install reaches is named once, below the panel set, and not
  before the page has said what the product is

#### Scenario: An integration does not ship yet

- **WHEN** an integration is named in the `works with` group
- **THEN** the release being described ships it — the group answers what the
  product DOES, so there is no honest place for a "coming soon" entry
- **AND** an integration whose bundle slips out of the release is removed from
  the group in the same change that slips it

#### Scenario: The page adds a section

- **WHEN** a section is added to the landing page
- **THEN** it renders where the page wrote it, and the layout inserts nothing of
  its own between the page's sections

#### Scenario: A reader wants to see the product

- **WHEN** a visitor who has installed nothing opens the landing page
- **THEN** the presentation states the model without them installing anything,
  and the recording is one tab away
- **AND** the console's own views are one link away, on the page that owns them

#### Scenario: The manifest is copied from the page

- **WHEN** a reader selects the `Pipeline` panel and copies its contents
- **THEN** they get text, not an image, and every field on it exists on the
  `Pipeline` CRD

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

The site SHALL provide a **card grid** for a set of peer concepts, a **callout**
for a statement a page is built around, a **tabbed panel set** for a series of
alternative views of one subject, and a **player** for a recording. The callout
SHALL be visually distinct from the plain blockquote, which sets a passage ASIDE:
the two carry opposite emphasis and SHALL NOT render alike.

The tabbed panel set SHALL be named on an ordinary markdown list, one list item
per panel, whose leading emphasised phrase is the tab's label and whose remaining
content is the panel. **Every word and every image SHALL live in the page.** The
theme SHALL supply only the tab strip, the panel geometry and the selection
behaviour.

**The player SHALL be named on an ordinary markdown link** whose target is the
recording and whose content is the poster image with its alt text. The page
therefore names the two files and writes the words, and the theme supplies the
element, its controls and its frame. A page SHALL NOT write a video element,
a source, a control or a poster attribute.

**With scripting unavailable every component SHALL degrade to its own content** —
the tabbed panel set to the labelled list, in order, with every panel visible,
and the player to the poster image linking to the recording. A page SHALL NOT
depend on a component's behaviour to make its content complete or comprehensible.

A component no page uses SHALL be removed rather than kept for a future one.

Where a component draws a kind's glyph it SHALL use the one the console draws
for that kind (`platform/console/ui/src/graph/shapes.tsx`), copied on the same terms as
the palette and the mark — one-directional, so that changing a glyph is a
two-file change — and rendered so it follows the theme's colour rather than
carrying a baked-in one.

#### Scenario: A glyph must follow the theme

- **WHEN** a kind's glyph is rendered in either theme
- **THEN** it takes the theme's own colour, and no colour is baked into the
  copied artwork

#### Scenario: A page presents alternative views of one subject

- **WHEN** a page names the tabbed panel set on a markdown list
- **THEN** one tab is shown per list item, labelled with that item's leading
  emphasised phrase, and selecting a tab shows that item's content

#### Scenario: A page publishes a recording

- **WHEN** a page names the player on a link wrapping a poster image
- **THEN** the theme renders a player with controls, the poster on its face, and
  the recording fetched only when the reader starts it

#### Scenario: Scripting is unavailable

- **WHEN** a page carrying a tabbed panel set is read with scripting disabled
- **THEN** every panel is visible in document order with its label, and no
  content is hidden, truncated or replaced by a placeholder

#### Scenario: A tab is linked directly

- **WHEN** a reader arrives at a page with a fragment naming one of its tabs
- **THEN** that tab is the selected one on arrival

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

### Requirement: Getting started takes a reader from an empty cluster to a route of their own

The site SHALL carry a Getting started page for a reader who has decided to try
the operator. It SHALL be reachable from the navigation in the same group as the
landing page and the Introduction, and SHALL declare its own URL rather than
relying on a permalink style the site does not configure.

The page SHALL be followable top to bottom in one sitting and SHALL carry, in
order:

- **prerequisites** — what a cluster must already have before the first command,
  including the storage decision that selects an install flag, stated up front
  rather than encountered as a failure;
- **the install** — copy-pasteable, with what each command is for;
- **the first answer** — how to ask, and **what a successful run looks like**:
  the states the conversation passes through, the pod that appears and exits,
  where the transcript is read, and where the result is read;
- **what goes wrong first** — the common failures and the observable each one
  announces itself through;
- **one route the reader writes**, applied against what the install already
  created, demonstrating that a route's reach comes from the route and not from
  the agent, and ending with the teardown of what that step created;
- **where to go next**, naming the document that owns each onward topic.

The page SHALL carry the commands and the manifest the reader types, and SHALL
NOT carry reference material: no CRD field table, no HTTP endpoint table, no
values reference, and no explanation of options the walkthrough does not use.
Where such detail is needed it SHALL be a link to the document that owns it.

Every command, flag and manifest on the page SHALL be verified against the chart
as it ships. A claim that cannot be verified SHALL be removed from the page
rather than restated from another document.

#### Scenario: A reader follows the page on a fresh cluster

- **WHEN** a reader with a cluster and an LLM credential follows the page from
  the top
- **THEN** they reach an installed operator, an answered question, and one
  Pipeline they wrote themselves, without needing another document to complete a
  step

#### Scenario: A run produces nothing visible

- **WHEN** the reader asks a question and no answer appears
- **THEN** the page names what to look at, in what order, and what the common
  causes look like when observed there

#### Scenario: The reader learns where capability comes from

- **WHEN** the reader applies the route the page has them write and asks again
- **THEN** the outcome differs from the demo route's in the way the page
  predicted, and the page attributes the difference to the wiring rather than to
  the agent

#### Scenario: The exercise leaves nothing running

- **WHEN** the reader finishes the route exercise
- **THEN** the page has them remove what that step created and states the cost of
  leaving it in place

#### Scenario: A contributor wants to explain a field on the page

- **WHEN** a step would need a CRD field's other values, an endpoint's semantics
  or a values key's rules to be explained
- **THEN** the page links the document that owns it and the explanation is not
  written a second time

#### Scenario: The page is current in the navigation

- **WHEN** Getting started is open
- **THEN** its sidebar entry is marked current, both visually and with
  `aria-current="page"`

#### Scenario: A reader arrives at the landing page ready to install

- **WHEN** a visitor follows the landing page's paths onward looking for how to
  start
- **THEN** Getting started is named there as the walkthrough, and the path to
  installation does not leave the site for the README

### Requirement: The Installation page gets an operator to a real install

The site SHALL carry an Installation page for an operator deploying agent-ops
for real, as distinct from the demo the Getting started page installs. The two
SHALL each say which they are in their opening lines, and SHALL link to each
other.

The page SHALL cover, in this order:

- **the decisions that are expensive to reverse**, before the install rather
  than inside it — storage and whether conversations keep their context, the
  RBAC posture the agent inherits, and who owns the CRDs;
- **the install** — namespace, credential, the command, and how to tell the
  release is healthy;
- **enabling a bundle** — what a bundle is, the flag each takes, and a link to
  the page that owns its values;
- **the parent chart's values, grouped by the decision they serve**;
- **wiring one route**, because an install with no Pipeline claiming a source
  drops every signal and answers nothing;
- **upgrade and uninstall** — where migration steps live, and what survives.

#### Scenario: An operator installs for real

- **WHEN** an operator who has run the demo follows the Installation page
- **THEN** they finish with a release installed on their own terms, one bundle
  enabled, and one route wired

#### Scenario: The expensive decisions are met before they are paid for

- **WHEN** an operator reaches the install command
- **THEN** storage, RBAC posture and CRD ownership have already been stated as
  decisions, not discovered later as symptoms

#### Scenario: A fresh install answers nothing

- **WHEN** an operator installs the chart and enables a bundle but wires no route
- **THEN** the page has already told them signals are dropped until a Ready
  Pipeline claims the source, and shown the smallest route that fixes it

### Requirement: Every command is given for both platforms, as tabs

A command block on a site page SHALL be given in a **PowerShell** and a
**Linux/macOS** form, so a reader on Windows never has to translate one. Where
the two are identical, both SHALL still be shown.

The page SHALL carry no tab markup. It writes two adjacent fenced blocks whose
LANGUAGES name the shell, and the theme pairs them into one tabbed widget — the
same division by which a page names a card class and the theme draws the card.

Choosing a platform SHALL apply to every block on the page at once and SHALL
persist across pages, since a reader's platform does not change between them.
The initial choice SHALL be taken from the browser.

Without JavaScript both blocks SHALL simply render in sequence, each labelled by
its own syntax highlighting. The commands are still present, which is the part
that matters.

#### Scenario: A reader on Windows

- **WHEN** a reader whose browser reports Windows opens a page with commands
- **THEN** the Windows form is shown first, and choosing it once holds for every
  block on that page and on the next page they visit

#### Scenario: The forms differ

- **WHEN** a command uses a line continuation or shell-specific quoting
- **THEN** each tab carries the form that actually runs on that platform, not a
  copy of the other

#### Scenario: JavaScript is unavailable

- **WHEN** the page is read with scripting disabled
- **THEN** both command blocks are visible in sequence and neither is hidden

### Requirement: The Installation page owns the parent chart's values, and only those

The parent chart's values SHALL have exactly one home, and it SHALL be this
page. Values belonging to a subchart SHALL stay with that subchart's page, and
the console's with the console's page. Neither SHALL restate the other.

The page SHALL group values by the DECISION they serve rather than enumerate
them, and SHALL name the exhaustive list as a command rather than reproducing
it. A hand-copied table of every value SHALL NOT be maintained.

#### Scenario: An operator needs a value the page does not name

- **WHEN** a value is not among those the page groups
- **THEN** the page names the command that lists every value, and the document
  that owns that value's semantics

#### Scenario: A bundle's values are wanted

- **WHEN** an operator wants to configure a bundle beyond enabling it
- **THEN** the Installation page links to that bundle's page rather than
  documenting its values

#### Scenario: A value is added to the chart

- **WHEN** a new value is added to the parent chart
- **THEN** the page needs an edit only if it changes a decision an operator
  makes, because the page carries decisions rather than an inventory

### Requirement: The Console page shows the console before it is installed

The site SHALL carry a Console page for a reader who has just met the console in
Getting started, or who has not installed anything at all. It SHALL be reachable
from the navigation in the same group as the landing page, and SHALL declare its
own URL.

The page SHALL cover, in this order:

- **what the console is** — a view of the whole install that is also a channel
  and also a signal source, stated in a few sentences;
- **a tour of its views**, as a tabbed panel set with one panel per view, each
  carrying a label, a one-line statement of the question that view answers, and
  a screenshot of it;
- **what it does for the reader**, as a set of peer capabilities;
- **authentication**, as its own section (covered by its own requirement);
- **what it cannot do** — that no write path to the Kubernetes API exists in the
  module.

The page SHALL carry decisions and their consequences. It SHALL NOT carry the
console's HTTP endpoints, its RBAC grant, or its values reference. Where such
detail is needed it SHALL be a link to the document that owns it.

#### Scenario: A reader who has installed nothing

- **WHEN** a visitor who has not installed the operator opens the Console page
- **THEN** they can see each view and read what question it answers, without
  running anything

#### Scenario: A reader arrives from Getting started

- **WHEN** a reader finishes Getting started and follows its what-next card
- **THEN** they arrive at the Console page, and it explains the screen the
  walkthrough left them looking at

#### Scenario: A contributor wants to document an endpoint on the page

- **WHEN** a section would need the console's endpoints, RBAC grant or full
  values list
- **THEN** the page links the document that owns it and the detail is not
  written a second time

#### Scenario: The page is current in the navigation

- **WHEN** the Console page is open
- **THEN** its sidebar entry is marked current, both visually and with
  `aria-current="page"`

### Requirement: The Console page states the console's authentication as a decision

The Console page SHALL carry authentication as its own section, written for an
operator deciding how to expose the console rather than for someone reading the
implementation. It SHALL state:

- **the shipped mode** — a shared token, where it comes from, that an
  unconfigured token authorizes nobody, and that it is indistinguishable from a
  wrong one;
- **the alternative** — that a deployment may declare an external authenticator
  in front, and what that declaration requires;
- **the interface to that authenticator** — the identity headers the console
  trusts, **enumerated completely and in preference order**, with the statement
  that an install must ensure the fronting proxy sets every one of them and lets
  no client-supplied copy through;
- **the consequence of forwarding no identity** — reads served, writes refused,
  and no identity invented.

The header list SHALL be verified against the console's own source rather than
copied from another document. A partial list SHALL NOT be published: the page's
purpose is to tell an operator what to allow-list, and a header omitted there is
a header left reaching the console from the client.

#### Scenario: An operator decides how to expose the console

- **WHEN** an operator reads the page before putting the console behind an
  ingress
- **THEN** both modes are stated with their consequences, and the values each
  requires are named

#### Scenario: An operator configures their own authenticator

- **WHEN** an operator puts a forward-auth proxy in front of the console
- **THEN** the page names every identity header the console trusts, in the order
  it prefers them, and states that all of them must be set by the proxy and none
  accepted from a client

#### Scenario: The console's trusted headers change

- **WHEN** the set of identity headers the console reads is changed in the
  source
- **THEN** the page is wrong until it is updated, and the change is not complete
  until it is

### Requirement: The site carries a guide per adoption tier, in learning order

The site SHALL publish how-to guides covering four tiers of adoption: writing an
agent and its wiring; giving that agent capabilities; implementing a signal or
channel adapter; and implementing an agent runtime.

The tiers SHALL be ordered by what a reader must UNDERSTAND, not by what they
can break. Risk is not monotonic along that order — a capability binding is pure
YAML and can grant more than an adapter's code ever could — so **each guide
SHALL state what its own mistake costs**, and no guide may imply that danger
grows with the tier.

**The FIRST guide SHALL be the wiring, and it SHALL create nothing new.** A
`Pipeline` names only objects a working install already has, so the fundamental
lesson costs the reader no new resources. A guide that opens by declaring an
identity teaches an inert object whose purpose is a Pipeline the reader has not
met.

Every guide SHALL have the same five parts, in order: what the thing IS, when it
applies and when it does not, the shape it is built from, sections NAMED FOR THE
TASK with their code beneath them, and where to go next.

A guide SHALL NOT restate reference material. The CRD reference and the contract
documents are linked, never reproduced.

A guide SHALL NOT consist of instructions alone. Explanation belongs immediately
before the code it explains — a page that cuts it entirely gives instructions
with no subject.

**A guide's TITLE SHALL name what the reader gets**, not the custom resource it
is built from. The kind is named in the page's opening sentence and in its
description, so the vocabulary stays findable without a title reading as
implementation.

#### Scenario: A guide is titled for its CRD

- **WHEN** a title names the kind rather than the outcome
- **THEN** it is wrong, and the outcome is what the title states

#### Scenario: A reader finishes Getting started

- **WHEN** they look for what to do next
- **THEN** the Introduction's guides section names the guides, and the first one
  builds a working route out of what the demo install already contains

#### Scenario: A reader wants a second route over installed pieces

- **WHEN** they have a profile, a source and a channel but no wiring between them
- **THEN** the first guide is sufficient on its own, and asks them to create no
  resource other than the Pipeline

#### Scenario: A guide would open with a numbered step

- **WHEN** a guide begins instructing before it has said what the thing is
- **THEN** it is wrong, and the opening states the subject first

#### Scenario: A guide would explain a CRD field in full

- **WHEN** a guide needs field semantics beyond what its task uses
- **THEN** it links the reference and does not reproduce it

#### Scenario: A tier looks safe because it is early

- **WHEN** a guide covers a tier that is early in the order but wide in effect
- **THEN** it states that effect as plainly as a later guide states its own

### Requirement: Custom resource templates and examples are generated, not written

Every custom resource template on the site SHALL be generated from the CRDs the
chart ships. Every worked example SHALL be rendered from the chart's own values
for the bundle that owns that lane.

Neither SHALL be hand-written or invented. An invented example is a second set
of values to keep true, and it is how a real identifier gets pasted in as the
better-looking example.

Because the site is built by a branch deploy with no build step, generated files
SHALL be committed — and CI SHALL regenerate them and FAIL on any difference.
Committing generated output without that check produces a file that is correct
the day it is written and silently wrong after the next field rename.

A guide SHALL carry the MINIMAL resource — the required fields plus what that
guide teaches — and link the full generated reference for the rest.

#### Scenario: A CRD field is renamed

- **WHEN** a field is added, renamed or removed in the CRDs
- **THEN** CI fails until the generated templates are regenerated

#### Scenario: A chart value changes

- **WHEN** a bundle's values change what it renders
- **THEN** CI fails until the generated examples are regenerated

#### Scenario: A guide covers a large kind

- **WHEN** a kind has more fields than a reader can usefully scan
- **THEN** the guide shows the minimal resource and links the full reference,
  rather than reproducing every field inline

#### Scenario: An example needs a value the chart does not carry

- **WHEN** a worked example would require an invented identifier
- **THEN** the chart's own placeholder is used, so the example and the shipped
  values cannot disagree

### Requirement: Product screenshots on the site are generated, not captured by hand

A screenshot published on the site SHALL be produced by a committed, repeatable
command rather than captured by hand. The command SHALL drive the application's
own built bundle against a fixture it also owns, and SHALL write the image files
the site references.

Screenshots SHALL ship one variant per theme, and the site SHALL show the variant
matching the reader's resolved theme.

A screenshot fixture SHALL contain no data from a real installation — no cluster
names, namespaces, hostnames or identities taken from one.

The generating command SHALL NOT run as part of the ordinary test suite, and the
produced images SHALL be committed, so that building the site never depends on
running it.

#### Scenario: The UI changes and the screenshots are stale

- **WHEN** the console's UI changes such that a published screenshot no longer
  shows it
- **THEN** re-running the committed command reproduces every screenshot, and no
  image is re-captured by hand

#### Scenario: A reader switches theme

- **WHEN** a reader changes the site's theme while a screenshot is on screen
- **THEN** the screenshot shown changes to the variant matching the new theme

#### Scenario: The site is built

- **WHEN** the site is built by GitHub Pages branch deploy
- **THEN** every referenced screenshot is present in the repository and no
  generation step runs

#### Scenario: A fixture is written

- **WHEN** the fixture behind the screenshots is authored or edited
- **THEN** it contains only invented names, and nothing identifying a real
  installation appears in any published image

### Requirement: A labelled set of short names is a chip set, named by the page

The site SHALL provide a **chip set** for a labelled group of short names — the
form a reader scans rather than reads, such as what an install ships or what a
bundle contains.

It SHALL be named on an ordinary markdown list of GROUPS: each item's leading
emphasised phrase is that group's label, and its nested list becomes that group's
chips. Every word SHALL live in the page. The theme SHALL supply only the label's
treatment, the chip's shape and the row's wrapping.

**With no stylesheet it SHALL remain its own content** — a labelled list of
names, in order, nothing hidden. The row SHALL wrap rather than scroll, and the
page body SHALL NEVER scroll sideways because of it.

No group SHALL be styled differently from another: the labels carry the meaning,
and colouring one row would state something about it the label does not.

A chip MAY carry the mark of the thing it names. The mark SHALL be a file the
PAGE names — never one the stylesheet knows about, since a vendor list in the
theme is product knowledge in the theme. Its alt text SHALL be empty, because the
name it belongs to is already beside it.

#### Scenario: A page names a chip set

- **WHEN** a page names the chip set on a list of groups
- **THEN** each group renders as its label followed by its names as chips

#### Scenario: The set is read on a phone

- **WHEN** a chip set wider than the viewport is rendered
- **THEN** it wraps onto further lines and the page body does not scroll sideways

#### Scenario: A chip carries a mark

- **WHEN** a chip names an integration that has its own mark
- **THEN** the page names the mark's file and the stylesheet says only how big
  it is

#### Scenario: A reader asks what this plugs into

- **WHEN** the landing page's `works with` group is read
- **THEN** it names what the install can reach, in one row of marks, and the
  reader is not asked to read three labelled rows before the page has said what
  the product is

### Requirement: A themed image is named once by the page and resolved by the theme

A page publishing an asset that has a per-theme variant SHALL name **one** file.
The theme SHALL resolve which variant is shown from the reader's resolved theme,
and SHALL re-resolve it when the theme is changed while the asset is on screen. A
page SHALL NOT name both variants, and SHALL NOT carry any class, markup or
wording that states which theme it is for — that is theme knowledge in content.

This SHALL hold for every themed asset alike — screenshot, diagram, recording and
the poster frame that stands in for one — so a page publishing any of them learns
no new rule.

Resolution happens in a DEFERRED script, so the variant a page names is on the
page before the swap — briefly for a dark-theme reader, and permanently for one
without scripting. **A published image SHALL therefore carry its own ground**,
never a transparent one that borrows the page's canvas: a transparent light
export on an already-dark page is not a mismatched colour but invisible ink.

#### Scenario: A reader switches theme

- **WHEN** a reader changes the site's theme while a themed image is on screen
- **THEN** the image shown changes to the variant matching the new theme

#### Scenario: A page publishes a diagram

- **WHEN** a page names a diagram that ships light and dark exports
- **THEN** it names one file, and the theme shows the reader's variant, exactly
  as it does for a screenshot

#### Scenario: A reader switches theme with a recording on screen

- **WHEN** a reader changes the site's theme while a recording and its poster are
  on screen
- **THEN** both resolve to the variant matching the new theme, and the page named
  neither of them

#### Scenario: The dark theme is applied before the swap runs

- **WHEN** a dark-theme reader loads a page whose first panel names a light image
- **THEN** that image is legible on its own ground until the script swaps it,
  rather than dark ink lost on the dark canvas

### Requirement: Third-party marks are committed unaltered and used referentially

A mark belonging to another project SHALL be committed from that project's own
canonical source, unaltered, under `docs/assets/img/logos/`, beside a note
recording for each one its source URL and its licence or trademark terms.

Marks SHALL NOT be recoloured, restyled or redrawn to suit this site's palette —
an altered mark is no longer that project's mark. A mark that does not read on a
ground SHALL be dropped rather than repainted.

They SHALL be used only to NAME an integration the product speaks to, never in a
way that suggests the other project endorses this one.

#### Scenario: A mark does not suit the palette

- **WHEN** a project's mark clashes with the site's colours
- **THEN** it is used as published or not at all

#### Scenario: A contributor adds an integration

- **WHEN** a mark is added for a new integration
- **THEN** its source URL and terms are recorded in the same directory in the
  same change

#### Scenario: A project refreshes its mark

- **WHEN** an upstream project publishes a new mark
- **THEN** the recorded URL is enough to re-fetch it, and no build step
  generates it

### Requirement: The landing page demonstrates the product with a generated recording

The landing page SHALL carry a **recording of the product working**, showing one
piece of work from the event that starts it to the answer a person replies to,
and then the machinery that carried it. It SHALL be the strip's SECOND panel,
after the presentation: the presentation states the model, the recording shows
it happening, and a reader who wants the model first is not made to infer it
from footage.

**The machinery is part of the claim, not an appendix**: what is waiting and what
is stuck, the wiring that routed the signal, and that every part of it is an
ordinary Kubernetes resource.

**It SHALL also show a person STARTING a conversation and choosing which
pipeline answers.** Everything else in the story is signal-driven, and without
this beat the recording never shows that a person can address a particular agent
by name. It SHALL come AFTER the beat in which a person replies in the thread, so
that a signal opening the work remains the story's opening claim. The last SHALL be shown as an actual MANIFEST — a
grid of object counts asserts that resources exist, while the object shows that
the incident itself is one and that `kubectl` already knows it.

**Every manifest a published asset shows SHALL be appliable**: each field name
SHALL exist on the CRD it claims to be. A screenshot of a manifest nobody can
apply teaches a field that does not exist, and is worse than no manifest.

**It SHALL be produced by a committed, repeatable command**, on the same terms as
the site's screenshots: driving the application's own built bundle against a
fixture the command owns, and writing committed files into the site's assets. A
hand-made screen capture SHALL NOT be published.

**The fixture SHALL be a TIMELINE, not a single state.** The recording's story
requires ordered beats — a signal admitted, a conversation opened, a run
answered, a reply relayed — so the command SHALL script those beats and the
console SHALL reach each of them the way it does in an install: from the data it
is served and the events it is streamed, never by the recorder painting them.

**Nothing in it SHALL come from a real installation** — no cluster name,
namespace, hostname, identity or image digest.

**The recording SHALL carry no text of its own.** No burned-in caption, title
card, subtitle or narration overlay: text in a recording cannot be selected,
translated, searched or read aloud. What the recording is showing SHALL be stated
by the PAGE, beside it, in the page's own words. Words the console itself renders
are the console's and are not affected.

**It MAY carry a caption track**, as a separate timed-text file the page names
and the viewer can turn off. That is text, not pixels — selectable, translatable
and available to a screen reader — so it is the form signposting takes here.
Its words SHALL come from the same source as the beats the page prints, so the
two cannot drift, and it SHALL ship ONE file for every theme, because the words
do not change with the palette.

**There SHALL be no audio track.** A silent recording needs no narration and no
music: narration would lock the demo to one language and re-record with every
voice change, and music carries nothing the page does not already say.

**Beats MAY cross-fade**, and the fade SHALL be taken out of the beats it joins
rather than added between them, so the recording's stated length is its real
length and the caption cues stay in step with it.

The recording SHALL be silent, SHALL ship **one variant per theme**, and SHALL
be delivered:

- **without autoplay** — the reader starts it;
- with a **poster frame** drawn from the recording itself, so the panel shows
  the product before a byte of video is fetched;
- with the file **not fetched until the reader asks for it**.

**It SHALL be bounded and the bounds SHALL be stated where the command lives**:
a duration a visitor will actually watch, and a per-variant byte budget the site
can carry. Exceeding either is a fault in the recording, never a reason to raise
the budget.

**The site SHALL NOT depend on the command to publish.** The produced files are
committed, exactly as the screenshots are, and the command SHALL NOT run as part
of the ordinary test suite.

#### Scenario: A visitor who has installed nothing opens the landing page

- **WHEN** a first-time visitor opens the site root and selects the recording
- **THEN** it shows one piece of work carried from its signal to its answer, and
  they can watch it without installing anything
- **AND** it goes on to show the queue, the wiring, and the conversation as a
  Kubernetes object

#### Scenario: A published manifest is read

- **WHEN** a reader reads a manifest in a published screenshot or recording
- **THEN** every field on it exists on that CRD, so copying it produces an object
  the cluster accepts

#### Scenario: A beat's words are reworded

- **WHEN** a beat's caption is rewritten
- **THEN** nothing else silently changes behaviour — what the poster frame is,
  and which beat it comes from, are declared rather than matched on the text

#### Scenario: The console's UI changes

- **WHEN** the console's UI changes such that the recording no longer shows it
- **THEN** re-running the committed command reproduces the recording, and no
  frame is re-captured by hand

#### Scenario: A reader arrives on a metered connection

- **WHEN** the landing page loads
- **THEN** the poster frame is what is fetched, and the recording itself is
  requested only when the reader starts it

#### Scenario: The page is read with scripting unavailable

- **WHEN** a reader without scripting opens the landing page
- **THEN** the panel is the poster image and a link to the recording, and no
  panel is empty or replaced by a placeholder

#### Scenario: A beat needs explaining

- **WHEN** what happens in the recording would not be obvious from the console's
  own screens
- **THEN** the page states it in prose beside the recording, and no caption is
  burned into the frames

#### Scenario: The fixture is authored

- **WHEN** the timeline behind the recording is written or edited
- **THEN** it carries only invented names, and nothing identifying a real
  installation appears in any published frame

#### Scenario: The recording grows past its budget

- **WHEN** a re-recording exceeds the stated duration or byte budget
- **THEN** the recording is shortened or re-encoded, and the budget stands

#### Scenario: A reader asks whether they can start work themselves

- **WHEN** the recording is watched to the end
- **THEN** it has shown a person opening a conversation and choosing the pipeline
  that answers it, and not only signals arriving
