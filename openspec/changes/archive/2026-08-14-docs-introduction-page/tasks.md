## 1. Write the page

- [x] 1.1 Create `docs/introduction.md` with front matter carrying `title:
      Introduction`, `permalink: /introduction/` and a one-sentence
      `description:` for `jekyll-seo-tag`. No `layout:` key —
      `jekyll-default-layout` assigns `page`. Open the body with an `##`, never
      an `#`: the layout already emits the `h1` from `title`.
- [x] 1.2 **Opening** (2–3 sentences): what agent-ops is, and the structural
      idea — identity, wiring and execution are separate objects, so what an
      agent may do comes from the routing that wakes it and never from the agent.
      One sentence of overlap with the landing page is allowed; a second telling
      of the pitch is not.
- [x] 1.3 **The parts, by the question each answers** — who the agent is, what
      wakes it, what it may touch, where it answers, what executes it, what the
      running thing is. One or two sentences each, naming the kind and linking to
      `concepts.md` on GitHub. Grouped by question, NOT enumerated as eleven
      kinds — that table exists and is better.
- [x] 1.4 **What happens when something fires** — the lifecycle in order and in
      prose: a signal arrives on a source; per-source policy is evaluated once
      above the fan-out; every Ready Pipeline listing that source opens its own
      conversation; a conversation is serial, gets a thread on each bound channel
      and its own isolated pod; the operator posts the answer back to every bound
      thread. Name no field and no endpoint. This section is the page's reason to
      exist — give it the most room.
- [x] 1.5 **What you can swap** — the three seams (signal source, agent runtime,
      channel), what a swap changes and what stays fixed, linking to
      `contracts.md`.
- [x] 1.6 **Where to go next** — install and demo (README), the kinds in full
      (`concepts.md`), the contracts (`contracts.md`), the bundles and the
      console, in the order an adopter walks them. Links go to GitHub, where
      those pages live today; do not add navigation entries for them.
- [x] 1.7 Close with what the model buys — a short list in the register of the
      landing page, no new claims.
- [x] 1.8 Re-read against the test: **would any sentence have to change if a CRD
      field were renamed?** Anything that would becomes a link. Confirm the page
      names no field, endpoint, values key, install command or YAML block.
      Target roughly 800–1000 words.

## 2. Publish it

- [x] 2.1 Add one entry to `docs/_data/nav.yml` under *Start here*, after
      *Overview*: `title: Introduction`, `url: /introduction/` — matching the
      page's `permalink` exactly, or the sidebar's current-page marking silently
      stops working.
- [x] 2.2 `docs/index.md`: make the Introduction the first path onward under
      *Where to start*, ahead of the README, as a site-internal link. Leave the
      closing note about the reference pages living on GitHub — it is still true.
- [x] 2.3 Confirm no layout, include, stylesheet or other data file was touched.
      If one was, the page needs rewriting in the elements the theme already
      styles.

## 3. Record it

- [x] 3.1 `CLAUDE.md`: update the `docs/` map line that reads "index.md is the
      site's landing page; the reference pages above are NOT yet site
      deliverables" to name `introduction.md` as the second site page, keeping
      the reference-pages sentence intact — it is still the rule.

## 4. Verification

- [x] 4.1 Confirm the page renders with the site shell: masthead, sidebar,
      palette and prose styling, in both themes.
      *Verified by building the site in a `ruby:3.3` container (jekyll 4.3.4 +
      jekyll-seo-tag + jekyll-default-layout — the repo keeps no Gemfile; the
      build ran against a copy in the scratchpad) and driving the result with
      playwright.*
- [x] 4.2 Confirm exactly one `h1` on the page, and that it reads "Introduction".
- [x] 4.3 Confirm the sidebar marks *Introduction* current with
      `aria-current="page"` when it is open, and *Overview* when the root is.
- [x] 4.4 Follow every link on the page and confirm none 404s and each lands on
      the document that owns that content.
- [x] 4.5 Confirm the reference pages are unchanged (`git status`), still carry
      no front matter, and still have no navigation entry.
- [x] 4.6 Read it at a phone width: no horizontal scroll on the body, sidebar
      behind its toggle.
- [x] 4.7 Confirm the build needs nothing new — no plugin added to `_config.yml`,
      no workflow, no Gemfile.

## 5. Revision: make it scannable

Prose-only proved wrong on review — seven concepts and a six-step lifecycle read
as walls nobody finishes. The design's "no theme work" line was revised rather
than worked around (design decisions 7–9).

- [x] 5.1 Add components to `docs/assets/css/agentops.css`, each named by a page
      with a kramdown attribute list and holding no words: `.ao-cards` (auto-fit
      grid, first paragraph is the card heading) and `.ao-callout` (a blockquote
      skinned to EMPHASISE, against the plain blockquote's aside).
- [x] 5.2 Rewrite the opening: lift the structural idea into a callout so the
      page does not open with two unbroken paragraphs.
- [x] 5.3 Rewrite the parts section as six cards, and lift the `Pipeline` out of
      the grid into a callout — it is not a seventh peer, and six cards grid
      evenly where seven leave an orphan.
- [x] 5.4 Add a **Follow the guides** section listing the task-shaped
      walkthroughs still to be written, in two groups. Every entry is UNLINKED
      and the section says so — a link to a page that does not exist is a defect,
      and a roadmap that looks clickable is worse than none.

## 6. Revision: two sections, everything a block

The page had grown five sections. Cut to a short opening plus **understand the
concepts** and **follow the guides** — design decision 8, which reverses
decision 3.

- [x] 6.1 Delete the lifecycle, the swappable seams and the closing summary from
      the page. The lifecycle becomes the FIRST listed guide ("What happens when
      something fires"); the seams and the closing claims are already owned by
      the contracts reference and the landing page.
- [x] 6.2 Make every concept a block, `Pipeline` included — it stops being a
      callout and becomes a full-width card of its own. No modifier class was
      needed: the grid is `auto-fit`, so empty tracks collapse and a one-item
      list spans the width. Commented in the stylesheet, since it looks
      accidental otherwise.
- [x] 6.3 Delete `.ao-steps` from the stylesheet. No page uses it now, and a
      component kept for a future page is dead CSS.
- [x] 6.4 Fold the onward reference links into the guides section rather than
      keeping a third section for them.
- [x] 6.5 Rebuild and re-verify: two `h2` sections and no others, two card grids,
      no `ao-steps` in the output, no stray `{: .ao-`, one `h1`, `aria-current`,
      no horizontal overflow at 1280 or 390, both themes legible.
- [x] 6.6 Re-check the stylesheet carries no literal colour.

## 7. Revision: the card the reference actually shows, and a real right column

- [x] 7.1 Rebuild `.ao-cards` as documentation cards: title is the kind's NAME
      (the question moves into the description), the WHOLE card is the link via
      an overlay on the title, an arrow is drawn only on cards that link, two
      columns, hover state.
- [x] 7.2 Give each card its kind's glyph, copied from
      `console/ui/src/graph/shapes.tsx` and drawn as a CSS **mask** with
      `background-color: var(--ao-brand)` so it follows the theme — a data-URI
      background image cannot inherit `currentColor`. The page names the icon
      with `{: .ao-icon-*}`; no SVG enters the markdown.
- [x] 7.3 Put every concept in ONE grid, `Pipeline` included. An odd count leaves
      the last card at normal width; a stretched final card reads as a summary of
      the others.
- [x] 7.4 Empty the guides section — the heading and one line saying there are
      none yet.
- [x] 7.5 Add the on-this-page column: `_includes/toc.html` (empty rail),
      `assets/js/toc.js` (builds from rendered headings, tracks scroll by
      position, special-cases the page bottom), the shell's third grid column,
      and `toc: true` by default in `_config.yml` with the landing page
      declining it.
- [x] 7.6 Make it a COLUMN, not a floating block: full viewport height, sticky,
      its own `border-left`, 18rem — mirroring the left sidebar.
- [x] 7.7 Verify in a browser: the ToC builds and marks the section being read at
      top and at bottom; the whole card is the link (hit-test a far corner);
      two columns; the rail disappears below 78rem with no horizontal overflow;
      icons render in both themes.
- [x] 7.8 Make the rail WIDE: cap the content column at the measure and give the
      rail everything left over (`minmax(--ao-toc-w, 1fr)`), so extra width on a
      large display goes to the column rather than stretching prose.

## 8. Revision: the shell is the SITE's, and the landing page pays for it

- [x] 8.1 Drop `toc: false` from the landing page — every page carries the
      column, so the shell no longer changes between pages.
- [x] 8.2 Delete the diagram/hero breakout. It sized against
      `100vw - sidebar - gutters`, knew nothing about a third column, and so drew
      the diagram 256px UNDERNEATH the rail. Deleted rather than scoped to a
      two-column shell that no longer occurs — dead CSS is untested CSS.
- [x] 8.3 Verify the landing page at 1920/1440/1280: the diagram's right edge
      equals the content's, never crossing into the rail, and no page scrolls
      horizontally.

## 9. Revision: readable measure, and a gutter rather than a gap

- [x] 9.1 Cut `--ao-measure` from 62rem (99 characters a line) to 36rem (~78).
- [x] 9.2 Put the spare width in the LEFT gutter (`--ao-main-pad-l`, 12rem past
      78rem), not between the text and the rail. An intermediate version capped
      prose inside a wider box and left ~450px of dead space before the rail,
      which reads as an inflated right column — the gap after the last word is
      what the eye takes for the layout.
- [x] 9.3 State the card columns (two, one below 48rem) instead of deriving them
      from `auto-fit`, which gave three at 1920 and two at 1440 — the section
      changed shape with the window.
- [x] 9.4 Verify at 1920/1440/1280: text ~78 characters, 32px from the text to
      the rail, two card columns, no horizontal overflow.

## 10. Revision: the reference's geometry, by ratio

- [x] 10.1 Set body type to 17px. Red Hat Text is narrow — at 16px the
      reference's 720px column reads 99 characters, which is why every attempt
      to keep its geometry produced unreadable lines and spare width to hide.
      At 17px the same column reads ~85–91 and the proportions fall out.
- [x] 10.2 Flip which track flexes: navigation and rail FIXED, middle track
      `1fr`, content flush to the rail (`margin-left: auto`), so the surplus
      becomes the GUTTER. With the rail on `1fr` it was taking half the window.
- [x] 10.3 Size the tracks by ratio against the text column — sidebar 0.39,
      gutter 0.80, rail 0.37 — since screenshots of the reference come at
      different zooms and only ratios are comparable.
- [x] 10.4 Cap the gutter (36rem) BELOW the content column (45rem), so the
      centre is the widest track by construction at every width.
- [x] 10.5 Verify at 2560/2000/1700/1600/1440/1280: ratios hold, the content
      column is the widest track at all six, and nothing scrolls sideways.

## 11. Revision: read the reference's stylesheet instead of its screenshots

- [x] 11.1 Load the reference in a browser, capture its stylesheets and read the
      layout variables: `--sl-sidebar-width: 18.75rem` (BOTH rails),
      `--sl-content-width: 45rem`. Screenshots could not settle this — they come
      at different zooms, and no ratio tells you which track flexes.
- [x] 11.2 Derive the distribution rule by measuring the live site at five
      widths: the leftover is split EVENLY between the gutter and the right
      container, with the rail panel keeping its width at that container's left.
- [x] 11.3 Reproduce it with an explicit `--ao-leftover` and
      `calc(var(--ao-toc-w) + var(--ao-leftover) / 2)`. Note in the stylesheet
      why `minmax(base, 1fr)` on two tracks cannot express this: `fr` sizes
      against the whole free space, so two such tracks come out equal — which is
      what produced an 810px rail beside a 66px gutter.
- [x] 11.4 Match the remaining details: 18.75rem rails, 45rem text, 40px from the
      text to the rail, and the two-column breakpoint at 72rem.
- [x] 11.5 Verify by measuring BOTH sites with one script at 2560/1920/1600/1440:
      sidebar, gutter, text and gap agree within 4px at every width.
- [x] 5.7 Confirm no literal colour entered the stylesheet:
      `grep -n '#[0-9a-fA-F]\{3,6\}' docs/assets/css/agentops.css` returns hits
      only inside the token blocks.
- [x] 5.8 Rebuild and re-verify: attribute lists attached (no stray `{: .ao-` in
      the output), one `h1`, `aria-current`, no horizontal overflow at 1280 or
      390, both themes legible.
- [x] 5.9 `CLAUDE.md`: record the components and the rule that a page names them
      rather than writing markup.
