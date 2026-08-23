# Claude context — docs/ (the published site)

Rules for the pages an adopter reads. The root `CLAUDE.md` routes WHICH document
receives what. This file governs HOW a page under `docs/` is written and built.

@.claude/site.md

**`docs/` IS the site root.** GitHub Pages builds it from `master` (Deploy from a
branch → `/docs`), so there is no workflow, no Gemfile and no Ruby in anyone's
path. Plugins are limited to the set Pages enables by default.

## The site's pages

| Page | Owns |
|---|---|
| `index.md` | the name, one sentence, the claim chips, the tab strip (the presentation, the recording, the manifest), `Why agent-ops?` and its areas table, paths onward |
| `introduction.md` | the model — two sections, concepts and guides, no reference detail |
| `getting-started.md` | the read-only DEMO walkthrough, console-first |
| `console-guide.md` | what the console is FOR: its views, and the authentication decision |
| `installation.md` | the REAL install, and the PARENT chart's values |
| `guides/*.md` | one adoption tier each, in learning order — hand-written prose around GENERATED resource blocks |

- **`console-guide.md` is published at `/console/`.** `console.md` is the
  untouched reference and keeps its own URL.
  - **What the console is FOR goes to the guide.** What it IS goes to the
    reference.
- **Every product asset is build output**, never a hand capture, and every one
  is rendered from ONE curated fixture that names no real cluster.
  - **The screenshots** — `npm run screenshots` in `console/ui`, published to
    `assets/img/console/`, shown on the CONSOLE page.
  - **The landing recording and its poster** — `npm run demo` in the same
    place, published to `assets/video/`, shown on the LANDING page.
- **The six views are toured in ONE place**, the Console page. The landing page
  shows the recording instead — a still cannot show work arriving and being
  answered.
- **The recording carries no text of its own.** No caption, no title card. What
  each beat shows is the page's words, beside it.
- **Every other `docs/*.md` is a reference page, not a site page.** They carry no
  front matter, so Jekyll copies them verbatim. Do not add front matter, headings
  or navigation entries to them — publishing one is its own change.
- **`cr-reference.md` is GENERATED and is one of those reference pages.** Every
  field of every kind, written by `.github/scripts/docs-generate.py` from
  `chart/crds/`. Never edit it, and never give it a nav entry.
  - **The `yaml` blocks in `guides/` come from the same run** — the resource
    templates from the CRDs, the worked examples from `helm template`.
  - **A page declares what it wants in an HTML comment**
    (`<!-- generated: … -->`) and the generator fills the block beneath it. CI
    regenerates everything and FAILS on a difference, which is the only thing
    that stops a committed generated file rotting.

**Adding a page is the markdown file plus ONE line in `_data/nav.yml`**, and the
page DECLARES its own `permalink`. No permalink style is configured, and the
sidebar marks the current entry by comparing URLs.

## `CHANGELOG.md`

Migration guides for every breaking change. The ONLY place upgrade steps live.

### Format: Keep a Changelog 1.1.0

Follows <https://keepachangelog.com/en/1.1.0/>. Binding, not a suggestion:

- **Newest first.** Unreleased at the top, then versions descending.
- **One `##` heading per version**, `## [<version>] — <YYYY-MM-DD>`.
  - Usually a CHART version, with the image tags it ships named in the entry.
  - A release that bumps ONE component and no chart gets its own heading, named
    for it — `## [console 0.15.9]`.
- **One heading per version, not per change.** Two things shipped in one chart
  version are one entry with two bullets, never two headings.
- **Group changes under the six `###` types**, in this order, omitting empty
  ones: `Added` · `Changed` · `Deprecated` · `Removed` · `Fixed` · `Security`.
- **A breaking change is marked in its entry**, not given a type of its own.
- **Upgrade steps belong to the version that needs them**, as a `### Upgrade`
  block after the change types.

The old shape — one `## Unreleased` holding thirty `###` version headings — is
what this replaces. It made every released version look unreleased and left the
type of each change unstated.

### Length: ten versions, then archive

`CHANGELOG.md` holds the **ten most recent versions** and nothing older.

- **Older entries move to `changelog/CHANGELOG-<range>.md`**, VERBATIM. They are
  already in this format when they get there, so moving one is a file move and
  nothing else.
- **An `## Older versions` section at the FOOT** of `CHANGELOG.md` links each
  archive file and the version range it covers.
- **Archives are append-only.** Moving an entry never edits it — a migration
  guide someone is following must not change under them.
- **Trim when the eleventh version lands**, as part of that release, never as a
  separate tidy-up.

Ten because a reader upgrading skips at most a few versions, and a 2000-line
file is one nobody scrolls.

## Writing: structure over prose

Everything here is written to be SCANNED. The markdown is the structure.

- **Structure first.** A procedure is NUMBERED STEPS. A mapping is a TABLE. A set
  is BULLETS. The claim a page rests on is a callout. *A paragraph that
  enumerates is a list that has not been written yet.*
- **Short sentences, one idea each.** Three clauses is three sentences. If it has
  to be read twice, it is wrong.
- **NO SEMICOLONS.** A `;` is a full stop that lost its nerve — the tell of a
  sentence that should have been two. Forbidden whatever the grammar allows.
- **Small paragraphs.** Past about three lines it stops being read.
- **Emphasise the load-bearing phrase**, not the sentence around it. Everything
  bold means nothing bold.
- **Cut what earns nothing.** Reasoning belongs in the reference page that owns
  it, in the root `CLAUDE.md`, or in the commit message.

**The failure mode is recognisable and has been shipped here more than once:**
long compound sentences, every point explained twice, nothing scannable — prose
doing a table's job.

Reference pages may be dense. A page an adopter meets first may not.

The binding statement of the shape rules is at the foot of this file. It is
quoted verbatim and is not paraphrased anywhere.

## Commands: every block is BOTH platforms

An adopter on Windows must not have to translate. Every command block gives a
**PowerShell** and a **Linux/macOS** version, and the theme renders them as tabs:

````markdown
```sh
kubectl -n agent-ops get secret x -o jsonpath='{.data.token}' | base64 -d
```

```powershell
kubectl -n agent-ops get secret x -o jsonpath='{.data.token}' | base64 -d
```
````

- **Write the two fences ADJACENT** and the theme pairs them — `sh` first, then
  `powershell`.
- **The page carries no markup and no tab widget.** It names the shell with the
  fence language, exactly as it names a card with `{: .ao-cards}`.
- **Give both fences even where the command is byte-identical.** A reader on
  Windows should never have to wonder.
- **ONLY SHELL LANGUAGES PAIR.** `tabs.js` groups `sh`/`bash`/`shell`/`zsh` and
  `powershell`, nothing else. A `yaml` block copied into a `powershell` fence
  never becomes a tab, so the page silently renders the same snippet twice —
  `installation.md` shipped exactly that.

What actually differs: line continuations (`\` vs a backtick), variable
assignment, and quoting around JSON.

## Components a page may name

A page needing more than prose NAMES a component with a **kramdown attribute
list**. The page names the class, the theme draws it. No `<div>`, no inline
style, no script in a page.

| Attribute | Renders |
|---|---|
| `{: .ao-cards}` | a two-column card grid, stated not derived. An odd count leaves the last card at normal width |
| `{: .ao-callout}` | a blockquote that EMPHASISES. The plain one is an ASIDE in `--ao-text-subtle`, so rendering a load-bearing claim in it puts a footnote where the weight belongs |
| `{: .ao-tabs}` | a list becomes tabbed panels, each item's leading bold phrase the label. With no script it stays the labelled list, so every word and image lives in the page |
| `{: .ao-icon-*}` | a kind glyph on a card title, copied from the console |
| `{: .ao-diagram}` | an exported drawing inside a panel. Below the measure it scrolls in its own frame rather than shrinking its labels. The landing page shows none — its model is the presentation |
| `{: .ao-demo}` | on a LINK whose target is a recording and whose content is the poster image, it becomes a player. With no script it stays the poster, linking to the file |
| `{: .ao-chipsets}` | a list of GROUPS becomes labelled chip rows — each item's leading bold phrase is the label, its nested list the chips. A chip may carry an ordinary markdown image, and the PAGE names that file. With no CSS it stays a labelled list of names |
| `{: .ao-claims}` | a list becomes the claim chips under the page's opening sentence. A claim may carry a mark the PAGE names |
| `{: .ao-presentation}` | an ORDERED list becomes the presentation: one item per beat, the item's text its caption, a fenced block under it the manifest stanza that beat is about. The THEME supplies the drawing, the timing and the controls, and it names no integration mark. With no script it stays the beats in order, each with its own lines |
| `{: .ao-areas}` | a two-column table of areas of use, the left column a name and its mark, the right what happens there. Below the measure the two columns become two lines |
| `{: .ao-console-strip}` | a blockquote becomes the full-width strip under that table — a mark, one lead line, one paragraph and a list of tags |
| `next:` in front matter | the what-next card (eyebrow / title / body / url) at the foot of the on-this-page rail. A page declaring none gets no card |

**`lede:`, `eyebrows:` and `stats:` are GONE, and so is the stat-icon include.**
The landing page's opening is now the page's own content, in the order the page
writes it, and `home.html` places one `<h1>` and nothing else.

- **The counts went because they answer a question a first-time reader has not
  asked**, in the position where they are deciding whether the product is for
  them at all. Such counts belong on the reference pages that own them.
- **The standfirst went because the page explained itself twice** before showing
  anything, and the panel set immediately below states the model in full.

**FRONT MATTER is the other half of that division.** Every word is the page's,
and the include only places it.

Content never moves into `_includes/` or `_data/` to get a look.

### Images and marks

- **Integration marks live in `assets/img/logos/`**, committed unaltered from
  each project's own source with their terms in that directory's README.
  - **The PAGE names each file.** A vendor list in `agentops.css` would be
    product knowledge in the theme.
  - **Never repaint one to fit the palette.** A mark that does not read on a
    ground is dropped instead.
- **An `<img>`-loaded SVG is parsed STRICTLY.** XML forbids a double hyphen
  inside a comment, so a comment mentioning a `--custom-property` makes the
  whole mark fail to render — silently, as blank space with `naturalWidth` 0.
  Check that before blaming the CSS.
- **A themed asset is named ONCE.** A page writes the `-light` file and nothing
  else.
  - `assets/js/themed.js` rewrites it to `-dark` from the resolved theme —
    screenshot, diagram, recording and poster alike, and a link in a panel as
    well as an image.
  - **It is ONE resolver on purpose.** The strip and the player both register
    with it, because two painters watching one `data-theme` would leave one
    asset on the wrong variant after a toggle.
  - A page that named both would be carrying theme knowledge.
- **A published image carries its own ground**, never a transparent one that
  borrows the page's canvas. The rewrite is a DEFERRED script, so the light file
  is on the page before it runs — and on a dark page a transparent light export
  is not a mismatched colour, it is invisible ink.

## Tables

- **Code never wraps in a table.** `table-layout: auto` will break
  `persistence.storageClassName` across lines to save pixels, inventing a key
  that does not exist. The stylesheet forbids it, so the column widens instead.
- **Watch the last column.** Two long code values in a three-column table crush
  the description to two words a line. When that happens the table is the wrong
  shape — use two columns, or a values snippet.
- **Give every table a header row.** A headerless table renders as an empty
  strip and reads as a rendering accident.

## Before calling a page done

**CHECK IT, do not remember it.** These rules were written and then broken on the
very next page, twice, and caught each time by the reader rather than the writer.

**1. The prose rules, mechanically.** Silence is the pass.

```sh
awk 'FNR==1{fm=0;b=0;n=0}
     /^---$/{fm=!fm;next} fm{next} /^```/{b=!b;next} b{next}
     {t=$0; gsub(/`[^`]*`/,"",t); gsub(/&[a-z]+;/,"",t)
      if (t ~ /;/) printf "%s:%d SEMICOLON\n", FILENAME, FNR}
     /^\|/||/^[0-9]+\./||/^[-*] /||/^ /||/^>/{next}
     /^$/{if(n>45)printf "%s:%d LONG PARAGRAPH (%d words)\n",FILENAME,s,n;n=0;next}
     {if(n==0)s=FNR;n+=NF}' $(ls docs/*.md docs/guides/*.md | grep -v cr-reference)
```

**`cr-reference.md` is excluded, and it is the only exclusion.** Its prose is
CRD doc comments, so a semicolon there is a Go source file's and the fix belongs
in `platform/manager/api/v1alpha1/`, not in a generated page.

**This file is the only place it is written out.** The root `CLAUDE.md` used to
carry a second copy, and the two drifted — one was fixed and the other was not.

**2. Build it and LOOK.** Not optional — the squeezed column, the wrapped key
and the headerless table were all invisible until rendered.

```sh
OUT=~/.cache/agentops-docs-site; rm -rf $OUT; mkdir -p $OUT
docker run --rm -v "$PWD/docs":/src -v "$OUT":/out -w /src -e JEKYLL_ENV=production \
  jekyll/jekyll:4 sh -c 'gem install jekyll-default-layout jekyll-seo-tag \
    jekyll-sitemap --no-document -q >/dev/null 2>&1; jekyll build -d /out --quiet'
```

Three details, each of which cost a round:

- **The image ships no Pages plugins.** `jekyll-default-layout` is the one the
  build dies without, and it must be installed as ROOT — so do not pass `-u`.
- **Serve it over HTTP, not `file://`.** `_config.yml` sets a `baseurl`, so the
  absolute asset paths only resolve under `/agent-ops-operator/`. Symlink the
  output to that name and `python3 -m http.server`.
- **The output must live under `$HOME`.** A VM-backed daemon (Rancher Desktop)
  does not mount `/tmp`, so a bind mount there is silently empty.

**3. Check what a screenshot cannot show.** Anchors, table shape and horizontal
overflow are per-page facts a glance at the top of the page misses. Assert them
in the browser rather than reading for them.

**The browser is already here** — `platform/console/ui/node_modules/playwright-core`,
with its Chromium in `~/.cache/ms-playwright`. Drive it from a script and assert
`document.documentElement.scrollWidth > clientWidth` per page, in BOTH
`colorScheme`s. Nothing needs installing.

- **Serve the build first.** `_config.yml` sets a `baseurl`, so symlink the
  output to `agent-ops-operator/` and `python3 -m http.server`.
- **Rebuilding while that server runs serves a HALF-WRITTEN site**, and every
  asset then 404s. It reads as a broken page rather than a race — stop the
  server, rebuild, start it again.

## Authoring rules (binding)
Concise + LLM-optimized. Cut filler, marketing tone, preambles. Every sentence earns its tokens.
Structure over prose:
Steps → numbered list.
Choices / mappings → table.
"X means Y" → **X.** Y on its own line.
Multi-rule bullet → parent + sub-bullets, one rule per line.
Prose paragraph stating > 2 rules → restructure.
