# Tasks

**`mockup/landing.html` is the reference for composition.** Open it before
starting (see `mockup/README.md`) and PORT from it — the geometry, the beat
script, the timings and the spacing are settled there. Re-deriving them from the
specs produces a different page, which is the failure this mockup exists to
prevent. Where the mockup and a spec disagree the SPEC wins, and the mockup is a
prototype that got it wrong.

Two places the mockup is deliberately NOT the target, because it is a standalone
page rather than the site:

- Its video autoplays and loops. **The site's recording keeps its poster and its
  no-autoplay behaviour** — the reader starts it.
- Its violet-ruled notes and its closing cost table are commentary for the
  reviewer, not page content.

## 0. Read the mockup

- [x] 0.1 Serve the repository root and open `mockup/landing.html`. Step every beat, switch all three tabs and toggle the theme. Verify you can state what each beat turns on before writing any code.

## 1. The presentation component

- [x] 1.1 Port the presentation's stylesheet rules from the mockup into `docs/assets/css/agentops.css`, replacing its literal colours with the palette's tokens — the stage, the nodes and their ghost state, the connector tracks, the stanza and the caption bar. Verify `grep -n '#[0-9a-fA-F]\{3,6\}' docs/assets/css/agentops.css` still returns hits only inside the two palette blocks.
- [x] 1.2 Port the mockup's beat driver into a `docs/assets/js/` script, changed to read the page's beat list, build the strip and remove the list. Verify with scripting disabled the beats render as an ordinary ordered list, in order, with nothing hidden.
- [x] 1.3 Port the drawing's geometry and ghost skeleton from the mockup — the node coordinates, the connector paths and their dotted tracks, every node starting dashed and faint, so a beat only brings an element forward. Verify by stepping to beat 1 and confirming the full composition is present and only that beat's subject is emphasised.
- [x] 1.4 Scale the stage to its container rather than reflowing it, setting the container's height from the scale factor and hiding its overflow. Verify at 1400px, 1000px, 760px and 620px that the stage's rendered width fits its container and neither it nor the page body scrolls sideways.
- [x] 1.5 Port the per-beat manifest stanzas from the mockup's beat table, showing only the lines each beat concerns. Verify every field name shown exists on the `Pipeline` CRD in `chart/files/crds/`.
- [x] 1.6 Add the transport controls — play/pause, direct beat selection, a progress indicator — and stop the automatic advance when a beat is selected directly. Verify each control is reachable and operable by keyboard and shows a visible focus state.
- [x] 1.7 Take the beat wording from the mockup, reserving the caption's height so no beat reflows it. Verify by measuring the caption's height on all nine beats and asserting one distinct value.
- [x] 1.8 Honour `prefers-reduced-motion`: no movement, the drawing fully composed, the beats readable as a list. Verify in a browser context with reduced motion requested.
- [x] 1.9 Register the component with `assets/js/themed.js`'s resolver rather than watching `data-theme` itself. Verify toggling the theme repaints the presentation and the player together, with neither left on the wrong variant.

## 2. The landing page's opening

- [x] 2.1 Rewrite `docs/index.md`'s opening as the name, one sentence, the two claim chips and the tab strip, taking the wording from the mockup. Verify the rendered page shows nothing between the name and the tab strip except those two lines and the chips.
- [x] 2.2 Delete the `lede:` and `stats:` front matter keys. Verify neither name appears in `docs/index.md`.
- [x] 2.3 Order the panels presentation, recording, manifest. Verify the first tab is the presentation and the recording keeps its poster, its no-autoplay behaviour and its caption track.
- [x] 2.4 Replace the three chip rows with one `works with` group below the tab strip. Verify every mark it names is a file the page names and that each ships in the release being described.
- [x] 2.5 Delete the stat-tile block and the first-`<h2>` content split from `docs/_layouts/home.html`. Verify the page's sections render in the order `index.md` writes them.
- [x] 2.6 Delete the stat-tile rules and the `stat-icon` include if nothing else uses them. Verify with a repository-wide grep for `ao-stat` and `stat-icon` that no reference remains.

## 3. The Why section

- [x] 3.1 Add the `## Why agent-ops?` heading, its one sentence, and the two-column table of six areas of use with a mark per row, taking all eight strings from the mockup. Verify the table has a header row and that no code value in it wraps.
- [x] 3.2 Add the console's full-width strip beneath the table. Verify it renders below the table and not as a row within it.
- [x] 3.3 Add the table and strip rules to the stylesheet. Verify at 620px that the table does not force the page body to scroll sideways.

## 4. Retire the exported landing drawing

- [x] 4.1 Delete the `landing` page from `docs/diagrams/agent-ops.drawio` and its entry from `docs/diagrams/export.py`. Verify `python3 docs/diagrams/export.py` runs clean and writes only the `site` variants.
- [x] 4.2 Delete `docs/assets/img/agent-ops-landing-{light,dark}.svg`. Verify a repository-wide grep for `agent-ops-landing` returns nothing outside the archived change history.

## 5. The recording's new beat

- [x] 5.1 Add a beat to `platform/console/ui/demo/story.ts` in which a person opens a new conversation and picks the pipeline from the `/` typeahead, placed after the reply beat. Verify the beat drives the console through its own data path and paints no state directly.
- [x] 5.2 Shorten existing holds so the recording stays inside `MAX_SECONDS`. Verify `npm run demo` completes without breaching the duration or per-variant byte budget, and that the budgets themselves are unchanged.
- [x] 5.3 Verify the regenerated caption track covers the new beat and that its cues stay in step with the recording's real length.

## 6. Verification across the page

- [x] 6.1 Build the site and assert per page, in both colour schemes, that `document.documentElement.scrollWidth` does not exceed `clientWidth`. Verify the landing page passes in both.
- [x] 6.2 Step every beat of the presentation and assert no two visible elements on the stage overlap and nothing spills the stage's bounds. Verify across all nine beats.
- [x] 6.3 Render the landing page with scripting unavailable. Verify the beats are an ordinary list, the panels are a labelled list with every image and manifest present, and no panel is empty or replaced by a placeholder.
- [x] 6.4 Run the prose lint from `docs/CLAUDE.md` over the changed pages. Verify it is silent.
- [x] 6.5 Screenshot the finished landing page in both themes and read the captures. Verify it matches `mockup/landing.html` band for band, and write the captures to the scratchpad rather than the repository.

## 7. Documentation

### Reference docs

- [x] 7.1 Update `docs/CLAUDE.md`'s components table with the presentation and the areas table, and correct the site's pages row, which currently says `index.md` owns "the tab strip (the recording, the diagram, the manifest)". Verify no row still names the retired diagram.
- [x] 7.2 Update `docs/.claude/site.md`: `home.html` no longer splits content at the first `<h2>`, and `diagrams/` no longer exports the `landing` page. Verify both statements match what the code now does.
- [x] 7.3 Confirm no `CHANGELOG.md` entry is owed — this ships no chart version and no image. Verify by checking the change touches nothing under `chart/`.

### The adopter site

- [x] 7.4 Read `docs/introduction.md`, `docs/getting-started.md`, `docs/installation.md` and `docs/console-guide.md` for sentences naming the landing page's tiles, chip rows or diagram. Verify each is either untouched or corrected, and say which.
- [x] 7.5 Read `docs/guides/*.md` on the same terms, and confirm the chip-set component they use is unaffected. Verify by rendering one guide that names it.
- [x] 7.6 Check `README.md` for claims about the landing page or the retired diagram. Verify it stays inside its 150-line budget with `wc -l README.md`.
