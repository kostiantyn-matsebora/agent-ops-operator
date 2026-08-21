## MODIFIED Requirements

### Requirement: The drawing is the deliverable and stands alone

`docs/diagrams/agent-ops.drawio` SHALL be the diagram, editable in the draw.io
app or the VS Code extension with no other tooling.

It SHALL carry only finished pages, each with a stated job, and no empty or
superseded ones. At time of writing there are three:

| Page | Job |
|---|---|
| `why` | the standalone poster, carrying its own masthead because nothing around it does |
| `site` | the same argument with the masthead removed, for a reader who opens the diagram full size |
| `landing` | the simplified spine the landing page leads with |

Every icon it uses SHALL be embedded as a `data:` URI, so the file has no
external asset references and renders wherever it is opened.

A page the SITE serves SHALL be exported by the committed export script, and
those exports SHALL be committed — the site is built by a branch deploy that runs
no build step, so an uncommitted render is a missing image. A render wanted for
anything else (a slide, a post, a README) is exported at that moment and is NOT
added to the repository, so no second copy of it can drift.

#### Scenario: A contributor opens the diagram

- **WHEN** a contributor opens `docs/diagrams/agent-ops.drawio`
- **THEN** every page in it is one the site or the poster uses, every icon
  renders, and nothing is fetched from outside the file

#### Scenario: A page the site serves is edited

- **WHEN** a page the site serves changes
- **THEN** the export script is re-run and its committed SVGs are updated in the
  same change

#### Scenario: A rendered image is needed

- **WHEN** someone needs a PNG or SVG for a slide or a post
- **THEN** it is exported from the drawing at that moment, and the export is not
  added to the repository

## ADDED Requirements

### Requirement: The landing page of the drawing is the poster's composition, minus what the prose states

The `landing` page SHALL keep the poster's own composition rather than restate it
in another shape: the **cluster panel** and the pill naming it, the **signal
column** beside it, the **declare zone** carrying the three kinds and a real
`Pipeline` manifest, the **run zone** below it, and the connectors from the
signal column into both.

It SHALL NOT carry the masthead, the verb ladder, the pluggability band or the
differentiator cards. Each of those is stated by the landing page in text, and a
claim the prose makes SHALL NOT also be drawn.

Reducing the drawing SHALL mean **removing whole elements**, never shrinking type
— the two are not interchangeable, and shrinking is what made the poster
unreadable in the column in the first place.

The signal column SHALL keep **at least one non-infrastructure example**, so
domain-neutrality survives the simplification.

Every object it names SHALL be a real CRD kind and every process a real process.
It SHALL carry no body prose — headings and one-line labels only.

#### Scenario: A section is added to the landing page's prose

- **WHEN** the landing page states a claim in prose that the `landing` page draws
- **THEN** it is removed from the drawing rather than kept in both

#### Scenario: A stranger reads the first panel

- **WHEN** a reader who knows nothing about the project reads the first panel
- **THEN** the sequence from an event to a running agent is legible without
  needing any CRD name understood first
- **AND** what one Helm install puts in their own cluster is marked on it

#### Scenario: The drawing has to be made smaller

- **WHEN** the drawing must fit a narrower place than it does
- **THEN** elements are removed or their layout compressed, and the type sizes
  are left alone

#### Scenario: Domain range is checked

- **WHEN** the signal column is read
- **THEN** at least one entry is not about infrastructure, sitting beside the
  cluster ones as an equal

### Requirement: A page the site leads with is authored for the width it is shown at

A drawing the site displays inline SHALL be authored so that at the content
column's width **no text on it renders below 12px**. Type size on the drawing is
therefore chosen from the export width and the column width together, not from
what reads well in the editor.

A drawing that fails this SHALL be simplified until it passes — reduced to fewer,
larger elements — and SHALL NOT be fixed by widening it past the column, since a
column-width breakout is only ever as wide as the column it breaks out of.

Its exports SHALL carry an **opaque ground** in each theme's own canvas colour
rather than a transparent one. The theme swap is a deferred script, so the light
export is on the page before it runs — and a transparent one there is not a
mismatched colour but invisible ink.

#### Scenario: The drawing is displayed inline

- **WHEN** the landing page renders the diagram at the content column's width
- **THEN** every label on it is at least 12px on screen

#### Scenario: More detail is wanted on the drawing

- **WHEN** a contributor wants to add elements to a drawing the site displays
  inline
- **THEN** they either keep it within the legibility rule or put the detail on
  the full-size page instead

#### Scenario: The light export is shown on a dark page

- **WHEN** the light variant is on a dark-theme page, before the swap runs or
  because scripting is unavailable
- **THEN** it carries its own ground and stays legible
