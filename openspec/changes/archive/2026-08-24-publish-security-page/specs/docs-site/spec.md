## MODIFIED Requirements

### Requirement: The site delivers the theme and the landing page only

The site's deliverables SHALL be the theme, `docs/index.md`,
`docs/introduction.md`, `docs/getting-started.md`, `docs/console-guide.md`,
`docs/security.md`, `docs/installation.md` and the guides under `docs/guides/`. A page is a
deliverable exactly when it carries YAML front matter; the remaining `docs/*.md`
reference pages carry none and SHALL NOT be edited for the site — no YAML front matter added, no headings changed, no links
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

#### Scenario: A site page links an architecture decision record

- **WHEN** a site page references reasoning recorded under `docs/adr/`
- **THEN** the record is linked where it lives, carrying no front matter and no
  navigation entry
- **AND** the record is not edited to be linked from the site
