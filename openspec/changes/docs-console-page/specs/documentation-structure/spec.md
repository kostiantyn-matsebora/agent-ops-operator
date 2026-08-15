## MODIFIED Requirements

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

## ADDED Requirements

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
