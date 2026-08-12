## MODIFIED Requirements

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
