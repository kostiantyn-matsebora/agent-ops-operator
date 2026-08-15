## Why

Getting started ends at a first answer **in the console**, and then hands the
reader nothing about the thing they are looking at. Its Next card points past it
to concepts, and its "where to go next" list links `docs/console.md` — a 636-line
reference page with no front matter, served as raw markdown, that opens with
`servedBy` and adapter identities. An adopter one screen into the product meets
implementation detail before they know what the screen does.

Nobody has ever seen the console before installing it, either. The site shows one
diagram and no product. A tool whose whole argument is "you can watch this
happen" has no picture of it happening anywhere a prospective adopter looks.

And the console's authentication — the one thing an operator must decide before
exposing it — is documented only inside that reference page, under
*Trust boundary*, in prose written for someone who already runs it.

## What Changes

- **A new site page, `docs/console-guide.md`, published at `/console/`.** The
  reference page keeps its filename and is not touched (see Impact).
  - **what it is** — a browser view of the whole install that is *also* a channel
    and *also* a signal source, in three sentences;
  - **a tour, as tabbed screenshots** — one tab per view, each with a label, a
    one-line description and a real screenshot;
  - **what it does for you** — cards, the same component the other pages use;
  - **authentication, as its own chapter** — how the shipped token mode works,
    and how to put your own authenticator in front, naming the exact headers the
    console trusts;
  - **what it cannot do** — no write path to the Kubernetes API.
- **A tabbed-screenshot component in the theme** — `{: .ao-tabs}` on an ordinary
  markdown list, in the same shape as `{: .ao-cards}`: the page holds every word,
  the theme holds the geometry. Without JavaScript it renders as a labelled list
  of screenshots, which is the whole content.
- **Screenshots are CAPTURED BY A SCRIPT, never by hand.** A Playwright spec
  serves the built console bundle against a curated fixture and writes both theme
  variants per view. A hand-captured PNG rots silently and leaks whatever cluster
  it came from.
- **The reading order becomes *Getting started → The console → Installation*.**
  Getting started's Next card points at the console page, the console page's
  points at Installation, and one navigation entry goes between them.

Explicitly NOT in this change:

- **No console behaviour change.** Not one line of `console/` ships differently.
  If the page needs a code change to be true, the page is wrong.
- **No console values reference.** `console.md`'s Values section owns that. The
  authentication chapter names the values a decision needs and links onward.
- **Not a replacement for the reference page.** The adopter page says what the
  console is for. `docs/console.md` keeps the endpoint tables, the RBAC grant
  and the full values list, is **not renamed** and is **not edited** — it has
  uncommitted work in it from another change, and a rename to buy a filename is
  churn with a merge conflict attached.
- **No second tab group, no lightbox, no carousel.** One tab set on one page.

## Capabilities

### New Capabilities

None. The published site is one capability and this page belongs to it.

### Modified Capabilities

- `docs-site`: the deliverables grow by the Console page and the screenshot
  assets, and the site gains requirements for what that page must show an
  adopter, for the tabbed component's no-JavaScript behaviour, and for
  screenshots being generated rather than captured.
- `documentation-structure`: the routing gains a split between the two documents
  that will both be called "the console page" — what the console is FOR goes to
  the site page, what it IS goes to the reference page — and the rule that a
  screenshot on the site is a build output.
- `console-application`: the console's own spec gains the requirement that its
  authentication contract is stated for an adopter, so the header set the console
  trusts is documented where an operator exposing it will look.

## Impact

- `docs/console-guide.md` (new site page, permalink `/console/`).
  `docs/console.md` is untouched — it carries no front matter, so Jekyll serves
  it verbatim at its own URL and the two never collide.
- `docs/_data/nav.yml` (one entry, between *Getting started* and *Installation*),
  `docs/getting-started.md` (Next card retargeted, closing list de-duplicated),
  `docs/index.md` (the path onward).
- `docs/assets/css/agentops.css` (the tab component), `docs/assets/js/tabs.js`
  (new), `docs/_layouts/default.html` (load it, gated like `toc.js`).
- `docs/assets/img/console/*.png` — generated screenshots, two per view.
- `console/ui/screenshots/` (new) + one `package.json` script. No `console/*.go`
  change, no chart change, no CRDs.
- `README.md`, `CLAUDE.md` — the routing rows and the `docs/` map lines.
- **Ordering:** the Installation page has already landed and holds Getting
  started's Next card. This change moves that card to the console page and gives
  the console page's card to Installation, so the chain grows a link rather than
  jumping one.
