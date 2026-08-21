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

1. the product diagram exported from `docs/diagrams/`;
2. a real `Pipeline` manifest, written in the page as a fenced code block;
3. one panel per console view, each carrying the question that view answers and
   a screenshot of it.

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
- **THEN** every label on the diagram is legible without opening it full size,
  at the width the content column gives it

#### Scenario: A reader wants to see the product

- **WHEN** a visitor who has installed nothing opens the landing page
- **THEN** the strip's later panels show each console view with the question it
  answers, without leaving the page

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

## ADDED Requirements

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

- **WHEN** the landing page's opening is read
- **THEN** it answers what wakes an agent, what an agent can reach and where it
  answers, rather than which subchart ships each one

### Requirement: A themed image is named once by the page and resolved by the theme

A page publishing an image that has a per-theme variant SHALL name **one** image
file. The theme SHALL resolve which variant is shown from the reader's resolved
theme, and SHALL re-resolve it when the theme is changed while the image is on
screen. A page SHALL NOT name both variants, and SHALL NOT carry any class,
markup or wording that states which theme it is for — that is theme knowledge in
content.

This SHALL hold for every themed image alike, screenshot and diagram, so a page
publishing one learns no new rule.

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
