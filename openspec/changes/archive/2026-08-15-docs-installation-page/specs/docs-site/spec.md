## MODIFIED Requirements

### Requirement: The site delivers the theme and the landing page only

The site's deliverables SHALL be the theme, `docs/index.md`,
`docs/introduction.md`, `docs/getting-started.md` and `docs/installation.md`.
The existing `docs/*.md` reference pages SHALL NOT be edited for the site — no
YAML front matter added, no headings changed, no links rewritten — SHALL NOT
appear in the site navigation, and SHALL NOT be treated as published
documentation by any site page, all of which link to them where they live until
a later change takes them onto the site.

Because branch deploy builds the whole source directory, those pages ARE part of
the build. Carrying no YAML front matter, they are static files to Jekyll: they
SHALL be copied to the site verbatim, unconverted and unstyled, and SHALL remain
reachable by URL only. The theme SHALL provide the layout the build assigns a
page that gains front matter without naming one; that layout carries no feature
beyond a title and the page content, and publishing a further page SHALL NOT
require editing it.

#### Scenario: A reference page is built

- **WHEN** the site is built with the untreated `docs/*.md` pages present
- **THEN** the build succeeds and each page is copied verbatim, unconverted
- **AND** no reference page has been modified in the repository
- **AND** nothing on the site links to one as though it were a site page

#### Scenario: A reader looks for the reference documentation

- **WHEN** a visitor on a site page follows a path to the CRD reference
- **THEN** the link goes to the page where it lives today
- **AND** no sidebar entry claims it as a site page

#### Scenario: A contributor is tempted to fix a page for the site

- **WHEN** a reference page would need front matter, a heading change or a
  rewritten link to look right on the site
- **THEN** the page is left alone and the work is deferred to the change that
  publishes the reference pages

#### Scenario: A site page is published

- **WHEN** a markdown file under `docs/` gains front matter and one navigation
  entry
- **THEN** it is built as a site page with the assigned layout, is reachable at
  its declared URL, and no layout, include or stylesheet was edited to publish it

## ADDED Requirements

### Requirement: The Installation page gets an operator to a real install

The site SHALL carry an Installation page for an operator deploying agent-ops
for real, as distinct from the demo the Getting started page installs. The two
SHALL each say which they are in their opening lines, and SHALL link to each
other.

The page SHALL cover, in this order:

- **the decisions that are expensive to reverse**, before the install rather
  than inside it — storage and whether conversations keep their context, the
  RBAC posture the agent inherits, and who owns the CRDs;
- **the install** — namespace, credential, the command, and how to tell the
  release is healthy;
- **enabling a bundle** — what a bundle is, the flag each takes, and a link to
  the page that owns its values;
- **the parent chart's values, grouped by the decision they serve**;
- **wiring one route**, because an install with no Pipeline claiming a source
  drops every signal and answers nothing;
- **upgrade and uninstall** — where migration steps live, and what survives.

#### Scenario: An operator installs for real

- **WHEN** an operator who has run the demo follows the Installation page
- **THEN** they finish with a release installed on their own terms, one bundle
  enabled, and one route wired

#### Scenario: The expensive decisions are met before they are paid for

- **WHEN** an operator reaches the install command
- **THEN** storage, RBAC posture and CRD ownership have already been stated as
  decisions, not discovered later as symptoms

#### Scenario: A fresh install answers nothing

- **WHEN** an operator installs the chart and enables a bundle but wires no route
- **THEN** the page has already told them signals are dropped until a Ready
  Pipeline claims the source, and shown the smallest route that fixes it

### Requirement: Every command is given for both platforms, as tabs

A command block on a site page SHALL be given in a **PowerShell** and a
**Linux/macOS** form, so a reader on Windows never has to translate one. Where
the two are identical, both SHALL still be shown.

The page SHALL carry no tab markup. It writes two adjacent fenced blocks whose
LANGUAGES name the shell, and the theme pairs them into one tabbed widget — the
same division by which a page names a card class and the theme draws the card.

Choosing a platform SHALL apply to every block on the page at once and SHALL
persist across pages, since a reader's platform does not change between them.
The initial choice SHALL be taken from the browser.

Without JavaScript both blocks SHALL simply render in sequence, each labelled by
its own syntax highlighting. The commands are still present, which is the part
that matters.

#### Scenario: A reader on Windows

- **WHEN** a reader whose browser reports Windows opens a page with commands
- **THEN** the Windows form is shown first, and choosing it once holds for every
  block on that page and on the next page they visit

#### Scenario: The forms differ

- **WHEN** a command uses a line continuation or shell-specific quoting
- **THEN** each tab carries the form that actually runs on that platform, not a
  copy of the other

#### Scenario: JavaScript is unavailable

- **WHEN** the page is read with scripting disabled
- **THEN** both command blocks are visible in sequence and neither is hidden

### Requirement: The Installation page owns the parent chart's values, and only those

The parent chart's values SHALL have exactly one home, and it SHALL be this
page. Values belonging to a subchart SHALL stay with that subchart's page, and
the console's with the console's page. Neither SHALL restate the other.

The page SHALL group values by the DECISION they serve rather than enumerate
them, and SHALL name the exhaustive list as a command rather than reproducing
it. A hand-copied table of every value SHALL NOT be maintained.

#### Scenario: An operator needs a value the page does not name

- **WHEN** a value is not among those the page groups
- **THEN** the page names the command that lists every value, and the document
  that owns that value's semantics

#### Scenario: A bundle's values are wanted

- **WHEN** an operator wants to configure a bundle beyond enabling it
- **THEN** the Installation page links to that bundle's page rather than
  documenting its values

#### Scenario: A value is added to the chart

- **WHEN** a new value is added to the parent chart
- **THEN** the page needs an edit only if it changes a decision an operator
  makes, because the page carries decisions rather than an inventory
