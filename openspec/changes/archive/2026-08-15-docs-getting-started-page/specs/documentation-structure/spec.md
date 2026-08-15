## MODIFIED Requirements

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
