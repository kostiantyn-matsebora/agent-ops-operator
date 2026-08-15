# Claude context — docs/ (the published site)

Rules for the pages an adopter reads. The root `CLAUDE.md` routes WHICH document
receives what. This file governs HOW a page under `docs/` is written and built.

`docs/` IS the site root: GitHub Pages builds it from `master` (Deploy from a
branch → `/docs`), so there is no workflow, no Gemfile and no Ruby in anyone's
path. Plugins are limited to the set Pages enables by default.

## The site's pages

| Page | Owns |
|---|---|
| `index.md` | the landing pitch, the diagram, paths onward |
| `introduction.md` | the model — two sections, concepts and guides, no reference detail |
| `getting-started.md` | the read-only DEMO walkthrough, console-first |
| `installation.md` | the REAL install, and the PARENT chart's values |

The other `docs/*.md` are **reference pages, not site pages**. They carry no
front matter, so Jekyll copies them verbatim. Do not add front matter, headings
or navigation entries to them — publishing them is its own change.

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
| `{: .ao-icon-*}` | a kind glyph on a card title, copied from the console |
| `next:` in front matter | the what-next card at the foot of the rail |

Content never moves into `_includes/` or `_data/` to get a look.

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
