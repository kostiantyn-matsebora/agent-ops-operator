## Why

The landing page leads with a **1778 × 1349 poster** shown in a 45rem column. At
that scale its 15px labels render at roughly 6px — present, and unreadable. The
drawing also restates the page body: the verb ladder, the three seams and the
three differentiators are all prose on the same page, so the reader meets each
claim twice and neither telling is the one they can read.

The page also never shows the product. The console is the thing an adopter would
recognise, six screenshots of it are already committed and generated, and the
landing page uses none of them.

## What Changes

- **A new, compressed diagram** — a third page in `docs/diagrams/agent-ops.drawio`,
  exported as `agent-ops-landing-{light,dark}.svg`. It is the POSTER'S OWN
  COMPOSITION, not a redesign: the cluster panel and its pill, the signal column
  beside it, the declare zone with its three kinds and the real `Pipeline`
  manifest, the run zone below, and the two connectors between them. What is
  dropped is only what the page body already states in prose — the verb ladder,
  the pluggability band and the three differentiator cards.
- **It is authored for its DISPLAYED size**, not for a poster. The drawing is
  compressed to 950px wide with type kept at poster sizes, so that at the content
  column's width no text renders below 12px. This, not the element count, is what
  makes it readable.
- **The manifest also becomes its own panel** — a real fenced `yaml` block in the
  page, so it is selectable, copyable and searchable as well as drawn.
- **One tab strip on the landing page**, replacing the collapsible diagram block:
  tab 1 the diagram, tab 2 the `Pipeline` manifest, then **six console views** —
  Overview, Topology, Conversations, Conversation, Queues, Configuration —
  reusing the committed screenshots.
- **The page opens with what it plugs into** — two groups of chips, one naming
  what ships in the chart and one what any other integration goes through, on a
  new general `{: .ao-chipsets}` component.
- **The strip comes BEFORE the stat tiles.** The layout places the tiles by
  splitting the page's rendered content at its first section heading, so a page
  states its opening, the tiles close it, and the sections follow. The page
  writes nothing about where the tiles go.
- **`stats_kicker` is retired** from the landing page — the drawing's pill states
  it, and the two together were the one claim said twice.
- **The theme resolves the diagram's theme variant**, exactly as it already does
  for screenshots: the page names one image, `assets/js/tabs.js` swaps the
  variant. Its rewrite extends from `.png` to `.svg`.
- **The simplified exports carry an OPAQUE ground.** The poster's transparent
  ground makes the light variant invisible on a dark page when scripting is off,
  and a plate also matches the framing the screenshots in the same strip have.
- **Removed**: the `.ao-diagram-details` disclosure block from
  `_layouts/home.html`, the `diagram`/`diagram_label`/`diagram_alt` front matter,
  and the CSS rules that only that block used.
- The full poster stays in the repository and stays linked, as the detailed view
  behind the simplified one.

## Capabilities

### New Capabilities

None. Both affected capabilities already exist.

### Modified Capabilities

- `docs-site`: the landing page requirement changes — the page leads with a tab
  strip rather than a disclosure block, the simplified diagram is its first
  panel, the `Pipeline` manifest is a panel of page text rather than an exported
  image, and the console tour appears on the landing page. The theme's
  variant-resolution rule extends from screenshots to any themed image a panel
  names.
- `architecture-diagrams`: the drawing gains a third page, and the requirement
  that it hold exactly one finished page is replaced by one naming each page and
  its job. The new page has its own content rule (the three-beat spine, nothing
  the landing page states in prose) and a legibility rule (authored for the
  width it is displayed at).

## Impact

| Area | Change |
|---|---|
| `docs/diagrams/agent-ops.drawio` | third page `landing` |
| `docs/diagrams/export.py` | exports two pages, four SVGs |
| `docs/assets/img/` | `agent-ops-landing-{light,dark}.svg` added |
| `docs/index.md` | diagram front matter removed, tab strip added to the body |
| `docs/_layouts/home.html` | diagram block removed |
| `docs/assets/js/tabs.js` | variant rewrite extended to `.svg` |
| `docs/assets/css/agentops.css` | diagram-disclosure rules removed, `.ao-diagram` in a panel added |
| `docs/CLAUDE.md`, root `CLAUDE.md` | the components table and the `docs/` map entry |

No Go code, no chart, no CRD. Nothing outside `docs/` is touched.
