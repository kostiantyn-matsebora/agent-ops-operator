## MODIFIED Requirements

### Requirement: The site delivers the theme and the landing page only

The site's deliverables SHALL be the theme, `docs/index.md`,
`docs/introduction.md` and `docs/getting-started.md`. The existing `docs/*.md`
reference pages SHALL NOT be edited for the site — no YAML front matter added,
no headings changed, no links rewritten — SHALL NOT appear in the site
navigation, and SHALL NOT be treated as published documentation by any site
page, all of which link to them where they live until a later change takes them
onto the site.

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

### Requirement: Getting started takes a reader from an empty cluster to a route of their own

The site SHALL carry a Getting started page for a reader who has decided to try
the operator. It SHALL be reachable from the navigation in the same group as the
landing page and the Introduction, and SHALL declare its own URL rather than
relying on a permalink style the site does not configure.

The page SHALL be followable top to bottom in one sitting and SHALL carry, in
order:

- **prerequisites** — what a cluster must already have before the first command,
  including the storage decision that selects an install flag, stated up front
  rather than encountered as a failure;
- **the install** — copy-pasteable, with what each command is for;
- **the first answer** — how to ask, and **what a successful run looks like**:
  the states the conversation passes through, the pod that appears and exits,
  where the transcript is read, and where the result is read;
- **what goes wrong first** — the common failures and the observable each one
  announces itself through;
- **one route the reader writes**, applied against what the install already
  created, demonstrating that a route's reach comes from the route and not from
  the agent, and ending with the teardown of what that step created;
- **where to go next**, naming the document that owns each onward topic.

The page SHALL carry the commands and the manifest the reader types, and SHALL
NOT carry reference material: no CRD field table, no HTTP endpoint table, no
values reference, and no explanation of options the walkthrough does not use.
Where such detail is needed it SHALL be a link to the document that owns it.

Every command, flag and manifest on the page SHALL be verified against the chart
as it ships. A claim that cannot be verified SHALL be removed from the page
rather than restated from another document.

#### Scenario: A reader follows the page on a fresh cluster

- **WHEN** a reader with a cluster and an LLM credential follows the page from
  the top
- **THEN** they reach an installed operator, an answered question, and one
  Pipeline they wrote themselves, without needing another document to complete a
  step

#### Scenario: A run produces nothing visible

- **WHEN** the reader asks a question and no answer appears
- **THEN** the page names what to look at, in what order, and what the common
  causes look like when observed there

#### Scenario: The reader learns where capability comes from

- **WHEN** the reader applies the route the page has them write and asks again
- **THEN** the outcome differs from the demo route's in the way the page
  predicted, and the page attributes the difference to the wiring rather than to
  the agent

#### Scenario: The exercise leaves nothing running

- **WHEN** the reader finishes the route exercise
- **THEN** the page has them remove what that step created and states the cost of
  leaving it in place

#### Scenario: A contributor wants to explain a field on the page

- **WHEN** a step would need a CRD field's other values, an endpoint's semantics
  or a values key's rules to be explained
- **THEN** the page links the document that owns it and the explanation is not
  written a second time

#### Scenario: The page is current in the navigation

- **WHEN** Getting started is open
- **THEN** its sidebar entry is marked current, both visually and with
  `aria-current="page"`

#### Scenario: A reader arrives at the landing page ready to install

- **WHEN** a visitor follows the landing page's paths onward looking for how to
  start
- **THEN** Getting started is named there as the walkthrough, and the path to
  installation does not leave the site for the README
