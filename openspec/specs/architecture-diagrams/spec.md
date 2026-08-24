# architecture-diagrams Specification

## Purpose

The diagram a stranger sees first, and the visual vocabulary anything drawn later
inherits. It exists because the repository could explain every CRD field in
detail and still not answer what the product is for, how it differs from the
automation a reader already runs, or why an agent with cluster access is safe to
adopt.

It is one self-contained `.drawio` file holding several finished pages. The
renders the SITE serves are exported by a committed script and committed with it,
because a branch deploy runs no build step. Any other render is made on demand
and not committed, so no second copy can drift. Its governing rule: every claim on
it is checkable against this repository,
and anything the shipped defaults do not grant is marked as conditional — a
diagram that overstates what an agent will do by default is worse than no diagram
at all.

## Requirements

### Requirement: The drawing is the deliverable and stands alone

`docs/diagrams/agent-ops.drawio` SHALL be the diagram, editable in the draw.io
app or the VS Code extension with no other tooling.

It SHALL carry only finished pages, each with a stated job, and no empty or
superseded ones. At time of writing there are three:

| Page | Job |
|---|---|
| `why` | the standalone poster, carrying its own masthead because nothing around it does |
| `site` | the same argument with the masthead removed, for a reader who opens the diagram full size |
| `landing` | the poster's own composition compressed to fit the landing page's column |

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

### Requirement: The diagram argues the product in its own vocabulary

The page SHALL carry, in this order:

1. Two chips — the category claim (**automation that thinks**) and the platform
   claim (**Kubernetes-native, end to end**).
2. A headline stating the promise, and a one-line subhead naming the three
   inputs — signal, prompt, capabilities — and the domain range.
3. A column of signals including **at least one non-infrastructure example**, so
   domain-neutrality is shown rather than asserted.
4. A boundary marking what one Helm install puts in the reader's own cluster.
5. The three declarations as their real CRD kinds — `Pipeline`, `AgentProfile`,
   `MCPToolset` — each subtitled with the question it answers (*what starts it*,
   *what it should do*, *what it may touch*).
6. A real, copy-pasteable `Pipeline` manifest in block YAML, whose field names
   match the CRD.
7. What the operator runs: one conversation per incident, its own pod, resumable.
8. A verb ladder defining the headline's verb: **investigates → explains → acts
   → asks**.
9. A pluggability band, and three differentiators.

Every object it names SHALL be a real CRD kind and every process a real process.
The page SHALL NOT contain body prose — labels and one-line subtitles only.

#### Scenario: A stranger is shown the diagram

- **WHEN** the diagram is put in front of a reader who knows nothing about the
  project
- **THEN** the promise is legible without needing any CRD name understood first,
  and the Kubernetes-native claim is visible without hunting for it

#### Scenario: Domain range is checked

- **WHEN** the signal column is read
- **THEN** at least one entry is not about infrastructure, sitting beside the
  cluster ones as an equal

#### Scenario: The YAML is checked against the API

- **WHEN** a reader copies the manifest from the picture
- **THEN** every field name exists on the `Pipeline` CRD

### Requirement: Claims on the diagram are checkable, and conditional ones are marked

Every numeric claim SHALL be verifiable from the repository, and the check SHALL
be recorded in the change that introduces it. At time of writing: `11` = files in
`chart/crds/`; `3` pluggable contracts = the channel, signal and work
contracts in `docs/contracts.md` (the activity contract is telemetry, not a
seam, so the label names the three); `0` Secrets = the manager grants itself no
`secrets` verbs in its RBAC; subcharts under `chart/charts/`.

The **Acts** rung SHALL be visually marked as conditional — it is granted by
wiring, and the shipped defaults do not grant it (`k8s-observability` on,
`k8s-admin` off, and the route's own account bound to nothing). A hero implying
autonomous remediation by default would be the one overclaim this diagram must
not make.

#### Scenario: A kind is added or removed

- **WHEN** the number of CRD kinds changes
- **THEN** the number on the diagram is updated in the same change

#### Scenario: A sceptical reader checks the privilege claim

- **WHEN** a reader looks at what the agent may do
- **THEN** *Acts* is distinguished from the other three rungs and states that
  the wiring must grant it

### Requirement: Pluggability is drawn, not merely asserted

The three extension points — signal adapters, agent runtimes, channel adapters —
SHALL each be marked on the part of the picture they extend with a socket badge,
and connected by a visible line to a tile naming concrete alternatives an
adopter would recognise. The band SHALL state that each seam is a documented
HTTP contract requiring no fork.

#### Scenario: A reader wonders about lock-in

- **WHEN** a reader scans the page without reading the body text
- **THEN** three sockets and three lines make it visible that the runtime, the
  signal source and the channel are all replaceable

### Requirement: A fixed visual vocabulary, with meaning never carried by colour alone

The diagram SHALL use line art on a two-colour system — ink plus a single accent
— rather than filled boxes, and every distinction SHALL be encoded in shape or
line style as well as colour.

The dark column below is recorded for whoever renders a dark variant later; the
committed drawing is the light one.

| Element | Light | Dark |
|---|---|---|
| Ink: strokes, headings, labels | `#1A1A1A` | `#E6EDF3` |
| Body / subtitle text | `#5A6472` | `#AAB4C0` |
| Muted caption | `#5F6B7A` | `#9AA4B2` |
| Accent (one only) | `#F59E0B` | `#F0A030` |
| Grouping zone (dotted, no fill) | `#B7C0CB` | `#3E4A59` |
| Cluster boundary (solid panel) | `#F8FAFC` / `#D8DEE6` | `#161B22` / `#2B3138` |
| Card: stat, chip | `#FFFFFF` / `#E2E8F0` | `#161B22` / `#30363D` |
| Extension tile, socket (dashed accent) | `#FFFCF5` | `#241C0E` |
| Code card | `#0D1117` | `#010409` |

Monospace SHALL be requested as the generic `monospace` family so each reader
gets a good one, never a named font that may be absent.

The page SHALL NOT carry a legend: every element on it is labelled, and a legend
would be furniture on the one page whose job is to be read in seconds.

#### Scenario: The diagram is printed in greyscale

- **WHEN** the page is rendered without colour
- **THEN** zones remain distinguishable from panels by border style, extension
  tiles by their dashed border, and the conditional rung by its own label

#### Scenario: A later diagram is added

- **WHEN** a subsequent change draws another diagram of this system
- **THEN** it reuses these values and the icons already embedded in the drawing
  rather than choosing new ones

### Requirement: A page the site leads with is authored for the width it is shown at

A drawing the site displays inline SHALL be authored so that at the content
column's width **no text on it renders below 12px**. Type size on the drawing is
therefore chosen from the export width and the column width together, not from
what reads well in the editor.

A drawing that fails this SHALL be simplified until it passes — reduced to fewer,
larger elements — and SHALL NOT be fixed by widening it past the column, since a
column-width breakout is only ever as wide as the column it breaks out of.

**This governs exported drawings only.** The site's presentation is not one: it
is real text laid out by the theme, and it meets the legibility rule by being
scaled to the width it is given rather than by being authored for it.

Its exports SHALL carry an **opaque ground** in each theme's own canvas colour
rather than a transparent one. The theme swap is a deferred script, so the light
export is on the page before it runs — and a transparent one there is not a
mismatched colour but invisible ink.

#### Scenario: The drawing is displayed inline

- **WHEN** a page renders an exported drawing at the content column's width
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

#### Scenario: An explanation outgrows a still

- **WHEN** an explanation needs more detail than a still can carry at the
  column's width
- **THEN** it is built as a presentation rather than by shrinking the drawing's
  type
