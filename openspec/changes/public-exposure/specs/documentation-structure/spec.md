## MODIFIED Requirements

### Requirement: README is a bounded one-page overview

`README.md` SHALL answer what a stranger asks in their first two minutes: **what
this is**, **whether it is real**, **how to try it**, and **where to go next**.
It SHALL contain the product pitch and architecture diagram, the licence and an
honest status line, **one install command that works without cloning the
repository**, and a links-onward index naming the published documentation site
first.

It SHALL NOT exceed 150 lines, and SHALL NOT contain reference material (adapter
or work contracts, HTTP API tables, tool-access resolution tables, subchart
documentation), upgrade instructions, or extended descriptions of behaviors — a
distinguishing behavior is named in a line, and the document that owns it is
linked.

**The README SHALL NOT restate the site.** The published site owns the model,
the walkthrough, the console tour and the installation detail; a README that
repeats them is a second source of truth, and the drift between them is
invisible until a reader follows the wrong one.

The install command SHALL be runnable by someone who has not cloned anything. A
command naming a path inside the repository is not a start, it is a step that
silently assumes the previous one.

Content removed from the README SHALL be linked, not deleted: every reader who
followed it there before SHALL be able to reach it from the index in one hop.

#### Scenario: Reader wants the product overview

- **WHEN** a first-time reader opens `README.md`
- **THEN** the pitch, the licence, one runnable install command, and the links
  onward are all present without following a link
- **AND** the file is at most 150 lines

#### Scenario: A feature change adds reference detail

- **WHEN** a change documents a new contract endpoint, CRD field semantics, or
  subchart component
- **THEN** the text is written to the relevant `docs/` page, not to `README.md`
- **AND** `README.md` changes only if the pitch, the licence or the start
  command changed

#### Scenario: Reader wants more than the quick start

- **WHEN** a reader finishes the install and wants the detail behind a behavior
- **THEN** the links-onward index names the published site first and the owning
  document for that content
- **AND** the README itself does not carry that detail

#### Scenario: The site already says it

- **WHEN** a change would add to the README something the site's landing,
  Introduction or Getting started page already covers
- **THEN** it is linked rather than repeated, because two copies drift and the
  reader cannot tell which is current

#### Scenario: The start command would require a clone

- **WHEN** the install command references a path inside the repository
- **THEN** it is replaced by one that resolves a published artifact, and the
  README does not ship until that artifact exists

#### Scenario: A step of the walkthrough would be explained twice

- **WHEN** a change would add expectations, a failure mode, a storage flag or a
  routing exercise to the README's start section
- **THEN** it is written to the Getting started page instead, and the README's
  start section keeps only the command
