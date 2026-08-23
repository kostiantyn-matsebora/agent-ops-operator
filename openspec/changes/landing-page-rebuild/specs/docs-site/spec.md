## ADDED Requirements

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

## MODIFIED Requirements

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

## REMOVED Requirements

### Requirement: The landing page is an adopter hub, not a second README

**Reason**: Replaced by "The landing page opens with what it is, then how it
works". The old requirement pinned an opening the page no longer has — three
labelled chip rows, then a panel set led by the recording, then a row of stat
tiles placed by splitting the page's content at its first heading. Every one of
those is deliberately gone: the counts answer a question a first-time reader has
not asked, the chip rows read as inventory before the page has said what the
product is, and the exported diagram they framed is retired in favour of the
presentation.

**Migration**: The replacement requirement carries forward every scenario still
true of the page — the manifest being copyable text, integrations naming only
what ships, the console's views living on their own page, and reference prose
being linked rather than restated. The scenarios it does not carry forward are
those about the exported diagram, which the landing page no longer shows.
