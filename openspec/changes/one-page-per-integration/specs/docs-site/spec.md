## ADDED Requirements

### Requirement: The site carries one page per integration, named for the system

The site SHALL carry a page for each system the install can reach, under
`docs/integrations/`, and each SHALL answer the same four questions in the same
order: **what starts work here**, **what the agent may reach**, **where it
answers**, and **what turning it on costs**.

That order is the `Pipeline`'s own — a source, a toolset, a channel — so a reader
who has met the model on the landing page meets it again in every integration,
rather than a shape invented per vendor.

**A page SHALL be named for the SYSTEM, not for the subchart that packages it.**
Packaging is renamed when its scope is understood better, and a reader's bookmark
should survive that. A page named for one sender rather than for the thing it
integrates is the mistake this rule exists to prevent.

**A page SHALL state what an integration ships AS**, because that decides what
the reader does next: a bundle they enable with a flag, an image whose resources
they declare themselves, or a contract they implement.

**A page SHALL NOT reproduce the exhaustive values of its chart.** The generated
resource reference and the chart's own `helm show values` are the exhaustive
lists, and a hand-copied inventory rots.

**Every integration the site names SHALL have an owner page, but not every one
SHALL have a page of its own.** Where an existing page already owns the subject,
the name SHALL lead there rather than to a new page restating it — the console
to the console guide, a mechanism to the guide that teaches it.

**THE SET SHALL NOT BE INDEXED BY A PAGE OF ITS OWN.** The site's navigation
already lists every integration page, so an index would be a second navigation
written by hand — the thing `_data/nav.yml` exists to prevent — and it would
have to be edited in step with the sidebar forever.

**A RUNTIME IS NOT AN INTEGRATION.** The four questions are the `Pipeline`'s
three seams — what starts work, what may be reached, where it answers — and a
runtime is the fourth thing: what EXECUTES the agent. A runtime vendor SHALL
therefore keep its own reference page rather than be filled into a shape that
would leave three of its four sections empty. Shipping as a bundle is not what makes something an integration.

#### Scenario: A reader arrives from a mark on the landing page

- **WHEN** a reader follows an integration named on the landing page
- **THEN** they reach a page that says what starts work there, what the agent may
  reach, where it answers, and what it costs to turn on

#### Scenario: A subchart is renamed

- **WHEN** a bundle subchart is renamed
- **THEN** the integration page's name is unchanged, because it is named for the
  system rather than for the packaging

#### Scenario: An integration ships without a chart

- **WHEN** an integration ships as an image with no chart rendering it
- **THEN** its page says so, and says which resources the reader declares
  themselves

#### Scenario: A reader wants every value

- **WHEN** a reader needs the exhaustive values for an integration's chart
- **THEN** the page sends them to the chart's own values rather than restating
  them

## RENAMED Requirements

- FROM: `### Requirement: Custom resource templates and examples are generated, not written`
- TO: `### Requirement: Published facts about the chart are generated, not written`

## MODIFIED Requirements

### Requirement: Published facts about the chart are generated, not written

Every custom resource template on the site SHALL be generated from the CRDs the
chart ships. Every worked example SHALL be rendered from the chart's own values
for the bundle that owns that lane.

**What a bundle RENDERS SHALL be generated too** — the objects each of its
components produces, attributed to the value that turns that component on. A
component inventory is a fact about the chart in exactly the way a worked example
is, and typing it by hand makes it correct on the day it is typed and silently
wrong afterwards.

None of the three SHALL be hand-written or invented. An invented example is a
second set of values to keep true, and it is how a real identifier gets pasted in
as the better-looking example.

**What each component turns on SHALL be DECLARED, not derived by toggling.** A
chart's flags are not independent — one component may refuse to render without
another — so the values that turn a component on are named, under the same
guard that keeps an invented identifier out of a worked example.

Because the site is built by a branch deploy with no build step, generated files
SHALL be committed — and CI SHALL regenerate them and FAIL on any difference.
Committing generated output without that check produces a file that is correct
the day it is written and silently wrong after the next field rename.

**The generator owns what is rendered. The page owns what it means.** A generated
block states the objects a component produces. The prose around it carries what a
diff cannot express — which object is conditional, which account carries which
grant, and why a component exists at all.

A guide SHALL carry the MINIMAL resource — the required fields plus what that
guide teaches — and link the full generated reference for the rest.

#### Scenario: A CRD field is renamed

- **WHEN** a field is added, renamed or removed in the CRDs
- **THEN** CI fails until the generated templates are regenerated

#### Scenario: A chart value changes

- **WHEN** a bundle's values change what it renders
- **THEN** CI fails until the generated examples are regenerated

#### Scenario: A bundle starts rendering a new object

- **WHEN** a component of a bundle begins rendering an object it did not before
- **THEN** CI fails until that bundle's rendered inventory is regenerated, so no
  page can claim a component renders less than it does

#### Scenario: A component cannot render alone

- **WHEN** a component refuses to render without another value being set
- **THEN** the values that turn it on are declared together, rather than the
  generator toggling one flag and reporting a failure as an empty result

#### Scenario: A guide covers a large kind

- **WHEN** a kind has more fields than a reader can usefully scan
- **THEN** the guide shows the minimal resource and links the full reference,
  rather than reproducing every field inline

#### Scenario: An example needs a value the chart does not carry

- **WHEN** a worked example would require an invented identifier
- **THEN** the chart's own placeholder is used, so the example and the shipped
  values cannot disagree

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

**EVERY NAME IN THAT GROUP SHALL LEAD TO THE PAGE THAT OWNS IT.** A row of marks
is a set of promises, and a promise a reader cannot follow is worse than one not
made — they went looking, and the page had nowhere to send them.

The destination SHALL be whichever page already owns that subject, and a name
SHALL NOT get a page of its own merely to be a destination. Where a name is a
MECHANISM rather than a system, the page that teaches the mechanism is the
owner.

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

#### Scenario: A reader follows a name in the group

- **WHEN** a reader selects a name in the `works with` group
- **THEN** they reach the page that owns that subject, and no name in the group
  is inert
