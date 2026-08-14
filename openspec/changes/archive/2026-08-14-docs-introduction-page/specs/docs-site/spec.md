## MODIFIED Requirements

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

## ADDED Requirements

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
