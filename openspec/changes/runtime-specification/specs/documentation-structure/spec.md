## MODIFIED Requirements

### Requirement: Reference material lives in docs/ pages split by audience

The repository SHALL provide these reference pages:

| Page | Owns |
|---|---|
| `docs/concepts.md` | CRD reference and capability resolution |
| `docs/contracts/<contract>.md` | one extension-point contract each: `runtime.md`, `channel-adapter.md`, `signal-adapter.md` |
| `docs/contracts.md` | the manager-owned surfaces: the aops MCP server contract, the activity contract, introspection, metrics, the HTTP API |
| `docs/<bundle>.md` | ONE PAGE PER BUNDLE SUBCHART |

Each page SHALL be reachable from the README's Documentation index in one
hop.

`docs/contracts.md` SHALL keep its URL. A reader's bookmark to the manager's
surfaces survives the split, and the moved contracts are linked from where
they were.

A page per contract rather than one shared page, for the same reason as a
page per bundle: the three extension points have disjoint audiences, and no
single adopter needs the whole of a shared page.

A page per bundle rather than one shared page: each subchart is optional, off
by default, and irrelevant to anyone not running it, so they have disjoint
audiences.

A new bundle then adds a file and an index line instead of growing a shared
one.

#### Scenario: Adapter author needs the contract

- **WHEN** an author implementing a channel or signal adapter follows the
  README's Documentation index
- **THEN** `docs/contracts/channel-adapter.md` or
  `docs/contracts/signal-adapter.md` contains the full
  long-poll/push/state/auth contract for that adapter kind, one hop from the
  index

#### Scenario: Runtime author needs the contract

- **WHEN** an author implementing an `AgentRuntime` image follows the
  README's Documentation index
- **THEN** `docs/contracts/runtime.md` contains the whole runtime
  specification, one hop from the index

#### Scenario: A manager-owned surface is looked up

- **WHEN** a reader looks up the aops MCP server contract, the activity
  contract or the manager's HTTP API
- **THEN** `docs/contracts.md` contains it, at the URL it had before the
  split

#### Scenario: A new bundle subchart ships

- **WHEN** a chart gains another optional bundle
- **THEN** its documentation is a new `docs/<bundle>.md` with one line in the
  README's Documentation index, and no existing page grows

#### Scenario: Reader needs CRD detail beyond the README table

- **WHEN** a reader wants the full description of a CRD kind or the
  merge/overwrite resolution rules for `toolsets` and `mcpConfigs`
- **THEN** `docs/concepts.md` contains it

### Requirement: The document routing rule is recorded for contributors

The repository's own CONTEXT SHALL state which document receives which kind
of update, and SHALL state the README line budget as a number:

| Content | Destination |
|---|---|
| concepts | `docs/concepts.md` |
| an extension-point contract (runtime, channel adapter, signal adapter) | its own page under `docs/contracts/` |
| a manager-owned surface (aops MCP server, activity, introspection, metrics, HTTP API) | `docs/contracts.md` |
| an integration's adopter-facing content | that integration's own page under `docs/integrations/` |
| upgrade steps | `CHANGELOG.md` |
| pitch, kind list, demo, install | `README.md` only |

That context is `CLAUDE.md` plus the topic files under `.claude/rules/` that
it indexes: one topic per file, with `CLAUDE.md` naming the routing so a
reader reaches it in one hop.

Where this requirement says `CLAUDE.md`, the index and the file it routes to
are meant together — a rule stated twice is a rule that drifts.

**The routing rule SHALL name integration pages by the SYSTEM they document,
not by the subchart that packages it.** It previously named them by their
bundle, so that renaming a subchart renamed its page and the rule together.

That was the wrong half to bind: the rename it was written for —
`docs/vm-bundle.md` to `docs/prometheus-bundle.md` — happened because the
page was named after one sender rather than after the thing.

A page named for the system would have survived it untouched, taking every
reader's bookmark with it.

**Where a chart's content is GENERATED, the routing rule SHALL send it to the
generator rather than to a page.**

A component inventory typed into a page is routed nowhere useful — the
destination is the marker the page declares, and the command that fills it.

It SHALL further record that `docs/` is a Jekyll source directory published
by GitHub Pages, and route the two kinds of site change separately:

- site presentation (layouts, includes, navigation data, CSS, fonts) to the
  theme files under `docs/`
- adopter-facing prose to a markdown page — never the reverse

It SHALL record that the site's `--ao-*` palette is copied from
`platform/console/ui/src/theme/theme.css`, so a token change is a two-file
change.

**Where two documents cover one subject for two audiences, the routing rule
SHALL say which receives what, in one line each.** For the console that split
is:

| Console document | Receives |
|---|---|
| `docs/console-guide.md` (site) | what the console is FOR — its views, what each answers, and the authentication decision an operator makes |
| `docs/console.md` (reference) | what the console IS — its endpoints, RBAC grant, values reference and internal structure |

A rule naming only one of a pair is how the next writer picks at random.

For security that split is:

| Security document | Receives |
|---|---|
| `docs/security.md` (site) | the threat, the posture a default install carries, what a control bounds and what is still open |
| `docs/installation.md` | the chart key that sets a control, its default and its YAML |

The two cover one subject on two axes — by threat and by key. The rule SHALL
state that the security page carries no values table, so a default is stated
in one place and cannot drift.

It SHALL further record that every product asset published on the site is a
**build output**, and SHALL name the command that regenerates **each kind**:
the screenshots of the console's views, and the landing page's recording of
the product working.

The rule SHALL state that a change to the console's UI is not complete until
both have been re-run, and SHALL name where each kind of asset is published,
so a contributor does not have to work out which pages a UI change made
stale.

A page's rule SHALL NOT be stated in two files. Where `docs/CLAUDE.md`
governs how a page reads, the routing rule SHALL route to it rather than
restating it.

#### Scenario: Contributor finishes a behavior change

- **WHEN** a contributor consults `CLAUDE.md` for what to document after a change
- **THEN** the routing rule names the destination file for that kind of content
- **AND** the README line budget is stated numerically so overrun is checkable

#### Scenario: A contract change is documented

- **WHEN** a change alters the runtime work contract, a channel or signal
  adapter contract, or a manager-owned surface
- **THEN** the routing rule sends an extension-point contract to its own page
  under `docs/contracts/`, and a manager-owned surface to `docs/contracts.md`

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
