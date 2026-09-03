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

**Holds the drawio SOURCE plus THREE generators, and they are not
interchangeable.**

| Source | Writes | For |
|---|---|---|
| `agent-ops.drawio` + `export.py` | `assets/img/agent-ops-{light,dark}.svg` | the page-scale picture the landing page links |
| `readme-flow.py` | `assets/img/readme-flow-{light,dark}.svg` | the README column |
| `threat-model.py` | `assets/img/security/threat-model-{light,dark}.svg` | the security page's trust boundaries |

- **The GUIDE, INTEGRATION, RUNTIME and SECURITY diagrams are not here.** They
  are specs in `.github/scripts/docs_diagrams.py` (`DIAGRAMS`,
  `INTEGRATION_DIAGRAMS`, `RUNTIME_DIAGRAMS`, `SECURITY_DIAGRAMS`, each with
  its `dir`), rendered by `docs-generate.py`, and CI
  fails on a stale one. The security page's four smaller illustrations are in
  that same dict, told apart by a `dir` key — a guide entry states none and
  lands in `assets/img/guides/`.
- **`threat-model.py` is NOT run by CI**, exactly as `readme-flow.py` is not.
  Both are run by hand and their output committed, so editing one and forgetting
  to run it ships a stale drawing silently.
- **AUTHOR A DRAWING AT THE FRAME'S WIDTH.** `.ao-diagram` is
  `min-width: 42rem` inside a column that is 616px at 1280 and 720px above 1440,
  so a wider canvas is scaled DOWN and its labels go with it. A 980px threat
  model rendered every label at 0.69 and was unreadable. 760px renders at
  0.88–0.95.
  - **A PORTRAIT drawing is the other failure.** The same picture at 672×872 was
    perfectly readable and filled the whole screen. Landscape, one glance.
- **A diagram followed by a TABLE needs a sentence between them.** The theme
  gives the diagram's paragraph `margin: 0`, so a table butts straight against
  the frame.

**Run the drawio exporter, never the exporter by hand.** It writes BOTH theme variants of the
one exported page (two SVGs) and repaints the dark one's icon ink, which drawio
cannot do because the icons are embedded images.

TWO drawio pages, and only one is exported:

| Page | Is |
|---|---|
| `site` | the whole argument behind the landing page's full-size link |
| `why` | the standalone poster — rendered on demand, never committed |

**There WAS a third**, `landing`, compressed to 950px for the landing page's own
strip, and it is deleted along with its two exports.

- **The landing page carries a DRAWING now** — the same argument built one beat
  at a time, in real text, scaled to whatever width it is given.
  - **IT IS ONE FIGURE, AND THE FIGURE IS THE CONTROL.** It had a transport —
    play button, beat counter, ten scrub dots, progress bar, and a box showing
    each beat's manifest lines, in two bordered boxes under the picture. That
    was MORE MACHINERY THAN THE THING IT EXPLAINED. Clicking the drawing pauses
    it, and nothing else sits below it.
  - **A REDUCED-MOTION READER GETS THE SAME FIGURE, STILL FROM THE START**,
    with a `· press to play` caption cue and a matching `aria-label` as the
    only sign it can be pressed. It never gets a second control and never a
    second copy of the beats reinserted below it — the reduced-motion branch
    used to reinsert that list, back when a rail's play button sat beside it
    as the still's only control. Once the rail was deleted, the reinsertion
    became a duplicate account of the whole model sitting under a picture
    that already showed it, with nothing marking either as clickable.
  - **The caption is IN the drawing**, in a lane reserved to the right of it,
    vertically tracking whatever the beat lights. Placing it beside the anchor
    was tried over ten candidate positions and collided on nearly every beat —
    six boxes, a chip row and seven connectors leave no clear space. Reserving
    the room is the fix a cost function cannot be, and it costs picture width.
- **The still had a ceiling the presentation does not.** 950 because the content
  column is 720 and there is no breakout, so making it fit meant REMOVING
  ELEMENTS rather than shrinking type, and every attempt to add detail had to
  pay for it that way.
- **Keeping it as well would be two statements of one model** that have to
  agree, one of which nobody would remember to re-export.

### The site shell

**Astro Starlight's geometry**, read off that site's own stylesheet and verified
against it live: BOTH rails 18.75rem, and the leftover SPLIT EVENLY between the
left gutter and the right container.

**THE TEXT COLUMN IS NOT FIXED, AND "45rem FIXED" IS THE CORRECTION.** Starlight
sizes it from WHICH RAILS THE PAGE CARRIES, and we had only the first rung:

| Page has | Column | Sits |
|---|---|---|
| sidebar + on-this-page rail | 45rem | flush against the rail |
| sidebar, NO rail | **58rem** | CENTRED in what the navigation leaves |
| no sidebar at all | 67.5rem | centred |

- **A page declining the rail kept the narrow measure AND lost the track that
  justified it** — a 720px column pinned to the navigation with 380px of nothing
  to its right, measured at 1440. That was the landing page, and it is the whole
  of why it read as a reference page with a lot on it.
- **`--ao-measure-wide` is the second rung**, and `index.md` reaches it by
  declaring `toc: false` — declining the rail IS the decision to widen.
- **The 58rem rung is the REFERENCE SITE'S OWN OVERRIDE**, not Starlight core:
  `html[data-has-sidebar]:not([data-has-toc])` in its `@layer components`. Core
  ships the first and third rows only.

The rail keeps its width at that container's left, so its half is empty space
PAST the rail, never a fatter rail.

**Reproduced with an explicit `--ao-leftover`**, because `minmax(base, 1fr)` on
two tracks does NOT share a remainder: `fr` sizes against the whole free space,
so both tracks come out the same WIDTH. That is what once gave an 810px rail
beside a 66px gutter.

**Body type is 17px on purpose.** Red Hat Text is narrow, and at 16px that 45rem
column reads 99 characters.
