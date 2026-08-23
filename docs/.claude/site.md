## The site's shell and mechanics

`CLAUDE.md` beside this file governs how a PAGE reads — its structure, the
command tabs, the components it may name, the table rules and the pre-flight
lint. What is here is the directory around those pages.

**Reference pages** — `concepts.md` (CRDs plus capability resolution),
`contracts.md` (work and adapter contracts plus HTTP API), and one page per
bundle subchart.

**The published site's Jekyll source** — `_config.yml`, `_layouts/`,
`_includes/`, `_data/nav.yml`, `assets/` (css, js, vendored Red Hat fonts, the
exported diagrams, the console screenshots and the landing recording).

### What each page owes, beyond the table

- **`index.md`'s order IS written in the page**, and `home.html` places one
  `<h1>` and nothing else.
  - **The layout used to SPLIT the rendered content at its first `<h2>`** and
    drop a row of stat tiles into the seam. Both are gone, along with `lede:`,
    `eyebrows:` and `stats:` — so a section now renders where the page wrote it,
    and the page can put its own chips between its sentence and its panel set.
  - **There is no diagram block in the layout and no `diagram:` front matter.**
    The strip is page content, which is what lets the presentation's beats and
    the manifest be the page's own words.
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

### The build, not a page

- **The FRAMES are the reproducible artifact, not the MP4.** Review the beat
  script and the frames. Nobody is to make the encoder deterministic.
- **`_layouts/page.html` is what `introduction.md` uses**, via
  `jekyll-default-layout`: front matter makes a file a page, and a missing
  layout then DOES fail the build.

### `diagrams/`

**Holds the drawio SOURCE plus `export.py`.**

**Run that, never the exporter by hand.** It writes BOTH theme variants of the
one exported page (two SVGs) and repaints the dark one's icon ink, which drawio
cannot do because the icons are embedded images.

TWO drawio pages, and only one is exported:

| Page | Is |
|---|---|
| `site` | the whole argument behind the landing page's full-size link |
| `why` | the standalone poster — rendered on demand, never committed |

**There WAS a third**, `landing`, compressed to 950px for the landing page's own
strip, and it is deleted along with its two exports.

- **The landing page carries a PRESENTATION now** — the same argument built one
  beat at a time, in real text, scaled to whatever width it is given.
- **The still had a ceiling the presentation does not.** 950 because the content
  column is 720 and there is no breakout, so making it fit meant REMOVING
  ELEMENTS rather than shrinking type, and every attempt to add detail had to
  pay for it that way.
- **Keeping it as well would be two statements of one model** that have to
  agree, one of which nobody would remember to re-export.

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
