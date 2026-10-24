## MODIFIED Requirements

### Requirement: The document routing rule is recorded for contributors

The repository's own CONTEXT SHALL state which document receives which kind of
update — concepts to `docs/concepts.md`, contracts to `docs/contracts.md`, an
integration's adopter-facing content to that integration's own page under
`docs/integrations/`, upgrade steps to `CHANGELOG.md`, README only for pitch,
kind list, demo, or install — and SHALL state the README line budget as a number.

That context is `CLAUDE.md` plus the topic files under `.claude/rules/` that it
indexes: one topic per file, with `CLAUDE.md` naming the routing so a reader
reaches it in one hop. Where this requirement says `CLAUDE.md`, the index and the
file it routes to are meant together — a rule stated twice is a rule that drifts.

**The routing rule SHALL name integration pages by the SYSTEM they document, not
by the subchart that packages it.** It previously named them by their bundle, so
that renaming a subchart renamed its page and the rule together. That was the
wrong half to bind: the rename it was written for — `docs/vm-bundle.md` to
`docs/prometheus-bundle.md` — happened because the page was named after one
sender rather than after the thing, and a page named for the system would have
survived it untouched, taking every reader's bookmark with it.

**Where a chart's content is GENERATED, the routing rule SHALL send it to the
generator rather than to a page.** A component inventory typed into a page is
routed nowhere useful — the destination is the marker the page declares, and the
command that fills it.

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
  belonging to a subchart to that integration's page instead

#### Scenario: A subchart is renamed

- **WHEN** a bundle subchart is renamed
- **THEN** the integration page it packages keeps its name, because the page is
  named for the system, and the routing rule points at a file that still exists

#### Scenario: A bundle renders a new object

- **WHEN** a contributor changes what a bundle's component renders
- **THEN** the routing rule sends them to regenerate that page's block, not to
  edit a table by hand

#### Scenario: A contributor documents the console

- **WHEN** a change alters the console and its documentation must be updated
- **THEN** the routing rule names which of the two console documents receives it,
  and the contributor does not have to choose between two similar filenames

#### Scenario: A contributor documents a security control

- **WHEN** a change alters what a control bounds, or the default it ships with
- **THEN** the routing rule names which of the two documents receives which half,
  and the contributor does not restate the default on the security page

#### Scenario: Security is inserted into the reading order

- **WHEN** the security page is published into the *Start here* group
- **THEN** it sits between the Console page and Installation in the navigation
- **AND** the Console page's what-next card points at it, and its own card points
  at Installation, so no card skips an entry

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
- **THEN** `CLAUDE.md` names the theme files for presentation and a markdown page
  for prose, and states that `docs/` is a published Jekyll source

#### Scenario: A palette token is changed in the console

- **WHEN** a contributor edits a `--ao-*` value in the console's theme
- **THEN** `CLAUDE.md` tells them the site's copied token block is the second
  half of that change
