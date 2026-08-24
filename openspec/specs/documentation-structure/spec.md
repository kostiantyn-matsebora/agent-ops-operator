# documentation-structure Specification

## Purpose

WHERE each kind of documentation lives, and the rules that keep it there.

One page per audience, a bounded README that is the landing page's counterpart on
the forge rather than a third document, migration guides in the changelog, and a
single declared reading order so the site is one chain rather than a pile of
pages.

The last requirement is the one with teeth outside this repository: a shipped
example carries a documented PLACEHOLDER and never a real value, because
documentation, chart values and fixtures are copied by adopters and read by
strangers.

## Requirements

### Requirement: README is a bounded one-page overview

`README.md` IS THE LANDING PAGE'S COUNTERPART ON THE FORGE. It carries what the
landing page carries, more concisely, and says the site is the source — the two
are one story for two audiences, and the forge audience is the larger one, so
sending a stranger away to learn what the project IS is a gap rather than a
boundary.

Its sections SHALL track the landing page's: the claims strip, the flow diagram
followed by how it works, what you write as a commented `Pipeline`, the console
and its recording, when it runs / why it is built this way / pluggable at three
seams, the comparison table, a one-line-per-kind CRD table, **a bounded start** —
the credential, the install command, one ask, and a link naming the site's
Getting started page as the walkthrough — and a links-onward index naming the
site first, with short contributing and status sections.

**THE SECTION LIST IS THE BOUND, and the line budget FOLLOWS it: at most 215
lines.** A section added to the landing page moves that number; a paragraph that
wandered in does not.

It SHALL NOT contain reference material (adapter or work contracts, HTTP API
tables, tool-access resolution tables, subchart documentation), upgrade
instructions, or extended descriptions of behaviors — a distinguishing behavior
is named in a line, and the document that owns it is linked. RESTATING a site
page is the failure; naming the same subject in a line is COVERING, and the
difference is where the DETAIL lives.

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
- **AND** the file is at most 215 lines

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

The repository's own CONTEXT SHALL state which document receives which kind of
update — concepts to `docs/concepts.md`, contracts to `docs/contracts.md`, a
bundle's components or values to that bundle's own `docs/<bundle>.md` page,
upgrade steps to `CHANGELOG.md`, README only for pitch, kind list, demo, or
install — and SHALL state the README line budget as a number.

That context is `CLAUDE.md` plus the topic files under `.claude/rules/` that it
indexes: one topic per file, with `CLAUDE.md` naming the routing so a reader
reaches it in one hop. Where this requirement says `CLAUDE.md`, the index and the
file it routes to are meant together — a rule stated twice is a rule that drifts.

The routing rule SHALL name bundle pages by the bundle they document, so renaming
a subchart renames its page and the rule together. `docs/prometheus-bundle.md` is
the destination for the Alertmanager and Prometheus bundle's content;
`docs/vm-bundle.md` no longer exists.

It SHALL further record that `docs/` is a Jekyll source directory published by
GitHub Pages, and route the two kinds of site change separately: site
presentation (layouts, includes, navigation data, CSS, fonts) to the theme files
under `docs/`, and adopter-facing prose to a markdown page — never the reverse.
It SHALL record that the site's `--ao-*` palette is copied from
`platform/console/ui/src/theme/theme.css`, so a token change is a two-file change.

**Where two documents cover one subject for two audiences, the routing rule SHALL
say which receives what, in one line each.** For the console that split is: what
the console is FOR — its views, what each answers, and the authentication
decision an operator makes — goes to the site page `docs/console-guide.md`, and
what the console IS — its endpoints, RBAC grant, values reference and internal
structure — goes to `docs/console.md`. A rule naming only one of a pair is how
the next writer picks at random.

For security that split is: the threat, the posture a default install carries,
what a control bounds and what is still open go to the site page
`docs/security.md`, and the chart key that sets a control, its default and its
YAML go to `docs/installation.md`. The two cover one subject on two axes — by
threat and by key — and the rule SHALL state that the security page carries no
values table, so a default is stated in one place and cannot drift.

It SHALL further record that every product asset published on the site is a
**build output**, and SHALL name the command that regenerates **each kind**: the
screenshots of the console's views, and the landing page's recording of the
product working. The rule SHALL state that a change to the console's UI is not
complete until both have been re-run, and SHALL name where each kind of asset is
published, so a contributor does not have to work out which pages a UI change
made stale.

A page's rule SHALL NOT be stated in two files. Where `docs/CLAUDE.md` governs
how a page reads, the routing rule SHALL route to it rather than restating it.

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
- **THEN** the routing rule names both commands that regenerate the site's
  product assets — the screenshots and the recording — and the change is not
  complete until both have been run

#### Scenario: A published asset's home is looked up

- **WHEN** a contributor asks which pages a console UI change made stale
- **THEN** the routing rule names where each kind of generated asset is
  published, and no page is discovered stale by a reader instead

#### Scenario: Contributor changes the site

- **WHEN** a contributor wants to restyle the site or add a page to it
- **THEN** the routing rule names the theme files for presentation and a markdown
  page for prose, and states that `docs/` is a published Jekyll source

#### Scenario: A palette token is changed in the console

- **WHEN** a contributor edits a `--ao-*` value in the console's theme
- **THEN** the routing rule tells them the site's copied token block is the
  second half of that change

#### Scenario: A contributor documents a security control

- **WHEN** a change alters what a control bounds, or the default it ships with
- **THEN** the routing rule names which of the two documents receives which half,
  and the contributor does not restate the default on the security page

#### Scenario: Security is inserted into the reading order

- **WHEN** the security page is published into the *Start here* group
- **THEN** it sits between the Console page and Installation in the navigation
- **AND** the Console page's what-next card points at it, and its own card points
  at Installation, so no card skips an entry

### Requirement: The site's reading order is one chain, declared in one place

The site's *Start here* group SHALL present its pages in the order a reader
follows them, and each page's what-next card SHALL point at the page that
follows it in that group. The order SHALL be: the landing page, the Introduction,
Getting started, the Console page, Security, Installation.

Security sits before Installation because it is an evaluation gate rather than an
install step: the reader deciding whether to run model output in their cluster
decides before they choose values, not after.

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

### Requirement: A shipped example carries a placeholder, never a real value

Documentation, chart values, sample manifests and test fixtures are copied by
adopters and read by strangers. Every identifier in them — a hostname, a
repository URL, a chat or group identifier, an address literal, a person's name
— SHALL be a documented placeholder from the publication guard's allowlist, and
never a value from a real deployment or a real person.

This holds even when the real value is more convincing. An example that works
when pasted unchanged is a leak that looks like a courtesy.

Where a placeholder already exists for a kind of value, THAT placeholder SHALL
be used rather than a second one invented beside it. Two placeholders for one
kind is how a real value gets chosen as the "better" example.

#### Scenario: A new field is documented

- **WHEN** a page, a values file or a sample gains an identifier
- **THEN** the value shown is a documented placeholder
- **AND** the publication guard permits it by name

#### Scenario: A real value is more convenient

- **WHEN** an author has a working value to hand and no placeholder exists for
  that kind
- **THEN** a placeholder is added to the allowlist first, and the real value is
  not committed in the meantime

#### Scenario: A fixture needs a person

- **WHEN** a test needs a sender, an author or an operator
- **THEN** the name states the ROLE the fixture exercises rather than naming
  anyone, so the test reads as what it tests

#### Scenario: The rule is enforced rather than remembered

- **WHEN** any change adds an identifier outside the allowlist
- **THEN** CI fails on it, in the tree and in the commit message alike

### Requirement: A published spec describes the system as it is

`openspec/specs/` is published with the repository and read as the current
contract. Every requirement in it SHALL be true of the code at the moment it is
published.

Where a spec and the code disagree, the CODE is authoritative and the spec is
the defect — this is a documentation rule, and correcting a spec SHALL never
change behaviour to suit it.

A capability SHALL carry a written `Purpose`. The scaffolding tool's placeholder
is not one: a capability whose purpose nobody wrote is a capability whose
boundary nobody has had to justify.

Retired vocabulary — a removed CRD field, a withdrawn rule, a superseded command
— SHALL NOT appear in a spec as a current claim. It MAY appear as an explicit
record that the thing was removed, and the difference between those two is the
whole of this requirement.

#### Scenario: A spec asserts a field the API does not have

- **WHEN** a published spec names a CRD field that the API types do not carry
- **THEN** the spec is corrected, and the code is not changed to add the field

#### Scenario: Two specs disagree

- **WHEN** two published specs make incompatible claims about one behaviour
- **THEN** both are corrected against the code, and the one that was wrong says
  what replaced its claim rather than dropping it silently

#### Scenario: The audit finds the code wrong

- **WHEN** a requirement is true, desirable, and NOT satisfied by the code
- **THEN** it is recorded as a defect and raised as its own change
- **AND** the requirement is NOT weakened to match the behaviour, because that
  turns a bug into a decision nobody made

#### Scenario: A capability has no written purpose

- **WHEN** a capability's `Purpose` is the scaffolding placeholder
- **THEN** a purpose is written, or the capability is merged into the one that
  absorbed it

#### Scenario: Retired vocabulary returns

- **WHEN** a change reintroduces a removed field name, rule or command as a
  current claim in any spec
- **THEN** CI fails, naming the term and the file
