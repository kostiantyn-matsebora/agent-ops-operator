## The site (`docs/`)

**Reference pages** — `concepts.md` (CRDs plus capability resolution),
`contracts.md` (work and adapter contracts plus HTTP API), and one page per
bundle subchart.

**The published site's Jekyll source** — `_config.yml`, `_layouts/`,
`_includes/`, `_data/nav.yml`, `assets/` (css, js, vendored Red Hat fonts, the
exported diagrams, the console screenshots and the landing recording).

**`docs/CLAUDE.md` owns how a page is written and built** — structure over
prose, the command tabs, the components a page may name, the table rules and the
pre-flight lint. `.claude/` routes what goes where, that one governs how it
reads.

**`docs/CHANGELOG.md`** holds every chart-version migration guide.

- **The ONLY place upgrade steps live.**
- **Newest first**, Keep a Changelog 1.1.0 format.
- **TEN versions.** Older ones move VERBATIM to
  `docs/changelog/CHANGELOG-<range>.md`, linked from its foot.

### The site's pages

| Page | Owns |
|---|---|
| `index.md` | the hero, what it plugs into, ONE tab strip (the recording of one incident, the diagram, the Pipeline manifest as copyable page text), then the stat tiles, then the sections |
| `introduction.md` | the adopter's orientation — the model, the seams, and NO reference detail |
| `getting-started.md` | THE walkthrough — install and a first answer IN THE CONSOLE |
| `installation.md` | THE REAL install, and the only home the PARENT chart's values have |
| `console-guide.md` | permalink `/console/` — what the console is FOR |

- **`index.md`'s order is not written in the page.** `home.html` SPLITS the
  rendered content at its first `<h2>` and drops the tiles in the seam, so the
  page states its words in order and says nothing about placement.
  - **There is no diagram block in the layout and no `diagram:` front matter.**
    The strip is page content, which is what lets the alt text and the manifest
    be the page's own words.
- **`introduction.md` carries no sentence a field rename would break.** That
  belongs in `concepts.md`.
  - **It is TWO SECTIONS — understand the concepts, follow the guides — and
    stays that way.** Anything else is a guide or a reference page. The
    signal-to-answer lifecycle is the first guide owed, not a section here.
- **`getting-started.md` ends where a getting-started page should:** at
  something working. Wiring is the NEXT card, not its last section — and that
  card owns every expectation, flag and failure mode.
  - **Its test is "would the reader TYPE it or READ it".** What they type is on
    the page, what they read ABOUT is a link. That is why README keeps only the
    commands.
- **`installation.md` puts decisions before commands**, values grouped by the
  decision they serve, bundle values left to their own pages.
- **`console-guide.md` covers the six views and the question each answers**,
  plus the authentication decision an operator makes before exposing it.
  Endpoints, the RBAC grant and the values list stay in the untouched reference
  `docs/console.md`, which keeps its own name and its own URL.

**How a page reads, how its assets are BUILT, and how Pages serves this
directory are `docs/CLAUDE.md`'s** — the build-output rule for the screenshots
and the recording, the recording's silence, and the reference pages' static-file
status are stated there and not restated here.

Two that stay, because they are about the SHELL rather than a page:

- **The FRAMES are the reproducible artifact, not the MP4.** Review the beat
  script and the frames. Nobody is to make the encoder deterministic.
- **`_layouts/page.html` is what `introduction.md` uses**, via
  `jekyll-default-layout`: front matter makes a file a page, and a missing
  layout then DOES fail the build.

### `diagrams/`

**Holds the drawio SOURCE plus `export.py`.**

**Run that, never the exporter by hand.** It writes BOTH theme variants of BOTH
site pages (four SVGs) and repaints the dark ones' icon ink, which drawio cannot
do because the icons are embedded images.

THREE drawio pages, and only two are exported:

| Page | Is |
|---|---|
| `landing` | the poster's own COMPOSITION, compressed to 950px |
| `site` | the full argument behind its full-size link |
| `why` | the standalone poster — rendered on demand, never committed |

**950 BECAUSE the content column is 720 and there is no breakout.** Displayed
size is type over canvas, so making it fit means REMOVING ELEMENTS and
tightening layout, never shrinking type. Adding detail back is what makes it
unreadable.

### The site shell

**Astro Starlight's geometry**, read off that site's own stylesheet and verified
against it live: BOTH rails 18.75rem, text 45rem FIXED, and the leftover SPLIT
EVENLY between the left gutter and the right container.

The rail keeps its width at that container's left, so its half is empty space
PAST the rail, never a fatter rail.

**Reproduced with an explicit `--ao-leftover`**, because `minmax(base, 1fr)` on
two tracks does NOT share a remainder: `fr` sizes against the whole free space,
so both tracks come out the same WIDTH. That is what once gave an 810px rail
beside a 66px gutter.

**Body type is 17px on purpose.** Red Hat Text is narrow, and at 16px that 45rem
column reads 99 characters.

### The DEMO WIRES THE CONSOLE

Where k8s-bundle renders a route, that route claims the console's source and
binds it as a channel, from `global.agentops.console` — a subchart reads no
other parent scope, and helm cannot derive a value from a value.

- **Those names DUPLICATE `console.signalSourceName` / `channelName`**, so the
  render FAILS when they disagree.
- **Scoped to demo mode**, because `console.enabled: false` is pinned to remove
  every console object with ONE value.
- **The claim rides the EXISTING route.** A second claimant makes every
  unaddressed console message ambiguous.
