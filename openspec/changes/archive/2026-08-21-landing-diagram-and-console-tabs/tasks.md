## 1. The simplified drawing

- [x] 1.1 Add a third page `landing` to `docs/diagrams/agent-ops.drawio`, canvas
      950px wide, keeping the `site` page's COMPOSITION — cluster panel and
      pill, signal column, declare zone with its three kinds and the manifest,
      run zone, both connectors — and dropping only the masthead, the verb
      ladder, the seams band and the differentiator cards. Verify by opening the
      file: three pages, and `landing` holds none of those four.
- [x] 1.2 Keep at least one non-infrastructure signal in the first beat (the
      *a room gets too warm* / Home Assistant entry). Verify it is present on
      the page.
- [x] 1.3 Keep the poster's type verbatim — 19px headings and labels, 16px
      subtitles, manifest and pill — and add a full-canvas ground rect so both
      exports ship an opaque background distinct from the cluster panel's.
      Verify by measuring in the built page: nothing renders below 12px.
- [x] 1.4 Reuse the palette and the embedded icons already in the file. Verify
      no new external asset reference appears (`grep -c 'xlink:href="http'` on
      the exports is 0).

## 2. Export

- [x] 2.1 Extend `docs/diagrams/export.py` to export BOTH `site` and `landing`,
      keeping the per-page "no single page in the export" failure check for
      each. Verify by running it: it writes four files and names each.
- [x] 2.3 Build the exporter's scratch directory BESIDE the source rather than in
      `/tmp`, and ignore it. A VM-backed daemon mounts the user's home and not
      `/tmp`, so a `/tmp` workdir bind-mounts EMPTY and the run fails silently.
      Verified by running the export: it fails from `/tmp`, succeeds beside the
      source.
- [x] 2.2 Run it and commit `docs/assets/img/agent-ops-landing-light.svg` and
      `-dark.svg`. Verify the dark export reports its icons repainted and its
      labels are legible on the dark canvas.

## 3. Theme: variant resolution and panel styling

- [x] 3.1 In `docs/assets/js/tabs.js`, extend `paint()` from `-light.png` to
      `-(light|dark)\.(png|svg)`, and make it rewrite `a[href]` inside a panel
      as well as `img[src]`. Verify in a browser: switching theme swaps both the
      diagram and the full-size link's target.
- [x] 3.2 In `docs/assets/css/agentops.css`, give a panel-hosted diagram
      (`.ao-diagram`) the strip's frame and a scroll frame — `overflow-x: auto`
      on its container, `min-width: 42rem` on the image — so a narrow viewport
      scrolls the diagram and never the page. Verify at a 360px viewport: the
      diagram scrolls inside its frame, the body does not.
- [x] 3.3 Remove the `.ao-diagram-details`, `-summary`, `-link`, `-hint` and
      `.ao-diagram--light` / `--dark` rules, which nothing uses once the
      disclosure block is gone. Verify with
      `grep -rn 'ao-diagram-' docs/` returning only the removed-rule-free files.

## 4. The layout

- [x] 4.1 Remove the `<details class="ao-diagram-details">` block from
      `docs/_layouts/home.html`, leaving hero → stat tiles → `{{ content }}`.
      Verify the rendered landing page shows the tiles then the page body.
- [x] 4.2 Remove the now-unread `diagram`, `diagram_label` and `diagram_alt`
      keys from `docs/index.md`'s front matter. Verify with
      `grep -n 'diagram' docs/index.md` returning only body references.

## 5. The landing page's tab strip

- [x] 5.1 Add the strip at the head of `docs/index.md`'s body as a markdown list
      named `{: .ao-tabs #tour}`, panel 1 being the diagram: one image naming
      `agent-ops-landing-light.svg` with its own alt text. No link to the
      full-size poster — that drawing gets its own page in a later change, and a
      bare link under a figure earns nothing. Verify the page renders one strip
      with the diagram selected on arrival.
- [x] 5.2 Add panel 2, the `Pipeline` manifest, as a fenced `yaml` block lifted
      unchanged from the drawing's YAML card. Verify every field on it exists in
      `chart/files/crds/` for `Pipeline`, and that the block renders as one code
      panel rather than a platform-tab pair.
- [x] 5.3 Add the six console panels — Overview, Topology, Conversations,
      Conversation, Queues, Configuration — each one short claim plus its
      `-light.png` screenshot, and a link to the Console page for the full tour.
      Verify each named image exists under `docs/assets/img/console/`.
- [x] 5.4 Check the landing prose for anything the simplified diagram no longer
      carries or now says twice, and reconcile. Verify by reading the page and
      the drawing side by side: no claim appears in both.

## 6. Page order and integrations

- [x] 6.1 Place the stat tiles by splitting the page's rendered content at its
      first `<h2>` in `docs/_layouts/home.html`, so the order is hero, opening,
      tiles, sections. Verify in the built page that the strip precedes
      `ul.ao-stats` and the first `h2` follows it.
- [x] 6.2 Add a general `{: .ao-chipsets}` component — a list of groups becomes
      labelled chip rows, laid out as a two-track grid so a wrapping row keeps
      its label in column one. Verify it renders above the strip, wraps at a
      phone width without the body scrolling, and reads as a labelled list with
      no stylesheet.
- [x] 6.4 Open the page with three groups answering the reader's own questions —
      where work arrives from, what an agent can reach, where it answers — with
      the console in both the first and the third. Verify no group is styled
      differently from another.
- [x] 6.5 Commit each integration's own mark under `docs/assets/img/logos/`,
      unaltered, with a README recording every source URL and its terms, and let
      the PAGE name each file so no vendor appears in the stylesheet. Verify each
      mark renders in both themes and that `agentops.css` names no vendor.
- [x] 6.7 Tighten the chips — smaller type, tighter padding, a narrower label
      column. Verify in the built page that only the largest group wraps to a
      second line and that no chip breaks across lines mid-name.
- [x] 6.6 Add `agent-ops.svg` as a standalone copy of the mark for the console
      chip, since `_includes/logo.svg` draws from theme custom properties an
      `<img>` cannot inherit. Verify it loads (a non-zero `naturalWidth`) — an
      XML comment containing a double hyphen makes an `<img>`-loaded SVG fail to
      parse silently, which is how this shipped blank once.
- [x] 6.3 Retire `stats_kicker` from `docs/index.md`, whose words the drawing's
      pill now carries, and correct the bundle tile's stale `VictoriaMetrics` to
      `Prometheus`. Verify neither string survives in the built page.

## 7. Documentation of the change itself

- [x] 7.1 Add `{: .ao-diagram}` (a diagram in a panel) to the components table in
      `docs/CLAUDE.md`, and note that a themed image is named once and resolved
      by the theme. Verify the table lists every class a page may name.
- [x] 7.2 Update the `docs/` entry in the root `CLAUDE.md`: the drawing has three
      pages, `export.py` writes four SVGs, and the landing page leads with a tab
      strip rather than a diagram block. Verify no sentence there still describes
      the disclosure block.

- [x] 7.3 Record the `/tmp`-is-not-mounted trap in the root `CLAUDE.md` gotchas,
      with the gpg credential-helper failure beside it. Verify both are stated
      where a contributor hits them, not only in the script.

## 8. Verification

- [x] 8.1 Run the prose lint from `docs/CLAUDE.md` over `docs/*.md`. Verify it
      is silent.
- [x] 8.2 Build the site and LOOK at it in both themes at desktop, tablet and
      phone widths: tab strip, diagram legibility, the yaml panel, all six
      screenshots, theme switching mid-panel, and deep-linking a tab by
      fragment.
- [x] 8.3 With scripting disabled, verify the strip degrades to the labelled list
      with every panel visible, and that the light diagram is still legible on
      the dark theme.
- [x] 8.4 Measure a label in the rendered first panel at the 720px column width.
      Verify it is at least 12px.
- [x] 8.5 Run `openspec validate landing-diagram-and-console-tabs --strict` and
      verify it passes.
