## Context

`docs/` is the site: GitHub Pages builds it from `master` with no workflow, no
Gemfile and no Ruby in anyone's path. The theme is finished — masthead, sidebar,
palette, fonts, prose styling for headings, lists, tables, code and blockquotes,
and a `page` layout that `jekyll-default-layout` assigns the moment a file gains
front matter. `_data/nav.yml` holds one entry.

`docs/index.md` is the landing page, and its job is the pitch: the diagram, what
"takes care of it" means, the three seams, and grouped paths onward. Those paths
all leave for GitHub, because the reference pages carry no front matter and are
therefore static files Jekyll copies verbatim.

So the site today can say what the product IS and can say every field of every
CRD, and has nothing in between. This change lands the middle.

## Goals / Non-Goals

**Goals:**

- One page that leaves an adopter able to read `concepts.md` and know what they
  are looking at.
- The lifecycle — signal to answer — stated once, on the site, in prose.
- Onward paths in the order an adopter wants them, each to the document that
  owns the content.
- Adding a page costs a markdown file and one navigation line, demonstrated.

**Non-Goals:**

- **Publishing the reference pages.** They stay untreated. This page links to
  them where they live, exactly as the landing page does.
- **Being a second `concepts.md`.** No field lists, no YAML, no endpoint names.
- **Being a second README.** No install command, no demo transcript, no status.
- **Anything one page needs.** The site gains a card grid, a callout and an
  on-this-page column (decisions 7 and 10), but each as a general facility every
  later page gets — not as styling for this one.
- **An edit link.** Still deferred with the reference pages.

## Decisions

### 1. The page teaches the model; it does not enumerate the API

MassTransit's introduction is largely a hub: one or two sentences per concept,
each linking to the page that owns it. Ours borrows the shape but carries a
little more weight, for a reason worth naming: **its onward links leave the
site.** A reader who clicks lands in raw markdown on GitHub, so a page that was
purely a set of link stubs would be a menu with no meal.

The page therefore states in its own prose the structural idea — identity, wiring
and execution are separate objects, so what an agent may do comes from the
routing that wakes it and never from the agent itself — and gives each concept
enough substance to mean something without a click. (An earlier draft carried the
lifecycle and the seams here too; decision 8 moves them to guides.)

Everything below that — fields, defaults, values keys, endpoint shapes — is a
link. The test for a sentence belonging here: **would it have to change if a CRD
field were renamed?** If yes, it belongs in `concepts.md` and this page links to
it instead.

### 2. Structure follows the questions, not the kinds

A list of eleven kinds is a reference table, and `concepts.md` already has the
good one. The page groups instead by the question each part answers — *who is
the agent, what wakes it, what may it touch, where does it answer, what runs it,
what is the running thing* — and names the kind that answers it. An adopter
holds four or five questions in their head, not eleven nouns.

*Alternative rejected:* mirroring the CRD list one-for-one. It reads as an index,
duplicates a table that exists, and goes stale the moment a kind is added.

### 3. The lifecycle is real and unwritten — and it is a guide, not this page

*Superseded by decision 8.*

"Something fires; what actually happens?" is a question the reference pages
answer only in pieces — grouping in the source spec, admission in the concepts
page, threads in the channel contract, delivery in the state-durability rules.
Nothing states it end to end, and it is the biggest gap in the documentation.

This decision originally put it here, as the page's centre of gravity. Decision 8
overturns that: the gap is real, the introduction is the wrong place to fill it,
and it is listed instead as the guide **"What happens when something fires"** —
the one to write first.

### 4. Front matter carries `title` and an explicit `permalink`

```yaml
---
title: Introduction
permalink: /introduction/
description: >-
  …one sentence, for search results and link previews…
---
```

The page opens with an H2, not an H1: `page.html` already emits `<h1>` from
`title`, and a second one would be a duplicate heading and an accessibility
defect.

`permalink` is explicit because `_config.yml` configures no permalink style, so
the default output would be `/introduction.html`. The sidebar marks the current
entry by comparing `page.url` to the navigation entry's `url`, so a page whose
URL is assumed rather than declared is one whose highlight silently stops
working. Declaring it makes the nav entry and the page agree by construction.

No `layout` key: `jekyll-default-layout` assigns `page`, which is the mechanism
that layout was built for and the one the next change relies on.

### 5. It goes under *Start here*, second

`Start here` currently holds *Overview* alone. *Introduction* follows it, and the
landing page's first path onward becomes this page rather than the README —
which is the ordering an adopter actually walks: what is this → how does it work
→ install it.

### 6. Off-site links stay off-site and are not disguised

Every onward link goes where the content lives today, on GitHub, as the landing
page's already do. The page does not pretend otherwise and adds no navigation
entry for a page that is not a site deliverable — a sidebar entry pointing at an
unpublished page is a defect, not a placeholder.

When the reference pages are published, those links become internal ones. That is
a find-and-replace in two files, and it is that change's work.

### 7. Two reusable components, named by the page and styled by the theme

Seven concepts as bullets read as one grey wall — the eye gets no purchase, and
the section that should be scannable is the least scannable on the page. They
are presented as a grid of cards instead, each carrying its question, the kind
that answers it, and a sentence or two.

The mechanism keeps **the theme holds no prose and the pages hold no theme**
intact in both directions:

- The page stays markdown — an ordinary list, with one kramdown attribute list
  (`{: .ao-cards}`) naming the class. That is an attribute, not markup: no
  `<div>`, no inline style, and the content is still readable as plain text in
  the repository.
- The theme gets a `.ao-cards` rule and holds no words.

*Alternatives rejected:* an `_includes/cards.html` would put the concept prose in
the theme, and a `_data/` file would do the same one directory over — both are
the exact inversion the rule forbids. Raw `<div>`s in the markdown fail from the
other side.

It is a **site component, not this page's styling**: the reference pages will
want it when they are published, which is what makes it theme work rather than a
one-page hack. The design's earlier "no theme work" line was written assuming
this page could be built from prose elements alone; seven concepts proved
otherwise, and a component every page can name is the smaller of the two
compromises.

**The Pipeline is lifted out of the grid**, because it is not a seventh peer: it
is the object the other six are arranged around, and the answer to "where do I
look to find out what an agent can do". It follows the grid as a callout. Six
cards also grid evenly where seven leave an orphan, which is the lesser reason
but a real one.

### 8. Two sections, and the lifecycle becomes a guide

The page is **understand the concepts**, then **follow the guides**, and nothing
else. The lifecycle, the swappable seams and the closing summary are cut.

This reverses decision 3, which made the lifecycle the page's centre of gravity.
That decision was wrong in a specific way: it was right that the sequence is
stated nowhere else end to end, and wrong that an introduction is where it goes.
Six bold-led paragraphs — even rewritten as a numbered flow — is a wall in front
of a reader who has not yet decided to invest, and its own author had already
had to trim it twice. Content that needs that much room is a **guide**, and it is
listed as one.

What the cut protects: an introduction whose job is to make the reference legible
does that job by being short enough to finish. Everything removed is either
listed as a forthcoming guide or already owned by a reference page.

*Consequence:* the `.ao-steps` component built for the lifecycle is **deleted**,
not kept for a future page. A component no page uses is dead CSS that rots
quietly, and the rule that a component must be general is not a licence to build
one speculatively.

### 9. A callout is a blockquote that emphasises, and it is not the plain one

Two paragraphs of unbroken text is the wrong first impression for a page whose
whole claim is that the model is legible, so the structural idea is lifted out of
the prose — and so is the Pipeline statement, which is the single most important
sentence on the page.

The first attempt used a plain blockquote, which was wrong and looked it: the
theme renders a blockquote as an **aside** — muted background, `--ao-text-subtle`
body — because that is what a blockquote is for. Rendering the page's
load-bearing claim in the de-emphasis style put a footnote where the emphasis
belonged.

`.ao-callout` is therefore a third component: the same blockquote element, named
by the page, skinned to carry weight (brand-tinted ground, full text colour,
heavier rule). The distinction is worth keeping sharp — an aside and a callout
look similar and mean opposite things, and a docs site that has only one of them
will misuse it.

### 10. The on-this-page column is JavaScript, and is the default for pages

GitHub Pages enables no table-of-contents plugin, and the site's founding rule is
that a feature needing one is implemented in the theme's own assets or dropped —
never by switching the build to an Actions workflow. kramdown already gives every
heading an id, so there is nothing to generate but the list: `assets/js/toc.js`
reads the rendered headings and builds it.

**It is the site's layout, not this page's** — `toc: true` in `_config.yml`
defaults, carried by every page including the landing one. Opt-in would mean
publishing a page depends on remembering a flag, and a forgotten flag is
invisible: the page just quietly lacks the column.

An earlier draft had the landing page decline it, on the reasoning that a pitch
is not something you navigate within. That produced a site whose shell changed
between its only two pages, which reads as a bug rather than as a decision — and
the landing page has seven sections, so it navigates fine.

*What that cost, and the choice taken:* the landing page's diagram and hero had a
**breakout** — widened past the prose measure to `100vw - sidebar - gutters`,
because the drawing is 1778px native with 12–15px labels and every pixel is
legibility. That subtraction knew about the sidebar and nothing else, so under
the three-column shell it overshot by exactly the rail's width and drew the
diagram *underneath* the rail (1536px right edge against a content edge of 1280).

A breakout is only ever as wide as the column it breaks out of, and the content
column is capped at the measure — so there is nowhere left to break into. The
rule is therefore **deleted**, not scoped to a shell that no longer occurs: dead
CSS is untested CSS. The diagram now fills its column (992px at 1920) and its
detail lives behind the full-size link it already carried.

The alternatives were weighed and declined: a two-column landing page restores
the breakout but reintroduces the shell inconsistency that prompted the change,
and widening the content column to preserve it puts a several-hundred-pixel
gutter between prose and rail on every page that has no diagram.

*Scroll tracking reads positions, not intersection ratios.* An
`IntersectionObserver` on headings goes quiet while a section taller than the
viewport is being read, which is where a long page spends most of its time. The
last-heading-above-the-fold calculation has no such hole. The bottom of the page
is special-cased, because a final short section's heading may never reach the top
of the viewport and its entry would otherwise never light up.

*It degrades in three directions:* fewer than two headings shows nothing rather
than a list of one; no JavaScript leaves the rail `hidden` as authored rather
than an empty column; and below 78rem it is dropped entirely rather than
squeezed, because it is a convenience where the left navigation is the way
between pages.

### 11. Cards are named, iconed and clickable — the console's card, not a box

The first card design was boxes with a question as the title and the kind name
buried in the body. That is not what a documentation card is. The shape, taken
from the reference:

- **the title is the thing's NAME** — `AgentProfile`, not "Who is the agent?";
  the question becomes the first words of the description;
- **the whole card is the link**, with an arrow saying so. The title's `::after`
  overlays the card, so the affordance is the card rather than a narrower phrase
  inside it — and the arrow is drawn only on a card that actually links
  somewhere;
- **each card carries its kind's glyph**, copied from
  `console/ui/src/graph/shapes.tsx` and drawn as a CSS **mask** so it takes
  `--ao-brand` and follows the theme. A data-URI background image cannot inherit
  `currentColor`, which is why the mask and not the obvious thing;
- **two columns**, and an odd count leaves the last card at normal width rather
  than stretching it — a full-width final card reads as a summary of the ones
  above it, which the Pipeline card is not.

The page still holds no markup: it names an icon with an attribute list
(`{: .ao-icon-profile}`) exactly as it names the grid, and the theme decides what
that looks like — the same division the landing page's stat icons already use.

### 12. The content column IS the measure, and the gutter is what fills the space

`--ao-measure` was 62rem and served as both the column and the cap on prose. In
this face that runs to **99 characters a line** — a container width wearing a
measure's name, and long paragraphs became work to follow. It is now 36rem,
about 78 characters.

An intermediate version split it in two — narrow prose (36rem) inside a wide box
(58rem), so cards and the diagram could stay large. That was wrong in a way worth
recording: every paragraph then ended several hundred pixels short of the
on-this-page rail, and a reader does not see "a generous content box", they see
**an inflated right column**. The gap between the last word and the rail is what
the eye reads as the layout.

So there is ONE width. The column equals the measure, the rail begins one gutter
after the text ends, and the space a large display has spare goes to the **left
gutter** (`--ao-main-pad-l`, 12rem past 78rem) and to the rail's own track — never
between the prose and the rail.

The left gutter is a token rather than a bare padding because the shell's middle
track is sized from it; a padding the grid did not know about pushes content past
its track, which is precisely how the diagram ended up under the rail.

### 13. The shell's geometry is READ from the reference, not inferred from it

The layout is Astro Starlight's, taken from its own stylesheet rather than
guessed from screenshots — which cannot work, because screenshots arrive at
different browser zooms and only ratios survive, and ratios do not tell you
which track flexes.

```
--sl-sidebar-width: 18.75rem   /* BOTH rails, one variable */
--sl-content-width: 45rem      /* fixed; the text never grows */
```

and the distribution rule, verified by measuring the live site at five widths:

> the leftover — viewport minus both rails minus the text column — is split
> **evenly** between the left gutter and the right container. The rail panel
> keeps its own width at that container's left, so its half becomes empty space
> **past** the rail rather than a wider rail.

Ours reproduces it with an explicit `--ao-leftover` and
`calc(var(--ao-toc-w) + var(--ao-leftover) / 2)`, and lands within 4px of the
reference at 1440, 1600, 1920 and 2560.

*The CSS trap that caused most of the churn:* `minmax(base, 1fr)` on two tracks
does NOT mean "share the remainder". `fr` sizes against the whole free space
including the bases, so two `1fr` tracks come out the SAME TOTAL WIDTH — which
gave an 810px rail beside a 66px gutter. Splitting a leftover requires stating
the split.

### 14. Which track flexes is the layout

The tracks are **navigation and rail fixed, middle flexible**, with the content
flush to the rail so the surplus becomes the gutter. Getting this backwards —
rail on `1fr`, middle track fixed — is what produced every complaint in
sequence: a rail holding half the window at 2000px, then a 608px void in the
gutter when the rail was capped, then a right column still visibly wider than
the reference's.

Two consequences worth stating as rules:

- **The content column is always the widest track.** The gutter's maximum
  (36rem) is deliberately below the column (45rem), so this holds by
  construction rather than by tuning.
- **Type size sets the column, not the other way round.** Red Hat Text is
  narrow: at 16px a 720px column runs to 99 characters, which is why every
  attempt to keep the reference's geometry produced unreadable lines and then
  hundreds of spare pixels to hide. Body type is 17px, at which 720px reads
  ~85–91 characters — and the reference's proportions then fall out with no
  cap, no flush-right trick and no dead space.

Geometry is matched by RATIO against the text column, not by pixels: screenshots
of the reference come at different browser zooms, so only ratios are comparable.
Sidebar 0.39, gutter 0.80, rail 0.37.

**Card columns are stated, not derived.** `auto-fit` with a minimum let the count
follow the window — two columns at 1440 and three at 1920 — so the section
changed shape with the browser. Two is the shape; one below 48rem.

## Risks / Trade-offs

- **The page drifts from the reference pages it summarises.** → Every sentence
  that could go stale is a link instead; the prose is deliberately at the level
  of *why the model is shaped this way*, which changes far more slowly than
  fields do. The stated test — "would a field rename break this sentence?" —
  is the guard.
- **It becomes a third telling of the pitch, after the README and the landing
  page.** → The three are given distinct jobs: the README is the pitch and the
  install, the landing page is the diagram and the paths onward, this page is
  the model and the lifecycle. The one overlap allowed is the single opening
  sentence of what the product is.
- **Every link leaving the site reads as an unfinished site.** → Accepted, and
  it is why the page carries the lifecycle in its own prose rather than deferring
  it. The landing page already carries a note that the reference pages are read
  on GitHub today; that note stays true and stays visible.
- **The nav highlight breaks if the URL and the entry disagree.** → The explicit
  `permalink` removes the assumption, and the verification step opens the page
  and checks the sidebar marks it current.
- **A later change publishes the reference pages and forgets these links.** →
  The links are all in one file and named as such in that change's scope; the
  `docs-site` requirement this change modifies is the same one that change will
  modify again, so it is read before the work starts.
