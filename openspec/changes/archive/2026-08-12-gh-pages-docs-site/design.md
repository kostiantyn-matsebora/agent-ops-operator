## Context

There is no adopter-facing page for this project anywhere. `README.md` is the
GitHub front door and `docs/` is six reference pages read as blobs; a person
told "look at agent-ops" lands in a repository listing.

The console already answers "what does this product look like": PatternFly 6
shell, a deliberately non-Red-Hat palette (`--ao-*` teal primary, violet accent)
declared once in `console/ui/src/theme/theme.css`, Red Hat variable fonts, an
inline SVG mark, a three-state light/dark/system switch that defaults to system
and subscribes to it live. That file's own comment states the rule the docs site
must inherit: **one source of truth** for the palette.

This change is scoped to the **theme and the landing page**. Taking the existing
reference pages onto the site — nav entries, cross-page links, per-page
constraints, on-page contents — is a follow-up that inherits what lands here.

Constraints that shaped every decision below:

- **No local toolchain.** This machine has no Go and no Ruby; builds happen in
  containers or in GitHub. A documentation site that cannot be published without
  `bundle exec` on the author's laptop would be published rarely.
- **Self-contained repository.** The modules take no dependencies outside the
  repo directory; a docs site pulling a remote theme or a font CDN is the same
  habit one layer up.
- **README is at exactly 150 lines**, its stated budget. It is the thing being
  trimmed here, not a thing being added to.

## Goals / Non-Goals

**Goals:**

- Publish a GitHub Pages site with zero CI and zero local tooling.
- Make it recognisably agent-ops to someone who has used the console.
- Give an adopter a front door: what this is, how to start, where to go next.
- Narrow `README.md` to the same two questions — what it is, how to start it —
  and let it link onward instead of carrying more.
- Land a shell that the follow-up extends by adding pages, not machinery.

**Non-Goals:**

- Publishing the existing `docs/*.md` reference pages as site pages. They are
  untouched, unlinked from the sidebar and unverified on the site.
- Writing the adopter guides themselves.
- Search, versioned docs, a custom domain, dark-mode diagram variants.
- Sharing code with the console UI. The site copies four things (token block,
  mark, font files, theme-choice semantics) and imports nothing; a build-time
  dependency from a Jekyll site on a Vite app would be worse than the copy.

## Decisions

### 1. Branch deploy from `master:/docs`, not an Actions workflow

GitHub Pages builds `docs/` as the site root when Pages is set to "Deploy from a
branch → `/docs`". Nothing else is needed: no workflow file, no `Gemfile`, no
`Gemfile.lock`, no Ruby anywhere in the contributor's path. `docs/_config.yml`
is the site config because `docs/` IS the site root.

*Alternative considered:* an Actions workflow with a pinned `Gemfile`. It buys
arbitrary plugins and a reproducible build, and costs the repository's first CI
workflow plus a Ruby dependency set to keep current. Rejected for now; the
site's shape does not depend on the choice, so it stays reversible.

*Consequence to respect:* plugins are limited to the set GitHub Pages enables.
A feature needing more gets implemented in the theme's own assets or dropped —
that rule is in the spec so the decision is not quietly reversed by the first
inconvenient feature.

### 2. The reference pages are in the source directory but out of the change

Branch deploy publishes the whole of `docs/`, so the existing `*.md` pages are
part of the source directory whether or not this change treats them. The scope
line is therefore about *treatment*, not exclusion: no front matter, no link
rewrites, no nav entries, no verification, no per-page requirements.

**What that means concretely, verified against a real build:** Jekyll renders
only files carrying YAML front matter. A markdown file with none is a STATIC
FILE — copied to the output byte for byte, never converted, never given a
layout. So the reference pages are served as raw markdown at
`/agent-ops-operator/concepts.md` and friends: unconverted, unstyled, unlinked
from anything on the site. Nothing on the site points at them (the landing page
links to the GitHub blobs), so no site link leads to one.

`jekyll-default-layout` therefore does not touch them, and the earlier
justification for `page.html` — "a missing layout fails the build" — was wrong
about pages that never reference a layout in the first place. The layout is kept
anyway, because it is what the follow-up change needs the moment it adds
`layout: page` to the first reference page, and because a page that DOES gain
front matter without naming a layout gets `page` assigned to it. It stays a
heading and a content slot; the on-page contents and the edit link belong to
that change.

*Alternative considered:* `exclude:` the reference pages so the site is
index-only and no raw markdown is served at a site URL. It matches the scope
line more literally and costs an exclusion list that the follow-up has to
unwind, plus a site whose only page is its front page. Rejected — a raw markdown
file at a URL nothing links to is inert, and the exclusion list is a second
place the page set would have to be kept correct.

### 3. Copy the token block; do not import PatternFly

`docs/assets/css/agentops.css` opens with the `--ao-*` light and dark blocks
copied verbatim out of `console/ui/src/theme/theme.css`, under both the
`pf-v6-theme-*` class selectors and `:root`, then every rule in the file is
written against those tokens. No PatternFly CSS or JS is loaded: the console
needs a component library because it renders tables, toolbars and a topology
graph; a prose site renders headings, paragraphs, tables and code.

Recognisability comes from four things, in descending order of how much work
they do: the palette, the shell arrangement (fixed masthead over left sidebar
over content — not a centred blog column), the mark, and the type.

*On drift:* the copy is deliberate and one-directional. The tokens are stable
(they are the product's identity, changed rarely) and the alternative — a build
step that extracts CSS from the console module into the docs directory — makes
the docs site depend on a Node build to publish a paragraph. The mitigation is
a comment at the head of each copied block naming its source file, and a task
to state the rule in `CLAUDE.md`.

### 4. Vendored variable fonts, no third-party requests

`RedHatDisplayVF.woff2`, `RedHatTextVF.woff2` and `RedHatMonoVF.woff2` are
copied from `console/ui/node_modules/@patternfly/react-core/dist/styles/assets/fonts/`
into `docs/assets/fonts/` (~300 KB total, SIL OFL-1.1, licence text committed
beside them). Variable fonts mean one file per family rather than one per
weight; the italics are not vendored — documentation prose barely uses them and
synthesised italic is acceptable at the size it does.

`@font-face` declares them with `font-display: swap`, and the stacks fall back
to `RedHatText`/system-ui so a page is readable before or without them.

*Alternative considered:* Google Fonts. One less binary in the repo, at the cost
of every adopter's browser announcing itself to a third party when reading
operator documentation, and a page whose typography depends on a CDN being up.

### 5. Theme control: the console's semantics, ~40 lines of vanilla JS

`docs/assets/js/theme.js` reimplements `useTheme.ts` without Zustand:

- Stores the CHOICE (`light` | `dark` | `system`), not the resolved theme, under
  its own `localStorage` key (`agentops-docs-theme` — the console's key belongs
  to the console; a docs site writing it would be reaching into another origin's
  namespace conceptually, and on a shared origin literally).
- `system` is the default and is a `matchMedia` subscription, not a startup read.
- Applies by putting `pf-v6-theme-light`/`pf-v6-theme-dark` on `<html>` (the
  copied token block keys off exactly those), plus `data-theme` and
  `style.colorScheme`.

The apply step ships **twice**: a tiny blocking inline `<script>` in `<head>`
reads storage and stamps the class before first paint, and the deferred module
wires the control and the subscription. Without the inline half, every page load
flashes light before the stored dark applies — the console avoids this with
`onRehydrateStorage`; a multi-page site has no equivalent, so the head script is
the mechanism, not an optimisation.

### 6. Navigation in `_data/nav.yml`, shipped nearly empty

Sidebar navigation is one YAML file of groups → entries. The sidebar template
marks the current page with `aria-current="page"` by comparing `page.url`
against the entry's URL. In this change it holds the landing page and nothing
else, because nothing else is a site deliverable yet.

Shipping the mechanism with one entry is the point: the follow-up change adds
reference pages by adding lines to a data file, not by inventing a navigation
system under deadline. A client-side on-page contents belongs to that change
too — it is a long-reference-page feature and there are no long pages here.

### 7. Layouts: `default` → `home`, plus the minimal `page`

`default.html` owns `<head>`, the inline theme script, the masthead, the sidebar
and the footer. `home.html` adds the landing layout. `page.html` is the
build-safety layout from decision 2 — title and content, nothing more. Includes:
`masthead.html`, `sidebar.html`, `logo.svg`, `theme-switcher.html`,
`footer.html`.

### 8. The landing page and the README answer the same two questions for
different readers

`docs/index.md` is the adopter hub: what the operator is, where to start, and
grouped paths onward — to the reference pages (on GitHub, until the follow-up
publishes them), the CHANGELOG and the repository. It is not a second README:
it carries no install transcript, no CRD table, no status.

`README.md` is trimmed to the same shape for the GitHub reader: pitch and
diagram, the one-line-per-kind list, the demo and the install, and links onward
with the site first. Everything else it might accumulate — behavior essays,
reference detail, upgrade steps — belongs to a `docs/` page or the CHANGELOG,
which the existing routing rule already says.

Neither restates the other, and the split is by consumer, not by content: a
person who arrived on GitHub gets the quick start inline because they are three
seconds from a `helm install`; a person who arrived on the site gets routed,
because they came to read.

### 9. Syntax highlighting from the palette

GitHub Pages uses kramdown + Rouge, which emits classed spans. A short Rouge
stylesheet is written against `--ao-*` tokens (comment = `--ao-text-subtle`,
string = `--ao-success`, keyword = `--ao-accent`, background = `--ao-code-bg`)
so one theme covers both light and dark instead of shipping two stock themes.
The landing page's code is shell and YAML; the vocabulary to cover is small.

## Risks / Trade-offs

- **The copied palette drifts from the console's** → Each copied block carries a
  header comment naming `console/ui/src/theme/theme.css` as its source, and
  `CLAUDE.md` records that a token change is a two-file change. No colour is
  stated literally anywhere else in the site CSS, so the sync is one block.
- **The untreated reference pages look shipped** → They are published, plainly
  styled and unlinked. A reader who reaches one by URL sees a real page in the
  theme; nothing on the site claims they are the documentation. The follow-up
  change is what makes them navigable, and until then the README and the landing
  page point at the GitHub blobs.
- **A default plugin changes behavior and the build breaks** → The failure mode
  is visible in the Pages deployment log, and the pages themselves are
  untouched, so recovery is a layout or a front-matter block, not a data loss.
- **`docs/` becomes a mixed directory** (prose + `_layouts` + `assets`) → This
  is the cost of branch deploy. Mitigated by the underscore convention (Jekyll
  directories sort together and are obviously infrastructure) and by the spec
  requirement that theme files hold no prose.
- **Trimming the README removes something a reader needed** → The trim is
  subtractive only where the content has an owner elsewhere; the links-onward
  index is what keeps it reachable, and it is checked against the current file
  section by section rather than rewritten from scratch.

## Migration Plan

1. Land the theme files, the landing page, the vendored fonts and the README
   trim in one commit. Nothing is published yet — Pages is off.
2. The maintainer enables Settings → Pages → Deploy from a branch → `master` /
   `/docs`. This is the only manual, non-repository step.
3. Verify the published landing page against the checklist in `tasks.md` (both
   themes, phone width, JS disabled, third-party hosts blocked, every link).
4. Add the site URL to the README and `CLAUDE.md`.
5. Follow-up change: take the reference pages onto the site.

**Rollback:** turn Pages off in repository settings. The `docs/` markdown is
unchanged by the theme, so the GitHub blob view is exactly what it was; deleting
the theme files returns the tree to its prior state with no dependency to
unwind, and the README trim stands or reverts independently.

## Open Questions

- **Custom domain** — not now; the `baseurl` decision assumes project pages at
  `/agent-ops-operator`, and moving to a domain root later is a `_config.yml`
  edit because every emitted link already goes through `site.baseurl`.
- **Should `CHANGELOG.md` be published too?** It is outside `docs/`, so it would
  have to be moved or duplicated. Left out; the site links to it on GitHub.
- ~~**An architecture diagram on the landing page**~~ — **RESOLVED: it ships.**
  `docs/diagrams/agent-ops.drawio` turned out to hold a finished poster of the
  whole pitch (the three acts, the stats, the three seams), not a sketch, so the
  landing page leads with it and takes its PROSE from it too — the page and the
  diagram must not tell the story twice with different words. Exported to
  `docs/assets/img/agent-ops.svg` with `rlespinasse/drawio-export` (a container,
  so no local toolchain); the export inlines its icons as data URIs, which is
  what keeps the no-third-party-request rule true and also why it is ~1 MB.
  **It ships as TWO variants, not one on a plate.** drawio bakes a
  `light-dark()` pair into every fill it owns and pins the resolution with
  `color-scheme` on the SVG root, so `--svg-theme light|dark` really does yield
  two palettes of one drawing; an `<img>` is its own document and does not
  inherit the page's scheme, so the page swaps them on the theme class (both
  `loading="lazy"`, so the hidden one is never rendered and never fetched).
  What drawio CANNOT recolour is an embedded image, and the icons are
  `shape=image` cells carrying their own SVG with hard-coded `#1A1A1A` ink —
  in the dark export they came out near-black on near-black: present, and
  invisible. `docs/diagrams/export.py` runs both exports and repaints that ink,
  which is why the export is a script and not two documented docker commands.
  The drawing was also brought into the product's palette (orange → `--ao-brand`
  teal, in the styles and inside the icons' own base64), and its four stat cells
  were deleted — the page states those as tiles immediately below it.
