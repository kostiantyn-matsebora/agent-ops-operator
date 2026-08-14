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
- **Theme changes.** If this page needs CSS, the page is wrong, not the theme.
- **An on-page table of contents.** Deferred with the reference pages, where
  page length makes it earn its place.

## Decisions

### 1. The page teaches the model; it does not enumerate the API

MassTransit's introduction is largely a hub: one or two sentences per concept,
each linking to the page that owns it. Ours borrows the shape but carries a
little more weight, for a reason worth naming: **its onward links leave the
site.** A reader who clicks lands in raw markdown on GitHub, so a page that was
purely a set of link stubs would be a menu with no meal.

The page therefore states, in its own prose:

- the structural idea — identity, wiring and execution are separate objects, and
  what an agent may do comes from the routing that wakes it, never from the agent
  itself;
- the lifecycle, end to end;
- what a swap at each seam does and does not change.

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

### 3. The lifecycle section is the page's centre of gravity

"Something fires; what actually happens?" is the question the reference pages
answer only in pieces — grouping in the source spec, admission in the concepts
page, threads in the channel contract, delivery in the state-durability rules.
Stating it once, in order, in prose, is the largest thing this page adds that no
existing document holds.

It stays at the level of what happens and why, naming no field: a signal arrives
on a source; policy is evaluated once above the fan-out; every Ready Pipeline
listing that source opens its own conversation; a conversation is serial, gets a
thread on each bound channel and its own pod; the answer is posted back by the
operator to every bound thread.

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
