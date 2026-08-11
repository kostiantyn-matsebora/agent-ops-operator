## ADDED Requirements

### Requirement: A generator is the source of truth; the drawing and its exports are outputs

`docs/diagrams/build-why.py` together with `docs/diagrams/icons/*.svg` SHALL be
the authoritative source for the diagram set. `docs/diagrams/agent-ops.drawio`
and every `docs/diagrams/*.svg` SHALL be generated outputs, committed to the
repository so a reader renders nothing.

The generator SHALL take the theme as its argument (`light` | `dark`) and emit
the same page in either palette from one definition of layout and copy, so the
two themes cannot drift apart.

The icon set SHALL be drawn once, in light strokes. The dark icons SHALL be
DERIVED by substitution at generation time — stroke `#1A1A1A` → `#E6EDF3`,
knockout fill `#FFFFFF` → `#0D1117`, accent untouched — because each icon is
embedded as a `data:` URI and an embedded image cannot be recoloured by any
stylesheet the page could carry.

A contributor MAY nudge `agent-ops.drawio` in the draw.io app, but the change
SHALL be folded back into the generator; otherwise the next generation discards
it. `CLAUDE.md` SHALL state this.

#### Scenario: The type scale or palette changes

- **WHEN** a maintainer changes a font size, a colour or a layout constant
- **THEN** it is changed once in `build-why.py`
- **AND** re-running the generator for both themes produces two pages that
  differ only in palette and icon strokes

#### Scenario: An icon is redrawn

- **WHEN** an icon in `docs/diagrams/icons/` is edited
- **THEN** only the light version is drawn, and the dark version is produced by
  the generator's substitution rules with no second hand-drawn file

#### Scenario: Someone edits the .drawio directly and stops there

- **WHEN** a change is made in the draw.io app but not in `build-why.py`
- **THEN** `CLAUDE.md` warns that regeneration overwrites it, and the change is
  not considered landed

### Requirement: One diagram set with three named views

The set SHALL comprise three pages named `why`, `components` and `domain`,
answering in order: what is this for, what are the moving parts, and what is the
model I have to learn. They SHALL share one visual vocabulary so that
`components` and `domain` read as zoom-ins of `why`.

#### Scenario: A reader wants the whole picture

- **WHEN** a reader views the `why` page
- **THEN** every process it depicts also appears on `components`, and every
  domain object it names is a real CRD kind that also appears on `domain`
- **AND** no box on it is an abstraction that exists on no other page

### Requirement: The `why` page sells the product in its own vocabulary

The `why` page SHALL carry, in this order:

1. Two chips — the category claim (**automation that thinks**) and the platform
   claim (**Kubernetes-native, end to end**).
2. A headline stating the promise, and a one-line subhead naming the three
   inputs — signal, prompt, capabilities — and the domain range.
3. A column of signals that includes **at least one non-infrastructure example**,
   so domain-neutrality is visible without being asserted.
4. A boundary marking what one Helm install puts in the reader's own cluster.
5. The three declarations as their real CRD kinds — `Pipeline`, `AgentProfile`,
   `MCPToolset` — each subtitled with the question it answers (*what wakes it*,
   *what it should do*, *what it may touch*).
6. A real, copy-pasteable `Pipeline` manifest in block YAML.
7. What the operator runs: one conversation per incident, its own pod, resumable.
8. A verb ladder defining the headline's verb: **investigates → explains → acts
   → asks**.
9. A pluggability band, and three differentiators.

The page SHALL NOT contain a sentence of body prose inside the picture area —
labels and one-line subtitles only. Explanatory prose belongs to
`docs/architecture.md`.

#### Scenario: A stranger reads the first image

- **WHEN** a reader who knows nothing about the project opens `README.md`
- **THEN** the promise is legible without needing any CRD name to be understood
  first, and the Kubernetes-native claim is visible above the fold

#### Scenario: Domain range is checked

- **WHEN** the signal column is read
- **THEN** at least one entry is not about infrastructure, sitting beside the
  cluster ones as an equal

### Requirement: Claims on the `why` page are checkable, and conditional ones are marked

Every numeric claim on the page SHALL be verifiable from the repository, and the
check SHALL be recorded in the change that introduces it. At time of writing:
`11` = files in `chart/files/crds/`; `3` pluggable contracts = the channel,
signal and work contracts in `docs/contracts.md` (the activity contract is
telemetry, not a seam, so the label names the three); `0` Secrets = the manager
grants itself no `secrets` verbs in its RBAC; `3` bundles = subcharts under
`chart/charts/`.

The **Acts** rung of the verb ladder SHALL be visually marked as conditional —
it is granted by wiring, and the shipped defaults do not grant it
(`k8s-observability` on, `k8s-admin` off, `rbacMode` defaulting to `readonly`).
A hero implying autonomous remediation by default would be the one overclaim
this diagram must not make.

#### Scenario: A kind is added or removed

- **WHEN** the number of CRD kinds changes
- **THEN** the `11` on the `why` page is updated in the same change

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

#### Scenario: A fourth extension point appears

- **WHEN** the operator gains another documented contract
- **THEN** the band gains a fourth tile and the part it extends gains a socket

### Requirement: The `components` page is a C4 component view with correct call direction

The `components` page SHALL show container boundaries for the manager, each
adapter process, the runtime pod, the Kubernetes API server, and the external
systems. Within the manager it SHALL show only `httpapi`, the reconcilers, the
`chat` op queue, `dispatch` and `ingest`. Total nodes SHALL NOT exceed 24
excluding the legend, and each external system SHALL appear once.

Every edge SHALL be labelled with the concrete endpoint, including at minimum
`GET /work`, `GET /channel/ops`, `POST /channel/inbound`, `POST /signal/inbound`,
`GET /activity/stream`, and watch/patch traffic against the Kubernetes API.

Arrowheads SHALL point at the receiver of the request. Because adapters, the
console and runtime pods all call the manager and the manager calls none of
them, those arrows point INTO the manager even where information travels
outward; long-poll edges SHALL be marked as such so the direction is not read as
an error.

#### Scenario: A reader asks who calls whom

- **WHEN** a reader traces any edge between the manager and an adapter
- **THEN** the arrowhead is on the manager, the label names the endpoint, and no
  edge originates at the manager and terminates at an adapter

#### Scenario: An adapter author looks for the contract surface

- **WHEN** an adapter author reads the page
- **THEN** every endpoint their implementation must call appears as a labelled
  edge, and the page links to `docs/contracts.md` for the payloads

### Requirement: The `domain` page shows every CRD kind with Pipeline as the hub

The `domain` page SHALL contain one node per CRD kind, with reference edges
carrying cardinality and ownership edges distinguished from them. `Pipeline`
SHALL be positioned and styled as the hub. The page SHALL show no CRD fields,
and SHALL make visible that `AgentProfile` carries no capabilities and that
`Conversation` holds materialized bindings rather than a reference back to the
`Pipeline` that created it.

It SHALL use the same visual language as the other two pages.

#### Scenario: A newcomer learns the model

- **WHEN** a newcomer reads the page
- **THEN** every kind in `chart/files/crds/` is present, and the wiring
  relationships converge visibly on `Pipeline`

#### Scenario: Someone looks for capabilities on the profile

- **WHEN** the `AgentProfile` node is inspected
- **THEN** no capability, toolset or MCP edge touches it

### Requirement: Every page ships as a theme-paired SVG with selectable text

Each page SHALL be exported to `docs/diagrams/<page>-light.svg` and
`<page>-dark.svg`, both with an OPAQUE background — `#FFFFFF` for light,
`#0D1117` for dark — because a transparent diagram is unreadable against one of
GitHub's two canvases.

Exports SHALL render text as text, never as curves, so the diagrams stay
selectable, searchable, reachable by assistive technology and greppable by the
coverage test. No SVG SHALL embed a raster image or a copy of the draw.io
source, and each SHALL be at most 250 KB.

Every Markdown embed SHALL select the variant with a `<picture>` element keyed
on `prefers-color-scheme`, with descriptive `alt` text.

#### Scenario: A reader browses in either theme

- **WHEN** a reader with a dark or light GitHub theme opens an embedding page
- **THEN** the matching variant is served and every stroke, icon and label is
  legible against that canvas

#### Scenario: Text is extracted from an export

- **WHEN** a committed SVG is searched for a label shown in the diagram
- **THEN** the label is found as SVG text content, not as path data

### Requirement: A fixed visual vocabulary, with meaning never carried by colour alone

The set SHALL use line art on a two-colour system — ink plus a single accent —
rather than filled boxes, and every distinction SHALL be encoded in shape or
line style as well as colour.

| Element | Encoding | Light | Dark |
|---|---|---|---|
| Ink: strokes, headings, labels | — | `#1A1A1A` | `#E6EDF3` |
| Body / subtitle text | — | `#5A6472` | `#AAB4C0` |
| Muted caption | — | `#5F6B7A` | `#9AA4B2` |
| Accent (one only) | — | `#F59E0B` | `#F0A030` |
| Grouping zone | dotted border, no fill | `#B7C0CB` | `#3E4A59` |
| Cluster boundary | solid panel | `#F8FAFC` / `#D8DEE6` | `#161B22` / `#2B3138` |
| Card (stat, chip) | rounded, thin border | `#FFFFFF` / `#E2E8F0` | `#161B22` / `#30363D` |
| Extension tile / socket | dashed accent border, tinted fill | `#FFFCF5` | `#241C0E` |
| Code card | rounded, dark, monospace | `#0D1117` | `#010409` |
| Icon | 64-unit line art, accent detail | ink stroke | derived light stroke |
| Connector | thin solid, dot at origin | ink | ink |
| Pluggability link | dashed, accent | accent | accent |

Monospace SHALL be requested as the generic `monospace` family so each reader
gets a good one, never a named font that may be absent.

#### Scenario: The diagram is printed in greyscale

- **WHEN** a page is rendered without colour
- **THEN** zones remain distinguishable from panels by border style, extension
  tiles by their dashed border, and the conditional rung by its own label

#### Scenario: A later page is added

- **WHEN** a subsequent change draws another diagram of this system
- **THEN** it reuses these values and the existing icon set rather than choosing
  new ones

### Requirement: A legend is required only where the shapes are not self-describing

`components` and `domain` SHALL each carry a legend covering the shapes and line
styles they use. The `why` page SHALL NOT: every element on it carries its own
label, and a legend there would add furniture to the one page whose job is to be
read in seconds.

#### Scenario: A reader meets an unfamiliar shape on a structural page

- **WHEN** `components` or `domain` is viewed on its own
- **THEN** a legend on that page defines every shape and line style it uses

#### Scenario: The hero is checked for furniture

- **WHEN** the `why` page is reviewed
- **THEN** it carries no legend, and no element on it is unlabelled

### Requirement: CRD coverage of the domain page is enforced by a test

A Go test SHALL read `docs/diagrams/domain-light.svg` and assert that every CRD
kind declared in `chart/files/crds/` appears as SVG text. The assertion is
ONE-DIRECTIONAL: every kind must be drawn; the diagram may contain labels that
are not kinds.

This is the staleness case worth testing and the only cheap one. Component
boundaries, endpoints and call direction are governed by the routing rule in
`CLAUDE.md` instead, because scraping route registrations and matching them to
arrow labels would be brittle enough to get disabled — and a disabled test is
worse than a rule that gets read.

#### Scenario: A twelfth kind is added without redrawing

- **WHEN** a CRD kind is added and the `domain` page is not regenerated
- **THEN** the coverage test fails and names the missing kind

#### Scenario: The diagram carries extra labels

- **WHEN** the page contains grouping frames or annotations that are not CRD
  kinds
- **THEN** the coverage test still passes
