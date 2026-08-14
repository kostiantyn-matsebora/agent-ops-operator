## Why

The site has exactly one page. An adopter who lands on it gets the pitch, the
diagram and a set of links — and every one of those links leaves for GitHub,
where the reference pages are raw markdown written for someone who has already
decided. There is nothing between "what is this" and "here is every field of
every CRD".

That missing middle is the page an adopter actually reads first: the one that
names the model's few moving parts, says how a signal becomes an answer, and
sends them onward knowing what they are looking at. MassTransit's concepts
introduction is the shape — a short orientation whose job is to make the
reference material legible, not to replace it.

## What Changes

- **A new site page, `docs/introduction.md`**, published at `/introduction/` and
  written for an adopter evaluating the operator. A short opening and **two
  sections**, deliberately:
  - the opening states what agent-ops is and the one structural idea that
    separates it from a bot or a runbook — identity, wiring and execution are
    separate objects, so what an agent MAY DO comes from the routing and never
    from the agent;
  - **Understand the concepts** — the model's parts as blocks, one per part, each
    answering a single question in a sentence or two, with the reference linked
    for the detail;
  - **Follow the guides** — the task-shaped walkthroughs, grouped, so a reader
    leaves knowing what to read next. **None is written yet**, so none is a link
    and the page says so; a roadmap that looks clickable is worse than none.
  - Nothing else. Material that fits neither section belongs in a guide or a
    reference page — including the signal-to-answer lifecycle, which is the first
    guide to write rather than a wall of prose in an introduction.
- **One navigation entry** in `docs/_data/nav.yml`, under *Start here*.
- **The landing page points at it**: `docs/index.md`'s *Where to start* leads with
  the Introduction rather than with the README.
- **The site's deliverable set grows by one page.** The reference pages under
  `docs/` are STILL untreated — no front matter, no navigation entry, still
  served as raw markdown, still linked where they live. Publishing them remains
  its own change, and this one does not start it.

Explicitly NOT in this change:

- **No reference prose.** The Introduction states no CRD field, no HTTP endpoint,
  no install command, and no values key. Where it would have to, it links.
- **No content in the theme.** The site gains two presentational components — a
  card grid and a callout — because a list of seven concepts reads as a wall.
  Pages name them with the markdown engine's attribute syntax (`{: .ao-cards}`),
  so the words stay in the page and the rules stay wordless. They are components
  any later page can use, not styling for this one; no layout or include is
  touched.
- **No guides.** They are listed, not written. Writing them is the next change,
  and the listing is what says which order to write them in.
- **No on-page contents and no edit link.** Both belong to the change that
  publishes the reference pages, where a long page makes them earn their place.

## Capabilities

### New Capabilities

None. The published site is one capability and this page belongs to it.

### Modified Capabilities

- `docs-site`: the site's deliverables become the theme, the landing page **and
  the Introduction**, and the site gains a requirement stating what that page is
  for and what it must not restate. The reference pages stay untreated, so that
  half of the existing requirement is preserved verbatim.

## Impact

- **New** — `docs/introduction.md`.
- **Edited** — `docs/_data/nav.yml` (one entry), `docs/index.md` (the first path
  onward), `docs/assets/css/agentops.css` (the three components),
  `CLAUDE.md` (the `docs/` map line naming the site's pages).
- **Untouched** — every layout, include and data file other than the navigation;
  every reference page; `README.md`; the chart; all Go modules.
- **Build** — none. GitHub Pages builds `docs/` from `master`, the page takes the
  layout `jekyll-default-layout` already assigns, and no plugin, workflow or Ruby
  toolchain is added.
