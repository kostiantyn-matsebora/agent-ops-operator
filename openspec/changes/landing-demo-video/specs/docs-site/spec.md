## ADDED Requirements

### Requirement: The landing page demonstrates the product with a generated recording

The landing page SHALL carry a **recording of the product working**, showing one
piece of work from the event that starts it to the answer a person replies to.
It SHALL be the strip's first panel.

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

- **WHEN** a first-time visitor opens the site root
- **THEN** the first panel shows one piece of work carried from its signal to its
  answer, and they can watch it without installing anything

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

## MODIFIED Requirements

### Requirement: The landing page is an adopter hub, not a second README

`docs/index.md` SHALL be the site's landing page: a short orientation for an
adopter — what the operator is, what "takes care of it" means, where it is
pluggable, and the paths onward grouped by what the reader is trying to do.

Its opening SHALL be, in this order: **what it plugs into**, then a **tabbed
panel set**, then the stat tiles.

What it plugs into SHALL be grouped by the QUESTION A READER ASKS, not by which
subchart ships it: what wakes an agent, what an agent can reach, and where it
answers. A reader deciding whether this fits their stack is asking those three,
and "which of these is in the chart" is a packaging fact they ask later, on the
Installation page.

The panel set's panels SHALL be, in order:

1. a **recording of the product working**, carrying one piece of work from the
   event that starts it to the answer a person replies to;
2. the product diagram exported from `docs/diagrams/`;
3. a real `Pipeline` manifest, written in the page as a fenced code block.

**The landing page SHALL NOT tour the console's views.** They belong to the
Console page, which takes each in turn with the question it answers, and
publishing them in both places is one tour at two altitudes. The landing page
SHALL instead link the Console page for them.

The manifest panel SHALL be page text rather than an exported image, so it can be
selected, copied and searched, and every field name on it SHALL exist on the
`Pipeline` CRD.

The diagram and the page SHALL NOT disagree about what the product claims, and
**nothing SHALL be said twice**: a claim the page states in prose SHALL NOT also
be drawn. The drawio source therefore carries a LANDING page distinct from the
standalone poster, carrying neither the poster's masthead nor the sections the
page states as prose.

Words the page can say SHALL be said by the page, not by the drawing — the
eyebrow pills, the headline and the standfirst are page text, because text in
an exported image cannot be selected, translated, searched, or read except
through alt text.

The exported asset SHALL be self-contained (no third-party request) and SHALL
ship as one variant per theme, selected by the applied theme. Both SHALL be
produced by a committed export script rather than by remembered commands, since
the dark variant requires a correction the exporter cannot make itself. The
diagram SHALL use the product's own palette.

Headline figures SHALL be presented as a row of stat tiles whose words come
from the page and whose markup comes from the layout. They are a KPI row, not
hero figures: values wear ink, identity is carried by a mark beside them, and
the number is never the only thing distinguishing one tile from another.

The layout SHALL place them by **splitting the page's rendered content at its
first section heading** — everything before the page's first section is its
opening, the tiles close that opening, and the sections follow. The page SHALL
NOT write a marker, a wrapper or any other instruction about where they go.

It SHALL NOT duplicate `README.md`'s CRD table, demo transcript, install
commands or status; those stay in the README, which the landing page links to.
Reference prose SHALL NOT be written into the landing page — it links to the
page that owns that content.

#### Scenario: An adopter opens the site root

- **WHEN** a first-time visitor opens the site root
- **THEN** they see what the project is and a grouped set of paths onward
- **AND** every path is a link to the document that owns that content

#### Scenario: A reader asks whether it fits their stack

- **WHEN** the landing page is opened
- **THEN** what ships in the chart is named before the reader scrolls, and so is
  the fact that anything else goes through the contracts

#### Scenario: An integration does not ship yet

- **WHEN** an integration is named in the opening
- **THEN** the release being described ships it — the groups answer what the
  product DOES, so there is no group meaning "coming soon" and no honest place
  to put one
- **AND** an integration whose bundle slips out of the release is removed from
  the opening in the same change that slips it

#### Scenario: One thing answers two of the questions

- **WHEN** an integration is both where work arrives from and where answers go
- **THEN** it appears in both groups, because each group answers its own
  question and neither is a list of distinct things

#### Scenario: The page adds a section to its opening

- **WHEN** a page's opening gains or loses a block before its first heading
- **THEN** the tiles still close that opening, and nothing in the page changed
  to make that so

#### Scenario: The first panel is read at the page's own width

- **WHEN** the landing page is opened and the first panel is on screen
- **THEN** the recording's poster frame reads as the product at the width the
  content column gives it, without being opened full size

#### Scenario: The diagram is read at the page's own width

- **WHEN** the landing page is opened and the diagram panel is selected
- **THEN** every label on the diagram is legible without opening it full size,
  at the width the content column gives it

#### Scenario: A reader wants to see the product

- **WHEN** a visitor who has installed nothing opens the landing page
- **THEN** the first panel plays one piece of work from its signal to its answer,
  without leaving the page
- **AND** the console's own views are one link away, on the page that owns them

#### Scenario: The manifest is copied from the page

- **WHEN** a reader selects the `Pipeline` panel and copies its contents
- **THEN** they get text, not an image, and every field on it exists on the
  `Pipeline` CRD

#### Scenario: The diagram is viewed in the dark theme

- **WHEN** a reader with the dark theme opens the landing page
- **THEN** the dark variant is shown, legible down to its icons, on its own
  opaque ground

#### Scenario: The landing page and the diagram disagree

- **WHEN** the product pitch changes in `docs/diagrams/`
- **THEN** the landing page's prose is updated from the diagram's copy and the
  asset is re-exported, so the page never states a claim the diagram contradicts

#### Scenario: A claim is stated in both places

- **WHEN** a section of prose on the landing page is also drawn on its diagram
- **THEN** it is removed from the drawing, which carries only what the prose
  does not

#### Scenario: The diagram is re-exported

- **WHEN** the drawio source changes and the diagram is exported again
- **THEN** one command produces every variant of every exported page with the
  dark ones' icons already corrected
- **AND** no manual repair step stands between the export and the site

#### Scenario: Content would be duplicated

- **WHEN** a contributor is tempted to restate the install command or a CRD
  description on the landing page
- **THEN** the landing page links to the owning document instead

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
for that kind (`console/ui/src/graph/shapes.tsx`), copied on the same terms as
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
