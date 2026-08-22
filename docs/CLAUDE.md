# Claude context — docs/ (the published site)

Rules for the pages an adopter reads. The root `CLAUDE.md` routes WHICH document
receives what. This file governs HOW a page under `docs/` is written and built.

`docs/` IS the site root: GitHub Pages builds it from `master` (Deploy from a
branch → `/docs`), so there is no workflow, no Gemfile and no Ruby in anyone's
path. Plugins are limited to the set Pages enables by default.

## The site's pages

| Page | Owns |
|---|---|
| `index.md` | the landing pitch, the tab strip (diagram, manifest, the console), paths onward |
| `introduction.md` | the model — two sections, concepts and guides, no reference detail |
| `getting-started.md` | the read-only DEMO walkthrough, console-first |
| `console-guide.md` | what the console is FOR: its views, and the authentication decision |
| `installation.md` | the REAL install, and the PARENT chart's values |

`console-guide.md` is published at `/console/`. `console.md` is the untouched
reference and keeps its own URL. **What the console is FOR goes to the guide,
what it IS goes to the reference.** The screenshots on both the guide and the
landing page are build output — `npm run screenshots` in `console/ui`, never a
hand capture.

The landing strip and the guide's tour show the SAME six images at different
altitudes: one line each there, the full tour here. Keep the labels identical.

The other `docs/*.md` are **reference pages, not site pages**. They carry no
front matter, so Jekyll copies them verbatim. Do not add front matter, headings
or navigation entries to them — publishing them is its own change.

## `CHANGELOG.md`

Migration guides for every breaking change. The ONLY place upgrade steps live.

### Format: Keep a Changelog 1.1.0

Follows <https://keepachangelog.com/en/1.1.0/>. Binding, not a suggestion:

- **Newest first.** Unreleased at the top, then versions descending.
- **One `##` heading per version**, `## [<version>] — <YYYY-MM-DD>`. Usually a
  CHART version, with the image tags it ships named in the entry. A release that
  bumps ONE component and no chart gets its own heading, named for it —
  `## [console 0.15.9]`.
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

- Older entries move to `CHANGELOG-<range>.md` in this directory, VERBATIM.
- An `## Older versions` section at the FOOT of `CHANGELOG.md` links each
  archive file and the version range it covers.
- **Archives are append-only.** Moving an entry never edits it — a migration
  guide someone is following must not change under them.
- Trim when the eleventh version lands, as part of that release, never as a
  separate tidy-up.

Ten because a reader upgrading skips at most a few versions, and a 2000-line
file is one nobody scrolls.

Adding a page is the markdown file plus ONE line in `_data/nav.yml`, and the page
DECLARES its own `permalink` (no permalink style is configured, and the sidebar
marks the current entry by comparing URLs).

## Writing: structure over prose

Everything here is written to be SCANNED. The markdown is the structure.

- **Structure first.** A procedure is NUMBERED STEPS. A mapping is a TABLE. A set
  is BULLETS. The claim a page rests on is a callout. *A paragraph that
  enumerates is a list that has not been written yet.*
- **Short sentences, one idea each.** Three clauses is three sentences. If it has
  to be read twice, it is wrong.
- **NO SEMICOLONS.** A `;` is a full stop that lost its nerve.
- **Small paragraphs.** Past about three lines it stops being read.
- **Emphasise the load-bearing phrase**, not the sentence around it. Everything
  bold means nothing bold.
- **Cut what earns nothing.** Reasoning belongs in the reference page that owns
  it, in the root `CLAUDE.md`, or in the commit message.

Reference pages may be dense. A page an adopter meets first may not.

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

Write the two fences ADJACENT and the theme pairs them — `sh` first, then
`powershell`. The page carries no markup and no tab widget: it names the shell
with the fence language, exactly as it names a card with `{: .ao-cards}`.

What actually differs is line continuations (`\` vs a backtick), variable
assignment, and quoting around JSON. Where a command is byte-identical, still
give both fences — a reader on Windows should never have to wonder.

## Components a page may name

The page names a class, the theme draws it. No `<div>`, no inline style, no
script in a page.

| Attribute | Renders |
|---|---|
| `{: .ao-cards}` | a two-column card grid. An odd count leaves the last card normal width |
| `{: .ao-callout}` | a blockquote that EMPHASISES (the plain one is a muted aside) |
| `{: .ao-tabs}` | a list becomes tabbed panels, each item's leading bold phrase the label. With no script it stays the labelled list, so every word and image lives in the page |
| `{: .ao-icon-*}` | a kind glyph on a card title, copied from the console |
| `{: .ao-diagram}` | an exported drawing inside a panel. Below the measure it scrolls in its own frame rather than shrinking its labels |
| `{: .ao-chipsets}` | a list of GROUPS becomes labelled chip rows — each item's leading bold phrase is the label, its nested list the chips. A chip may carry an ordinary markdown image, and the PAGE names that file. With no CSS it stays a labelled list of names |
| `next:` in front matter | the what-next card at the foot of the rail |

Content never moves into `_includes/` or `_data/` to get a look.

**Integration marks live in `assets/img/logos/`**, committed unaltered from each
project's own source with their terms in that directory's README. The PAGE names
each file — a vendor list in `agentops.css` would be product knowledge in the
theme. Never repaint one to fit the palette: a mark that does not read on a
ground is dropped instead.

**An `<img>`-loaded SVG is parsed STRICTLY.** XML forbids a double hyphen inside
a comment, so a comment mentioning a `--custom-property` makes the whole mark
fail to render — silently, as blank space with `naturalWidth` 0. That is worth
checking before blaming the CSS.

**A themed image is named ONCE.** A page writes the `-light` file and nothing
else — `assets/js/tabs.js` rewrites it to `-dark` from the resolved theme, for
screenshots and diagrams alike, and for a link in a panel as well as an image.
A page that named both would be carrying theme knowledge.

That rewrite is a DEFERRED script, so the light file is on the page before it
runs. **A published image therefore carries its own ground**, never a transparent
one that borrows the page's canvas — on a dark page a transparent light export is
not a mismatched colour, it is invisible ink.

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

```sh
# 1. the prose rules, mechanically
awk '/^---$/{fm=!fm;next} fm{next} /^```/{b=!b;next} b{next}
     /;/ && !/&[a-z]+;/ {printf "%s:%d SEMICOLON\n", FILENAME, NR}
     /^\|/||/^[0-9]+\./||/^ /||/^>/{next}
     /^$/{if(n>45)printf "%s:%d LONG PARAGRAPH (%d words)\n",FILENAME,s,n;n=0;next}
     {if(n==0)s=NR;n+=NF}' docs/*.md

# 2. build it and LOOK at it — a rendering fault is invisible in markdown
#    (see the root CLAUDE.md for the container build and playwright)
```

Silence is the pass on 1. Step 2 is not optional: the squeezed column, the
wrapped key and the headerless table were all invisible until rendered.
