## 1. Site scaffolding

- [x] 1.1 Create `docs/_config.yml`: `title`, `description`, `url`
  (`https://kostiantyn-matsebora.github.io`), `baseurl: /agent-ops-operator`,
  `repository`, kramdown + Rouge defaults, `exclude:` for `diagrams/` and any
  non-page assets, and `defaults` mapping the index to the `home` layout
- [x] 1.2 Add `docs/_site/` and `docs/.jekyll-cache/` to `.gitignore`

## 2. Identity assets

- [x] 2.1 Copy `RedHatDisplayVF.woff2`, `RedHatTextVF.woff2`,
  `RedHatMonoVF.woff2` from
  `console/ui/node_modules/@patternfly/react-core/dist/styles/assets/fonts/`
  into `docs/assets/fonts/`
- [x] 2.2 Commit the SIL OFL-1.1 licence text as `docs/assets/fonts/LICENSE.txt`
- [x] 2.3 Create `docs/_includes/logo.svg` from
  `console/ui/src/components/Logo.tsx` — same paths, same
  `var(--ao-brand)`/`var(--ao-surface)` fills, `role="img"` +
  `aria-label="agent-ops"`

## 3. Stylesheet

- [x] 3.1 Create `docs/assets/css/agentops.css` opening with the `--ao-*` light
  and dark token blocks copied verbatim from
  `console/ui/src/theme/theme.css`, under `:root, .pf-v6-theme-light` and
  `.pf-v6-theme-dark`, with a header comment naming the source file
- [x] 3.2 Declare `@font-face` for the three families with `font-display: swap`
  and a system fallback stack; set Display for headings, Text for body, Mono for
  code
- [x] 3.3 Build the shell: fixed masthead (mark + "agent-ops" + theme switcher),
  left sidebar nav, content column, footer — the console's arrangement, no
  centred blog column
- [x] 3.4 Style prose: headings, paragraphs, lists, blockquotes, tables (bordered
  with `--ao-border`, header row on `--ao-surface-alt`), inline code and code
  blocks on `--ao-code-bg`
- [x] 3.5 Write the Rouge highlight rules from tokens (comment → subtle text,
  string → success, keyword → accent, background → code-bg) so one scheme serves
  both themes
- [x] 3.6 Responsive rules: sidebar collapses behind a toggle below the narrow
  breakpoint, content goes full width, tables/code/pre get
  `overflow-x: auto`, body never scrolls horizontally
- [x] 3.7 Verify no hex colour appears outside the token block (`grep -n '#[0-9a-fA-F]\{3,6\}' docs/assets/css/agentops.css`)
- [x] 3.8 Honour `prefers-reduced-motion` for any transition the shell adds

## 4. Layouts and includes

- [x] 4.1 `docs/_layouts/default.html`: `<head>` with `jekyll-seo-tag`, the
  blocking inline theme script, stylesheet and font preloads (all through
  `site.baseurl`), skip link, masthead, sidebar, content slot, footer
- [x] 4.2 `docs/_layouts/home.html`: the landing layout
- [x] 4.3 `docs/_layouts/page.html`: the minimal layout — title and content only.
  It is NOT what the untreated reference pages use (with no front matter they
  are static files: copied verbatim, never given a layout). It exists for the
  change that publishes them, where front matter makes each one a page and
  `jekyll-default-layout` assigns it `page` — a layout named but missing DOES
  fail the build. It gains features there, not here
- [x] 4.4 `docs/_includes/masthead.html`, `sidebar.html`, `theme-switcher.html`,
  `footer.html` — presentation only, no product prose
- [x] 4.5 Sidebar marks the current entry with `aria-current="page"` and a visual
  state by comparing `page.url` to the entry URL

## 5. Theme control

- [x] 5.1 Inline head script: read `agentops-docs-theme` from `localStorage`,
  resolve `system` against `prefers-color-scheme`, stamp
  `pf-v6-theme-*` + `data-theme` + `colorScheme` on `<html>` before first paint
- [x] 5.2 `docs/assets/js/theme.js`: wire the three-state control, persist the
  CHOICE (never the resolved theme), and subscribe to `matchMedia` so `system`
  follows the OS live
- [x] 5.3 Verify no light flash on a stored dark choice across a full page
  navigation, and that the control reflects the stored choice on load

## 6. Navigation and landing page

- [x] 6.1 `docs/_data/nav.yml`: the group/entry structure with the landing page
  as its only entry — the mechanism the follow-up change extends by adding lines
- [x] 6.2 `docs/index.md`: the adopter hub — the product diagram, then its own
  copy as prose (what "takes care of it" means, why it is built this way, the
  three seams), then grouped paths onward to the reference pages (linked where
  they live today), the README, the CHANGELOG and the repository. No CRD table,
  demo transcript or install copy
- [x] 6.3 `docs/diagrams/export.py`: export the drawio source to BOTH variants
  (`--svg-theme light|dark`) in the `rlespinasse/drawio-export` container, and
  repaint the embedded icons' ink in the dark one — drawio cannot recolour an
  image, so its `shape=image` icons come out near-black on the dark ground. A
  script rather than a comment, because a hand step after every export is the
  kind that gets skipped once and ships a broken diagram
- [x] 6.4 Swap the two variants by theme class, both `loading="lazy"` so the
  hidden one is never fetched; no plate and no frame, so each sits on its own
  page's canvas. Verify legibility in BOTH themes, icons included
- [x] 6.5 Bring the diagram into the product's palette: orange → `--ao-brand`
  teal (styles AND the icons' own embedded accent), and delete the four stat
  cells from the drawing — the page states them as tiles right below, and
  saying it twice is what made the hero read as padding
- [x] 6.6 Stat tiles: content in the page's front matter, markup in the layout,
  icons in `_includes/stat-icon.html`. A KPI row, not four hero figures — values
  in ink with the identity carried by the tile's rule, proportional figures
- [x] 6.7 Size the diagram for LEGIBILITY, not for the prose measure: it fills
  the content column, and past a wide breakpoint it breaks out of the measure up
  to 78rem, bounded by `calc(100vw - sidebar - gutters)` so the body can never
  scroll sideways (pinned at 1280/1440/1920). Its 12–15px labels are why — a
  drawing this dense is unreadable at prose width. It also links to itself full
  size for anyone who wants to inspect it
- [x] 6.10 Make it collapsible with `<details open>` — expanded by default,
  because a reader who came to understand the product should not have to ask for
  the picture, and foldable so a returning reader reaches the paths onward a
  screen sooner. `<details>` and not a script: it folds with JS off and is a tab
  stop and a space-bar toggle for free
- [x] 6.8 Give the drawio a SECOND page, `site`, for the landing page: the same
  drawing with the poster's own masthead removed (eyebrow pills, headline,
  standfirst). `why` keeps them — it is standalone and nothing around it speaks.
  `export.py` exports `site` only
- [x] 6.9 The page states what the drawing gave up, in real text: the two
  eyebrow pills, the `h1` and the standfirst, from the page's own front matter.
  Selectable, translatable, and read without going through alt text — which is
  the reason to move them out rather than merely to avoid saying them twice

## 7. Repository documentation

- [x] 7.1 Trim `README.md` to what it is and how to start it: pitch and diagram,
  the one-line-per-kind CRD table, the demo, install, a links-onward index with
  the site first, and short development and status sections. Anything cut is
  linked, never dropped; `wc -l README.md` ends at most 150 and should land well
  under
- [x] 7.2 Add to `CLAUDE.md`: `docs/` is a Jekyll source published by GitHub
  Pages; routing rows for site presentation (theme files) versus adopter prose
  (a markdown page); and the note that the palette is copied from
  `console/ui/src/theme/theme.css`, so a token change is a two-file change
- [x] 7.3 Add a `docs/` line to the repository map in `CLAUDE.md` noting the
  theme directories
- [x] 7.4 Confirm no `docs/*.md` reference page was modified by this change
  (`git diff --name-only docs/` lists only new theme files and `index.md`)

## 8. Publish and verify

- [ ] 8.1 Maintainer step: Settings → Pages → Deploy from a branch → `master` /
  `/docs`; confirm the deployment succeeds and the site is live at
  `https://kostiantyn-matsebora.github.io/agent-ops-operator/`.
  **Two things to settle first — the repository is PRIVATE**: Pages on a private
  repository requires a paid plan, and every link the landing page makes into
  the repository (the reference pages, the CHANGELOG, the README) 404s for a
  visitor without access, which is most of a public docs site's audience
- [x] 8.2 The build does not fail on the untreated reference pages; each is
  copied verbatim (no front matter means Jekyll treats it as a static file, so
  it is never converted and never given a layout) and is reachable by URL as raw
  markdown, unlinked from anything on the site
- [x] 8.3 Landing page: every link resolves, including the ones pointing at the
  reference pages on GitHub
- [x] 8.4 Both themes checked on the landing page; system-follow verified by
  flipping the OS theme with the page open
- [x] 8.5 Phone-width check: sidebar collapses, no horizontal body scroll, wide
  content scrolls in its own frame
- [x] 8.6 Load the site with all third-party origins blocked and confirm the
  fonts, palette and navigation are intact; confirm the network panel shows
  requests to the site's own origin only
- [x] 8.7 Keyboard pass: skip link is the first stop and moves focus to the
  content; the theme control and nav are reachable and operable

All of 5.3 and 8.2–8.7 were verified against a LOCAL build of the site
(`jekyll/jekyll` container, served at its `/agent-ops-operator` base path,
driven by Playwright: 18/18 checks — both themes, live OS-follow, no
pre-paint flash, phone width, skip link and keyboard, same-origin-only
requests). Re-confirm them on the published site once 8.1 is done.
