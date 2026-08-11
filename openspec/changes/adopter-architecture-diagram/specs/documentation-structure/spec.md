## MODIFIED Requirements

### Requirement: README is a bounded one-page overview

`README.md` SHALL contain only the product pitch and architecture diagram, a
one-line-per-kind CRD table, the behaviors that distinguish the operator, the
five-minute demo, install, a Documentation index, development, and status. It
SHALL NOT exceed 150 lines, and SHALL NOT contain reference material (adapter
or work contracts, HTTP API tables, tool-access resolution tables, subchart
documentation) or upgrade instructions.

The architecture diagram SHALL be the committed `why` export from
`docs/diagrams/`, embedded through a `<picture>` element that selects the light
or dark variant by `prefers-color-scheme` and carries descriptive `alt` text.
It SHALL NOT be an inline ASCII drawing: a picture that must be maintained by
hand in a line-budgeted file is the reason the previous one showed three of the
system's processes and none of its kinds.

#### Scenario: Reader wants the product overview

- **WHEN** a first-time reader opens `README.md`
- **THEN** the pitch, the list of CRD kinds, the demo command, and the install
  command are all present without following a link
- **AND** the file is at most 150 lines

#### Scenario: A feature change adds reference detail

- **WHEN** a change documents a new contract endpoint, CRD field semantics, or
  subchart component
- **THEN** the text is written to the relevant `docs/` page, not to `README.md`
- **AND** `README.md` changes only if the kind list, pitch, demo, or install
  command changed

#### Scenario: Reader opens the README in either GitHub theme

- **WHEN** the README is viewed with a light or a dark GitHub theme
- **THEN** the architecture diagram is served in the matching variant and is
  legible against that canvas

#### Scenario: The architecture diagram must change

- **WHEN** the product's top-level story or component set changes
- **THEN** the change is made in `docs/diagrams/build-why.py` and both themes
  are regenerated, and `README.md` itself is unchanged apart from `alt` text

### Requirement: Reference material lives in docs/ pages split by audience

The repository SHALL provide `docs/concepts.md` (CRD reference and capability
resolution), `docs/contracts.md` (work contract, channel adapter contract,
signal adapter contract, HTTP API), `docs/architecture.md` (the visual index),
and ONE PAGE PER BUNDLE SUBCHART (`docs/<bundle>.md`). Each page SHALL be
reachable from the README's Documentation index in one hop.

A page per bundle rather than one shared page: each subchart is optional, off by
default, and irrelevant to anyone not running it, so they have disjoint
audiences — and a new bundle then adds a file and an index line instead of
growing a shared one.

`docs/architecture.md` SHALL embed the `components` and `domain` diagrams with
at most one orienting paragraph each, and SHALL link into the page that owns the
underlying detail. It SHALL NOT restate CRD field semantics, endpoint payloads,
or subchart values — a sentence explaining any of those belongs on the page that
owns it. The page is an index into the reference, never a fourth copy of it.

#### Scenario: Adapter author needs the contract

- **WHEN** an author implementing a channel or signal adapter follows the
  README's Documentation index
- **THEN** `docs/contracts.md` contains the full long-poll/push/state/auth
  contract for both adapter kinds and the runtime work contract

#### Scenario: A new bundle subchart ships
- **WHEN** a chart gains another optional bundle
- **THEN** its documentation is a new `docs/<bundle>.md` with one line in the
  README's Documentation index, and no existing page grows

#### Scenario: Reader needs CRD detail beyond the README table

- **WHEN** a reader wants the full description of a CRD kind or the
  merge/overwrite resolution rules for `toolsets` and `mcpConfigs`
- **THEN** `docs/concepts.md` contains it

#### Scenario: Reader wants to see the shape of the system

- **WHEN** a reader follows the Documentation index to `docs/architecture.md`
- **THEN** the component view and the domain model are both present as embedded
  theme-paired diagrams
- **AND** each is introduced by at most one paragraph that links to
  `docs/contracts.md` or `docs/concepts.md` for the detail

#### Scenario: Someone adds reference prose to the architecture page

- **WHEN** a change would explain a CRD field, an endpoint payload, or a
  subchart value on `docs/architecture.md`
- **THEN** the text is written to `docs/concepts.md`, `docs/contracts.md`, or
  `docs/<bundle>.md` instead, and the architecture page links to it

### Requirement: The document routing rule is recorded for contributors

`CLAUDE.md` SHALL state which document receives which kind of update —
concepts to `docs/concepts.md`, contracts to `docs/contracts.md`, VM bundle to
`docs/vm-bundle.md`, upgrade steps to `CHANGELOG.md`, README only for pitch,
kind list, demo, or install — and SHALL state the README line budget as a
number.

It SHALL additionally route diagram updates: a change that adds or removes a
CRD kind, changes a process boundary, or adds, removes or renames an adapter or
runtime contract endpoint SHALL update the affected page in
`docs/diagrams/build-why.py` and regenerate both themes in the same change.
`CLAUDE.md` SHALL record that the GENERATOR and `docs/diagrams/icons/` are the
source, and that `agent-ops.drawio` and every `*.svg` are generated artefacts —
a nudge made in the draw.io app must be folded back into the generator or the
next regeneration discards it.

#### Scenario: Contributor finishes a behavior change

- **WHEN** a contributor consults `CLAUDE.md` for what to document after a change
- **THEN** the routing rule names the destination file for that kind of content
- **AND** the README line budget is stated numerically so overrun is checkable

#### Scenario: Contributor adds a contract endpoint

- **WHEN** a change adds an endpoint to the channel, signal, or work contract
- **THEN** `CLAUDE.md` directs the contributor to update the `components` page
  and re-export, alongside the `docs/contracts.md` update

#### Scenario: Contributor is tempted to edit an output

- **WHEN** a contributor wants to correct a label in a committed diagram
- **THEN** `CLAUDE.md` directs them to `build-why.py`, and states that both the
  `.drawio` and the SVGs are regenerated from it, never edited as the source

#### Scenario: A diagram change must cover both themes

- **WHEN** any diagram is regenerated
- **THEN** the light and dark variants are produced from the same generator run
  and committed together, so the two cannot drift
