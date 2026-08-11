## ADDED Requirements

### Requirement: The drawing is the deliverable and stands alone

`docs/diagrams/agent-ops.drawio` SHALL be the diagram, editable in the draw.io
app or the VS Code extension with no other tooling. It SHALL carry exactly one
finished page and no empty or superseded ones.

Every icon it uses SHALL be embedded as a `data:` URI, so the file has no
external asset references and renders wherever it is opened. Rendered images
(SVG, PNG) are produced from it on demand and are NOT committed, so there is
never a second copy to keep in step.

#### Scenario: A contributor opens the diagram

- **WHEN** a contributor opens `docs/diagrams/agent-ops.drawio`
- **THEN** it opens with one page, every icon renders, and nothing is fetched
  from outside the file

#### Scenario: A rendered image is needed

- **WHEN** someone needs a PNG or SVG for a README, a slide or a post
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
   `MCPToolset` — each subtitled with the question it answers (*what wakes it*,
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
`chart/files/crds/`; `3` pluggable contracts = the channel, signal and work
contracts in `docs/contracts.md` (the activity contract is telemetry, not a
seam, so the label names the three); `0` Secrets = the manager grants itself no
`secrets` verbs in its RBAC; `3` bundles = subcharts under `chart/charts/`.

The **Acts** rung SHALL be visually marked as conditional — it is granted by
wiring, and the shipped defaults do not grant it (`k8s-observability` on,
`k8s-admin` off, `rbacMode` defaulting to `readonly`). A hero implying
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
