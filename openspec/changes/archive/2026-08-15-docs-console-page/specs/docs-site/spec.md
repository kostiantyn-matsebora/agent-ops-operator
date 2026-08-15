## MODIFIED Requirements

### Requirement: The site delivers the theme and the landing page only

The site's deliverables SHALL be the theme, `docs/index.md`,
`docs/introduction.md`, `docs/getting-started.md`, `docs/installation.md` and
`docs/console-guide.md`. The existing `docs/*.md` reference pages SHALL NOT be
edited for the site — no YAML front matter added, no headings changed, no links
rewritten — SHALL NOT appear in the site navigation, and SHALL NOT be treated as
published documentation by any site page, all of which link to them where they
live until a later change takes them onto the site.

A site page SHALL be free to declare a permalink that a reference page's own
filename resembles. `docs/console-guide.md` at `/console/` and the untreated
`docs/console.md` served verbatim at its own path are two different documents at
two different URLs, and neither SHALL be renamed or edited to accommodate the
other.

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

#### Scenario: A site page and a reference page name the same subject

- **WHEN** the site publishes a page about the console and an untreated
  reference page about the console is present in the same directory
- **THEN** both build, each is reachable at its own URL, and neither was renamed
  or edited to free a filename

#### Scenario: A site page is published

- **WHEN** a markdown file under `docs/` gains front matter and one navigation
  entry
- **THEN** it is built as a site page with the assigned layout, is reachable at
  its declared URL, and no layout, include or stylesheet was edited to publish it

### Requirement: Presentation beyond prose is a named component, not page markup

A page needing a form the prose elements do not provide SHALL name a component
class the theme styles, using the markdown engine's attribute syntax. It SHALL
NOT contain HTML markup, inline styles or script, and the component's rule SHALL
carry no words.

A component SHALL be general — usable by any page that needs that form — and
SHALL NOT be styling introduced for one page. Content SHALL NOT move into
`_includes/` or `_data/` to obtain a visual form; both are theme scope and hold
no product prose.

The site SHALL provide a **card grid** for a set of peer concepts, a **callout**
for a statement a page is built around, and a **tabbed panel set** for a series
of alternative views of one subject. The callout SHALL be visually distinct from
the plain blockquote, which sets a passage ASIDE: the two carry opposite emphasis
and SHALL NOT render alike.

The tabbed panel set SHALL be named on an ordinary markdown list, one list item
per panel, whose leading emphasised phrase is the tab's label and whose remaining
content is the panel. **Every word and every image SHALL live in the page.** The
theme SHALL supply only the tab strip, the panel geometry and the selection
behaviour.

**With scripting unavailable the component SHALL degrade to its own content** —
the labelled list, in order, with every panel visible. A page SHALL NOT depend on
the tab behaviour to make its content complete or comprehensible.

A component no page uses SHALL be removed rather than kept for a future one.

Where a component draws a kind's glyph it SHALL use the one the console draws
for that kind (`console/ui/src/graph/shapes.tsx`), copied on the same terms as
the palette and the mark — one-directional, so that changing a glyph is a
two-file change — and rendered so it follows the theme's colour rather than
carrying a baked-in one.

#### Scenario: A glyph must follow the theme

- **WHEN** a kind's glyph is rendered in either theme
- **THEN** it takes the theme's own colour, and no colour is baked into the
  copied artwork

#### Scenario: A page presents alternative views of one subject

- **WHEN** a page names the tabbed panel set on a markdown list
- **THEN** one tab is shown per list item, labelled with that item's leading
  emphasised phrase, and selecting a tab shows that item's content

#### Scenario: Scripting is unavailable

- **WHEN** a page carrying a tabbed panel set is read with scripting disabled
- **THEN** every panel is visible in document order with its label, and no
  content is hidden, truncated or replaced by a placeholder

#### Scenario: A tab is linked directly

- **WHEN** a reader arrives at a page with a fragment naming one of its tabs
- **THEN** that tab is the selected one on arrival

## ADDED Requirements

### Requirement: The Console page shows the console before it is installed

The site SHALL carry a Console page for a reader who has just met the console in
Getting started, or who has not installed anything at all. It SHALL be reachable
from the navigation in the same group as the landing page, and SHALL declare its
own URL.

The page SHALL cover, in this order:

- **what the console is** — a view of the whole install that is also a channel
  and also a signal source, stated in a few sentences;
- **a tour of its views**, as a tabbed panel set with one panel per view, each
  carrying a label, a one-line statement of the question that view answers, and
  a screenshot of it;
- **what it does for the reader**, as a set of peer capabilities;
- **authentication**, as its own section (covered by its own requirement);
- **what it cannot do** — that no write path to the Kubernetes API exists in the
  module.

The page SHALL carry decisions and their consequences. It SHALL NOT carry the
console's HTTP endpoints, its RBAC grant, or its values reference. Where such
detail is needed it SHALL be a link to the document that owns it.

#### Scenario: A reader who has installed nothing

- **WHEN** a visitor who has not installed the operator opens the Console page
- **THEN** they can see each view and read what question it answers, without
  running anything

#### Scenario: A reader arrives from Getting started

- **WHEN** a reader finishes Getting started and follows its what-next card
- **THEN** they arrive at the Console page, and it explains the screen the
  walkthrough left them looking at

#### Scenario: A contributor wants to document an endpoint on the page

- **WHEN** a section would need the console's endpoints, RBAC grant or full
  values list
- **THEN** the page links the document that owns it and the detail is not
  written a second time

#### Scenario: The page is current in the navigation

- **WHEN** the Console page is open
- **THEN** its sidebar entry is marked current, both visually and with
  `aria-current="page"`

### Requirement: The Console page states the console's authentication as a decision

The Console page SHALL carry authentication as its own section, written for an
operator deciding how to expose the console rather than for someone reading the
implementation. It SHALL state:

- **the shipped mode** — a shared token, where it comes from, that an
  unconfigured token authorizes nobody, and that it is indistinguishable from a
  wrong one;
- **the alternative** — that a deployment may declare an external authenticator
  in front, and what that declaration requires;
- **the interface to that authenticator** — the identity headers the console
  trusts, **enumerated completely and in preference order**, with the statement
  that an install must ensure the fronting proxy sets every one of them and lets
  no client-supplied copy through;
- **the consequence of forwarding no identity** — reads served, writes refused,
  and no identity invented.

The header list SHALL be verified against the console's own source rather than
copied from another document. A partial list SHALL NOT be published: the page's
purpose is to tell an operator what to allow-list, and a header omitted there is
a header left reaching the console from the client.

#### Scenario: An operator decides how to expose the console

- **WHEN** an operator reads the page before putting the console behind an
  ingress
- **THEN** both modes are stated with their consequences, and the values each
  requires are named

#### Scenario: An operator configures their own authenticator

- **WHEN** an operator puts a forward-auth proxy in front of the console
- **THEN** the page names every identity header the console trusts, in the order
  it prefers them, and states that all of them must be set by the proxy and none
  accepted from a client

#### Scenario: The console's trusted headers change

- **WHEN** the set of identity headers the console reads is changed in the
  source
- **THEN** the page is wrong until it is updated, and the change is not complete
  until it is

### Requirement: Product screenshots on the site are generated, not captured by hand

A screenshot published on the site SHALL be produced by a committed, repeatable
command rather than captured by hand. The command SHALL drive the application's
own built bundle against a fixture it also owns, and SHALL write the image files
the site references.

Screenshots SHALL ship one variant per theme, and the site SHALL show the variant
matching the reader's resolved theme.

A screenshot fixture SHALL contain no data from a real installation — no cluster
names, namespaces, hostnames or identities taken from one.

The generating command SHALL NOT run as part of the ordinary test suite, and the
produced images SHALL be committed, so that building the site never depends on
running it.

#### Scenario: The UI changes and the screenshots are stale

- **WHEN** the console's UI changes such that a published screenshot no longer
  shows it
- **THEN** re-running the committed command reproduces every screenshot, and no
  image is re-captured by hand

#### Scenario: A reader switches theme

- **WHEN** a reader changes the site's theme while a screenshot is on screen
- **THEN** the screenshot shown changes to the variant matching the new theme

#### Scenario: The site is built

- **WHEN** the site is built by GitHub Pages branch deploy
- **THEN** every referenced screenshot is present in the repository and no
  generation step runs

#### Scenario: A fixture is written

- **WHEN** the fixture behind the screenshots is authored or edited
- **THEN** it contains only invented names, and nothing identifying a real
  installation appears in any published image
