# documentation-structure Specification

## Purpose
TBD - created by archiving change organize-docs. Update Purpose after archive.
## Requirements
### Requirement: README is a bounded one-page overview

`README.md` SHALL answer two questions and no others: **what this is** and **how
to start it**. It SHALL contain the product pitch and architecture diagram, a
one-line-per-kind CRD table, **a bounded start** — the credential, the install
command, one ask, and a link naming the site's Getting started page as the
walkthrough — a links-onward index naming the published documentation site
first, and short development and status sections. It SHALL NOT exceed 150 lines,
and SHALL NOT contain reference material (adapter or work contracts, HTTP API
tables, tool-access resolution tables, subchart documentation), upgrade
instructions, or extended descriptions of behaviors — a distinguishing behavior
is named in a line, and the document that owns it is linked.

The bounded start SHALL remain copy-pasteable without leaving the file, and
SHALL NOT grow back into the walkthrough: what a run looks like, which flags a
cluster's storage requires, what goes wrong first, and how to wire a route
belong to the Getting started page and SHALL NOT be restated here.

Content removed from the README SHALL be linked, not deleted: every reader who
followed it there before SHALL be able to reach it from the index in one hop.

#### Scenario: Reader wants the product overview

- **WHEN** a first-time reader opens `README.md`
- **THEN** the pitch, the list of CRD kinds, the install command, and one ask are
  all present without following a link
- **AND** the file is at most 150 lines

#### Scenario: A feature change adds reference detail

- **WHEN** a change documents a new contract endpoint, CRD field semantics, or
  subchart component
- **THEN** the text is written to the relevant `docs/` page, not to `README.md`
- **AND** `README.md` changes only if the kind list, pitch, or start commands
  changed

#### Scenario: Reader wants more than the quick start

- **WHEN** a reader finishes the install and wants the detail behind a behavior
- **THEN** the links-onward index names the published site first and the owning
  document for that content
- **AND** the README itself does not carry that detail

#### Scenario: A step of the walkthrough would be explained twice

- **WHEN** a change would add expectations, a failure mode, a storage flag or a
  routing exercise to the README's start section
- **THEN** it is written to the Getting started page instead, and the README's
  start section keeps only the commands

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
concepts to `docs/concepts.md`, contracts to `docs/contracts.md`, a bundle's
components or values to that bundle's own `docs/<bundle>.md` page, upgrade steps
to `CHANGELOG.md`, README only for pitch, kind list, demo, or install — and SHALL
state the README line budget as a number.

The routing rule SHALL name bundle pages by the bundle they document, so renaming
a subchart renames its page and the rule together. `docs/prometheus-bundle.md` is
the destination for the Alertmanager and Prometheus bundle's content;
`docs/vm-bundle.md` no longer exists.

It SHALL further record that `docs/` is a Jekyll source directory published by
GitHub Pages, and route the two kinds of site change separately: site
presentation (layouts, includes, navigation data, CSS, fonts) to the theme files
under `docs/`, and adopter-facing prose to a markdown page — never the reverse.
It SHALL record that the site's `--ao-*` palette is copied from
`console/ui/src/theme/theme.css`, so a token change is a two-file change.

**Where two documents cover one subject for two audiences, the routing rule SHALL
say which receives what, in one line each.** For the console that split is: what
the console is FOR — its views, what each answers, and the authentication
decision an operator makes — goes to the site page `docs/console-guide.md`, and
what the console IS — its endpoints, RBAC grant, values reference and internal
structure — goes to `docs/console.md`. A rule naming only one of a pair is how
the next writer picks at random.

It SHALL further record that a product screenshot published on the site is a
**build output**: the command that regenerates it SHALL be named, and the routing
rule SHALL state that a change to the console's UI is not complete until those
screenshots have been regenerated.

#### Scenario: Contributor finishes a behavior change

- **WHEN** a contributor consults `CLAUDE.md` for what to document after a change
- **THEN** the routing rule names the destination file for that kind of content
- **AND** the README line budget is stated numerically so overrun is checkable

#### Scenario: A chart value is added or changed

- **WHEN** a value is added to the parent chart, or an existing one changes
  meaning
- **THEN** the routing rule sends it to `docs/installation.md`, and a value
  belonging to a subchart to that subchart's page instead

#### Scenario: A subchart is renamed

- **WHEN** a bundle subchart is renamed
- **THEN** its `docs/` page is renamed with it and the routing rule names the new
  page, so no rule points at a file that no longer exists

#### Scenario: A contributor documents the console

- **WHEN** a change alters the console and its documentation must be updated
- **THEN** the routing rule names which of the two console documents receives it,
  and the contributor does not have to choose between two similar filenames

#### Scenario: The console's UI changes

- **WHEN** a change alters what a console view looks like
- **THEN** the routing rule names the command that regenerates the site's
  screenshots, and the change is not complete until it has been run

#### Scenario: Contributor changes the site

- **WHEN** a contributor wants to restyle the site or add a page to it
- **THEN** `CLAUDE.md` names the theme files for presentation and a markdown page
  for prose, and states that `docs/` is a published Jekyll source

#### Scenario: A palette token is changed in the console

- **WHEN** a contributor edits a `--ao-*` value in the console's theme
- **THEN** `CLAUDE.md` tells them the site's copied token block is the second
  half of that change

### Requirement: The site's reading order is one chain, declared in one place

The site's *Start here* group SHALL present its pages in the order a reader
follows them, and each page's what-next card SHALL point at the page that
follows it in that group. The order SHALL be: the landing page, the Introduction,
Getting started, the Console page, Installation.

Navigation order and the what-next chain SHALL agree. A page SHALL NOT send a
reader past an entry that sits between them in the navigation.

#### Scenario: A page is inserted into the reading order

- **WHEN** a new page is published into the *Start here* group
- **THEN** the navigation entry and the what-next cards on both sides are
  adjusted together, so no card skips the new page and no entry is unreachable
  from the page before it

#### Scenario: A reader follows the cards without the sidebar

- **WHEN** a reader starts at the landing page and follows only the what-next
  card on each page
- **THEN** they visit every page in the *Start here* group, in navigation order

