# Rebuild the landing page around a presentation

## Why

**The landing page opens with inventory, not an argument.** Three rows of vendor
chips read as a logo list, four stat tiles answer a question nobody has yet — how
many CRD kinds there are — and the one thing that would tell a first-time visitor
whether this is for them sits behind a tab.

**The reference class settles the shape, and it is not the one we shipped.**
Argo CD, the closest analogue, leads with the **name**, then **one line**, then
the **UI in motion**, then a `Why Argo CD?` section. Flux, Crossplane and KEDA are
quieter still. None of them opens with a chip inventory and none of them opens
with a KPI row.

**The exported drawing cannot carry the explanation.** It is a still, so it must
state the whole model at once at 950px, and every attempt to add detail to it has
had to remove elements instead. A presentation that builds the same model one
piece at a time has no such ceiling, and its words stay real text.

**The agreed composition is committed with this change**, working, at
`mockup/landing.html`.

## What Changes

- **NEW: an in-page presentation component**, named by a page on a markdown list
  of beats. Nine beats, roughly a minute, autoplaying with play/pause and beat
  dots. The drawing accretes on one canvas, a manifest stanza changes with the
  beat, and with no scripting it degrades to the beats as an ordinary list.
- **The landing page's opening becomes the name, one line, two claim chips and
  the tab strip** — in that order. The tab strip's first panel is the new
  presentation, the second is the existing recording, the third the manifest.
- **BREAKING (site content): the `lede:` front matter key is deleted**, and with
  it the standfirst. The name and one line replace it.
- **BREAKING (site content): the `stats:` front matter key and the stat-tile
  block in `_layouts/home.html` are deleted.** The layout's content-splitting
  seam goes with them.
- **The three chip rows leave the opening.** The chip-set component stays — the
  guides use it — and the landing page keeps ONE group, labelled `works with`,
  under the tab strip.
- **NEW section `## Why agent-ops?`** — a heading, one sentence, and a
  two-column table of six areas of use, each row carrying a small mark. Below it
  a full-width strip for the console, which is not a seventh area but where all
  six are watched and answered.
- **The `landing` page of `docs/diagrams/agent-ops.drawio` is retired**, along
  with its two exported SVGs. The `site` page and the full-size link are
  untouched.
- **The recording gains a beat** in which a person starts a conversation and
  picks the pipeline from the `/` typeahead — the one thing on the page that
  shows a person choosing who answers.

## Capabilities

### New Capabilities

- `landing-presentation`: the in-page presentation — what a beat is, how the
  page declares them, the accreting canvas and its ghost skeleton, the
  per-beat manifest stanza, the transport controls, the scaling that keeps it
  from ever scrolling, and what it degrades to without scripting.

### Modified Capabilities

- `docs-site`: the landing page's opening order, its panel set, the removal of
  the stat tiles and the layout seam that placed them, the new `Why agent-ops?`
  section and the console strip, and the recording's move from first panel to
  second.
- `architecture-diagrams`: the `landing` drawing page is retired, so the
  requirement describing its composition is removed and the inline-legibility
  requirement narrows to the drawings the site still displays.

## Impact

### Reference docs

- **`docs/CLAUDE.md`** — the components table gains the presentation and the
  areas table; the site's pages table currently says `index.md` owns "the tab
  strip (the recording, the diagram, the manifest)", which stops being true.
- **`docs/.claude/site.md`** — states that `home.html` splits content at the
  first `<h2>` and drops the tiles in the seam, and that `diagrams/` holds three
  drawio pages of which `landing` is exported. Both stop being true.
- **`docs/CHANGELOG.md`** — no entry. This ships no chart version and no image;
  the changelog holds upgrade steps, and an adopter has nothing to upgrade.
- **`.claude/rules/documentation.md`** — its routing table sends "how a page
  READS" to `docs/CLAUDE.md`, which is where the new components are recorded. No
  change to the rule itself.

### The adopter site

- **`docs/index.md`** — rewritten. This is the change.
- **`docs/introduction.md`, `docs/getting-started.md`, `docs/installation.md`,
  `docs/console-guide.md`, `docs/guides/*.md`** — checked for sentences that
  name the landing page's tiles, chip rows or diagram. None is expected to
  change, and the check is a task rather than an assumption.
- **`README.md`** — carries its own diagram and links the site. Unaffected, and
  verified rather than assumed.

### Code and assets

- `docs/assets/css/agentops.css` — the presentation, the areas table and the
  console strip; the stat-tile rules are deleted.
- `docs/assets/js/` — one new script for the presentation.
- `docs/_layouts/home.html` — the stat-tile block and the content split are
  deleted.
- `docs/diagrams/agent-ops.drawio` and `docs/diagrams/export.py` — the `landing`
  page and its export entry are removed.
- `docs/assets/img/agent-ops-landing-{light,dark}.svg` — deleted.
- `platform/console/ui/demo/story.ts` — one added beat, and shortened holds to
  stay inside the existing 75-second ceiling.
- `docs/assets/video/*` — regenerated by `npm run demo`.
