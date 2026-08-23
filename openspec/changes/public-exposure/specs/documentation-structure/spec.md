## MODIFIED Requirements

### Requirement: README is a bounded one-page overview

`README.md` SHALL answer what a stranger asks in their first two minutes: **what
this is**, **whether it is real**, **how to try it**, and **where to go next**.
It SHALL contain the product pitch and architecture diagram, the licence and an
honest status line, **one install command that works without cloning the
repository**, and a links-onward index naming the published documentation site
first.

It SHALL NOT exceed 240 lines, and SHALL NOT contain reference material (adapter
or work contracts, HTTP API tables, tool-access resolution tables, subchart
documentation), upgrade instructions, or extended descriptions of behaviors — a
distinguishing behavior is named in a line, and the document that owns it is
linked.

**The README SHALL name the published site as the main source of information**,
prominently and not only inside the links-onward index. The README is the short
version; the site is the document it is short for.

**It SHALL cover what the landing page covers, more concisely, and SHALL NOT
restate it.** Covering is naming what the project is, what it is for, how it
works, what a reader declares and why it is built that way — the landing page's
own sections, each in a line or a short list. Restating is reproducing a site
page's DETAIL: the walkthrough, the installation decisions, the console tour and
the guides belong to the site in full, and a README that repeats them is a
second source of truth whose drift is invisible until a reader follows the wrong
one.

**A README that covers none of it is the opposite failure**, and is equally out
of conformance. A reader who cannot tell from this page what the project does,
what they would write, or why it is built this way has been given an index
rather than an overview.

**The two surfaces SHALL carry the shared story in different media**, because
neither medium works on the other surface. The site uses its presentation tabs
and console recordings; the README SHALL use a diagram the forge renders from
SOURCE TEXT, scaled to the reader's column and following their theme. Neither is
a copy of the other.

**A page-scale exported drawing SHALL NOT be the README's diagram.** It is
composed for a page, and a forge column shrinks it past legibility; it is linked
as a click-through instead.

**The diagram is the visual and the prose is the content.** Content the diagram
also carries SHALL still be written out, because a reader skims headings, and a
reader reaching an image through assistive technology has only its alt text.

The install command SHALL be runnable by someone who has not cloned anything. A
command naming a path inside the repository is not a start, it is a step that
silently assumes the previous one.

Content removed from the README SHALL be linked, not deleted: every reader who
followed it there before SHALL be able to reach it from the index in one hop.

#### Scenario: Reader wants the product overview

- **WHEN** a first-time reader opens `README.md`
- **THEN** the pitch, the licence, one runnable install command, and the links
  onward are all present without following a link
- **AND** what the project does, what a reader would declare, and why it is
  built that way are each covered without following a link
- **AND** the file is at most 240 lines

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

- **WHEN** a change would add to the README the DETAIL of something the site's
  landing, Introduction or Getting started page carries
- **THEN** it is linked rather than reproduced, because two copies drift and the
  reader cannot tell which is current
- **AND** naming the same subject in a line, which the landing page also names,
  is covering rather than restating and is what the README is for

#### Scenario: A reader wants to know which document is authoritative

- **WHEN** a first-time reader opens `README.md`
- **THEN** the published site is named as the main source of information before
  the reader reaches the links-onward index

#### Scenario: The diagram and the prose carry the same thing

- **WHEN** the README's diagram already carries a step, an example manifest or a
  design reason
- **THEN** the prose carries it too, because a reader skims headings and the
  diagram is not what they read first

#### Scenario: A diagram is chosen for the README

- **WHEN** the README needs to show the shape of the system
- **THEN** it is rendered by the forge from source text, so that it scales to the
  reader's column, follows their theme, and is reviewable in a diff
- **AND** a page-scale exported drawing is linked rather than embedded

#### Scenario: A landing page section is added or reworked

- **WHEN** the site's landing page gains a section, or reworks one
- **THEN** the README is considered in the same change, and either covers it in
  a line or a short list, or the change says why that section is the site's
  alone

#### Scenario: The start command would require a clone

- **WHEN** the install command references a path inside the repository
- **THEN** it is replaced by one that resolves a published artifact, and the
  README does not ship until that artifact exists

#### Scenario: A step of the walkthrough would be explained twice

- **WHEN** a change would add expectations, a failure mode, a storage flag or a
  routing exercise to the README's start section
- **THEN** it is written to the Getting started page instead, and the README's
  start section keeps only the command
