# documentation-structure Specification

## Purpose
TBD - created by archiving change organize-docs. Update Purpose after archive.
## Requirements
### Requirement: README is a bounded one-page overview

`README.md` SHALL answer two questions and no others: **what this is** and **how
to start it**. It SHALL contain the product pitch and architecture diagram, a
one-line-per-kind CRD table, the five-minute demo, install, a links-onward index
naming the published documentation site first, and short development and status
sections. It SHALL NOT exceed 150 lines, and SHALL NOT contain reference
material (adapter or work contracts, HTTP API tables, tool-access resolution
tables, subchart documentation), upgrade instructions, or extended descriptions
of behaviors — a distinguishing behavior is named in a line, and the document
that owns it is linked.

Content removed from the README SHALL be linked, not deleted: every reader who
followed it there before SHALL be able to reach it from the index in one hop.

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

#### Scenario: Reader wants more than the quick start

- **WHEN** a reader finishes the install and wants the detail behind a behavior
- **THEN** the links-onward index names the published site first and the owning
  document for that content
- **AND** the README itself does not carry that detail

### Requirement: Migration guides live in CHANGELOG.md

`CHANGELOG.md` SHALL be the only file containing chart-version upgrade
instructions. It SHALL list entries newest-first keyed by chart version, retain
the `BREAKING` marker on entries that carry one, and open with an `Unreleased`
heading for the next entry. `README.md` SHALL contain no `Migrating to …`
section.

#### Scenario: Operator plans an upgrade

- **WHEN** an operator upgrading across several chart versions opens
  `CHANGELOG.md`
- **THEN** every migration guide that was previously in `README.md` is present
- **AND** entries appear newest-first (1.12 before 1.8 before 1.7 … before 1.0)

#### Scenario: A new breaking change ships

- **WHEN** a change introduces a breaking chart or CRD change requiring upgrade
  steps
- **THEN** the upgrade steps are added to `CHANGELOG.md` under the version
  heading (or `Unreleased`), and not to `README.md`

### Requirement: Reference material lives in docs/ pages split by audience

The repository SHALL provide `docs/concepts.md` (CRD reference and capability
resolution), `docs/contracts.md` (work contract, channel adapter contract,
signal adapter contract, HTTP API), and ONE PAGE PER BUNDLE SUBCHART
(`docs/<bundle>.md`). Each page SHALL be reachable from the README's
Documentation index in one hop.

A page per bundle rather than one shared page: each subchart is optional, off by
default, and irrelevant to anyone not running it, so they have disjoint
audiences — and a new bundle then adds a file and an index line instead of
growing a shared one.

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

### Requirement: Reorganization preserves content and links

Moving documentation SHALL preserve the existing prose verbatim apart from
heading level and link-target adjustments. Every relative link and heading
anchor in `README.md`, `CLAUDE.md`, `CHANGELOG.md`, and `docs/*.md` SHALL
resolve to an existing file and, where an anchor is given, to an existing
heading in that file.

#### Scenario: Link check after reorganization

- **WHEN** every relative markdown link and anchor across `README.md`,
  `CLAUDE.md`, `CHANGELOG.md`, and `docs/*.md` is resolved
- **THEN** each target file exists and each anchor matches a heading in it

#### Scenario: A former in-README anchor is followed

- **WHEN** a reader follows what used to be an in-page anchor such as the work
  contract or the VictoriaMetrics bundle
- **THEN** the link points at the `docs/` page now holding that section, not at
  a missing README anchor

### Requirement: The document routing rule is recorded for contributors

`CLAUDE.md` SHALL state which document receives which kind of update —
concepts to `docs/concepts.md`, contracts to `docs/contracts.md`, VM bundle to
`docs/vm-bundle.md`, upgrade steps to `CHANGELOG.md`, README only for pitch,
kind list, demo, or install — and SHALL state the README line budget as a
number.

It SHALL further record that `docs/` is a Jekyll source directory published by
GitHub Pages, and route the two kinds of site change separately: site
presentation (layouts, includes, navigation data, CSS, fonts) to the theme files
under `docs/`, and adopter-facing prose to a markdown page — never the reverse.
It SHALL record that the site's `--ao-*` palette is copied from
`console/ui/src/theme/theme.css`, so a token change is a two-file change.

#### Scenario: Contributor finishes a behavior change

- **WHEN** a contributor consults `CLAUDE.md` for what to document after a change
- **THEN** the routing rule names the destination file for that kind of content
- **AND** the README line budget is stated numerically so overrun is checkable

#### Scenario: Contributor changes the site

- **WHEN** a contributor wants to restyle the site or add a page to it
- **THEN** `CLAUDE.md` names the theme files for presentation and a markdown page
  for prose, and states that `docs/` is a published Jekyll source

#### Scenario: A palette token is changed in the console

- **WHEN** a contributor edits a `--ao-*` value in the console's theme
- **THEN** `CLAUDE.md` tells them the site's copied token block is the second
  half of that change

