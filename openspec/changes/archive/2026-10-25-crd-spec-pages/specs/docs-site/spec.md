# docs-site Delta

## MODIFIED Requirements

### Requirement: Navigation is declared once, in one data file

Site navigation SHALL be declared in `docs/_data/nav.yml` — groups of entries,
each a title and a path — and rendered by the sidebar include. The entry for
the page currently being viewed SHALL be marked as current, both visually and
for assistive technology.

- **Adding a page SHALL require editing exactly one navigation source.**
- An entry pointing at a file that does not exist SHALL be a defect.
- The file SHALL carry an entry for every site deliverable.

**Each group SHALL be expandable and collapsible.** The group holding the
current page SHALL be open when the page loads. A reader's open/closed choices
SHALL persist for that reader across pages.

With no script available every group SHALL be open, so nothing in the
navigation is unreachable. The toggle SHALL be operable by keyboard and SHALL
state its expanded or collapsed state to assistive technology.

#### Scenario: The current page is shown

- **WHEN** any page in the navigation is open
- **THEN** its sidebar entry is visually distinguished and carries
  `aria-current="page"`
- **AND** the group holding it is open

#### Scenario: A page is added to the site later

- **WHEN** a contributor publishes a further page on the site
- **THEN** one entry in `_data/nav.yml` is the only theme-side change needed
- **AND** no navigation markup is written a second time

#### Scenario: A reader collapses a group

- **WHEN** a reader collapses a nav group and opens another page
- **THEN** the group stays collapsed for that reader, unless it holds the new
  current page

#### Scenario: Scripting is unavailable

- **WHEN** the sidebar renders with no script
- **THEN** every group is open and every entry reachable

### Requirement: The landing page opens with what it is, then how it works

`docs/index.md` SHALL be the site's landing page: a short orientation for an
adopter — what the operator is, where it earns its keep, and the paths onward
grouped by what the reader is trying to do.

Its opening SHALL be, in this order: the **project's name**, **one sentence**
saying what it is, the **claim chips**, and then a **tabbed panel set**.
Nothing SHALL stand between the name and the panel set except those two lines
and the chips.

**The one sentence is the whole of the standfirst.** A second explanatory
paragraph SHALL NOT be added beneath it — the panel set immediately below
states the model in full, and a page that explains itself twice has buried
what it is showing.

The panel set's panels SHALL be, in order:

1. the **flow picture** — the same generated drawing `README.md` carries
   (`readme-flow`), named once by the page and theme-resolved, its alt text
   stating the model it shows
2. the **recording of the product working**, carrying one piece of work from
   the event that starts it to the answer a person replies to
3. a real `Pipeline` manifest, written in the page as a fenced code block

The picture SHALL come first. It answers the question a first-time reader
actually has — what is this and how does it fit together — and the recording
answers the next one.

One still SHALL be the whole answer. The page SHALL NOT also carry an
animated restatement of the same model.

**The landing page SHALL NOT tour the console's views.** They belong to the
Console page, which takes each in turn with the question it answers —
publishing them in both places is one tour at two altitudes.

The landing page SHALL instead link the Console page for them.

The manifest panel SHALL be page text rather than an exported image, so it
can be selected, copied and searched. Every field name on it SHALL exist on
the `Pipeline` CRD.

Words the page can say SHALL be said by the page. The name, the sentence and
the chips are page text, because text in an exported image cannot be
selected, translated, searched, or read except through alt text.

**What the install reaches SHALL be named once, as a single labelled group**
below the panel set, rather than as several labelled rows above it. A reader
decides whether this fits their stack after they know what it is, not before.

Anything named in that group SHALL ship in the release being described.

**EVERY NAME IN THAT GROUP SHALL LEAD TO THE PAGE THAT OWNS IT.** A row of
marks is a set of promises.

A promise a reader cannot follow is worse than one not made — they went
looking, and the page had nowhere to send them.

The destination SHALL be whichever page already owns that subject, and a name
SHALL NOT get a page of its own merely to be a destination.

Where a name is a MECHANISM rather than a system, the page that teaches the
mechanism is the owner.

The page SHALL carry a section headed by a **question the reader is asking** —
where this earns its keep — introduced by one sentence and answered by a
**two-column table** naming each area of use and what happens there.

Each row MAY carry the mark of the system it names.

**A table SHALL be used rather than a grid of tiles.** The rows are read
against each other, and a tile grid spends most of its area on ornament for
six lines of text.

**The viewer SHALL be given its own full-width strip** beneath that table
rather than a row within it.

It is not one more area of use — it is where every area is watched and
answered — and a row would state that it is a peer.

**Headline figures SHALL NOT be presented.** A count of kinds, contracts or
bundles answers a question a first-time reader has not yet asked.

It occupies the position where the reader decides whether the product is for
them at all. Such counts belong on the reference pages that own them.

The layout SHALL place the page's sections **in the order the page writes
them** and SHALL NOT split the page's content to insert anything of its own.

It SHALL NOT duplicate `README.md`'s CRD table, demo transcript, install
commands or status — those stay in the README, which the landing page links
to. Reference prose SHALL NOT be written into the landing page — it links to
the page that owns that content.

#### Scenario: An adopter opens the site root

- **WHEN** a first-time visitor opens the site root
- **THEN** they see the project's name, one sentence saying what it is, and
  the flow picture, before anything else

#### Scenario: A reader asks whether it is for them

- **WHEN** the landing page is read past its opening
- **THEN** a question-headed section answers where the product earns its
  keep, as a table of areas of use rather than a grid of figures

#### Scenario: A reader asks whether it fits their stack

- **WHEN** the landing page is read
- **THEN** what the install reaches is named once, below the panel set, and
  not before the page has said what the product is

#### Scenario: An integration does not ship yet

- **WHEN** an integration is named in the `works with` group
- **THEN** the release being described ships it — the group answers what the
  product DOES, so there is no honest place for a "coming soon" entry
- **AND** an integration whose bundle slips out of the release is removed
  from the group in the same change that slips it

#### Scenario: The page adds a section

- **WHEN** a section is added to the landing page
- **THEN** it renders where the page wrote it, and the layout inserts nothing
  of its own between the page's sections

#### Scenario: A reader wants to see the product

- **WHEN** a visitor who has installed nothing opens the landing page
- **THEN** the flow picture states the model without them installing
  anything, and the recording is one tab away
- **AND** the console's own views are one link away, on the page that owns
  them

#### Scenario: The flow drawing changes

- **WHEN** `readme-flow.py` is edited and re-run
- **THEN** the README and the landing page show the same regenerated drawing,
  because both name the same files

#### Scenario: The manifest is copied from the page

- **WHEN** a reader selects the `Pipeline` panel and copies its contents
- **THEN** they get text, not an image, and every field on it exists on the
  `Pipeline` CRD

#### Scenario: Content would be duplicated

- **WHEN** a contributor is tempted to restate the install command or a CRD
  description on the landing page
- **THEN** the landing page links to the owning document instead

#### Scenario: A reader follows a name in the group

- **WHEN** a reader selects a name in the `works with` group
- **THEN** they reach the page that owns that subject, and no name in the
  group is inert

### Requirement: The Introduction states no reference detail

The Introduction SHALL NOT restate content another document owns. In prose it
SHALL name no CRD field, no HTTP endpoint, no Helm values key, no install
command and no YAML example. Where such detail is needed it SHALL link to the
document that owns it.

**The domain-model diagram's edge labels are the one exception.** An edge MAY
be labelled with the ref field that makes the relation (`signalSourceRefs`,
`capabilityRef`), because the label is what ties the picture to the YAML an
adopter writes.

Every such label SHALL name a field the CRDs actually have, and the
generated-docs check SHALL fail on one they no longer have.

The governing test SHALL be whether a sentence would have to change if a CRD
field were renamed. A sentence that would belongs in the reference page, not
here — a diagram label that would is caught by the check instead.

#### Scenario: A field is renamed

- **WHEN** a CRD field is renamed
- **THEN** the Introduction's prose needs no edit, and the diagram's stale
  edge label fails the generated-docs check until it is updated

#### Scenario: A contributor adds reference prose to the Introduction

- **WHEN** a contributor is tempted to document a field, endpoint or values
  key on the Introduction
- **THEN** it goes in the document that owns that content and the
  Introduction links to it

## REMOVED Requirements

### Requirement: The Introduction orients an adopter to the model

**Reason**: The concepts section's card grid is replaced by the linked
domain-model diagram, and the card-shaped scenarios (odd card counts,
whole-card targets) do not survive the form change.

**Migration**: The added requirement below carries the page's obligations in
their diagram shape. The page, its URL and its two-section structure are
unchanged.

#### Scenario: The cards are gone

- **WHEN** the Introduction's concepts section renders
- **THEN** it is the domain-model diagram, not a card grid

## ADDED Requirements

### Requirement: The Introduction orients an adopter with the domain model

The site SHALL carry an Introduction page for an adopter who has read the
landing page and has not yet decided.

It SHALL be reachable from the navigation under the same group as the landing
page, and SHALL declare its own URL rather than relying on a permalink style
the site does not configure.

The page SHALL carry a short opening and then exactly two sections:
**understand the concepts**, and **follow the guides**. It SHALL NOT grow
further sections — material that fits neither belongs in a guide or a
reference page.

- The opening SHALL state what the operator is and the structural idea that
  separates it from a bot or a runbook — identity, wiring and execution are
  separate objects, so what an agent may reach comes from the route that
  STARTED it and never from the agent itself.
- The concepts section SHALL present the model as **the domain-model
  diagram**: every CRD kind, grouped by role — what starts work, the
  capability shape, the two wiring kinds, what a route produces.
- Each kind on the diagram SHALL carry the glyph the console draws for it,
  and each SHALL be a link to that kind's spec page.
- A connector SHALL carry an arrowhead only where the relation is genuinely
  directional — data flowing, or one kind opening another. A pure reference
  SHALL be a plain line.
- Related kinds SHALL be grouped so one connector serves the group, rather
  than one connector per field.
- Beside the diagram the section SHALL keep one or two sentences per kind,
  each linking to that kind's spec page.
- The guides section SHALL list the task-shaped walkthroughs when there are
  any, and SHALL say plainly that there are none when there are none. It
  SHALL NOT list guides that do not exist as though they were available.

#### Scenario: An adopter reads the Introduction before the reference

- **WHEN** an adopter who has read only the landing page opens the
  Introduction
- **THEN** they can name what STARTS a conversation, what decides what it may
  touch and what executes it, and can see which guide to read next

#### Scenario: A reader arrives from the console

- **WHEN** a reader who has used the console's topology view reads the
  diagram
- **THEN** each kind carries the glyph the console draws for it, in the
  theme's own colour

#### Scenario: A kind on the diagram is followed

- **WHEN** a reader clicks any kind on the diagram
- **THEN** they reach that kind's spec page, and the shape carries a visible
  affordance that it leads somewhere

#### Scenario: A relation is not directional

- **WHEN** the diagram connects a kind to one that merely references it
- **THEN** the connector carries no arrowhead

#### Scenario: No guides exist yet

- **WHEN** the guides section has nothing to list
- **THEN** it says so, and lists no unwritten guide as though it could be
  read

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
