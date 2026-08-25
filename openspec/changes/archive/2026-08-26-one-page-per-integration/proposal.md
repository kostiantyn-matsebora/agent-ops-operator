# One page per integration, and the render table stops being typed

## Why

**A reader who clicks a logo on the landing page lands nowhere.** The `works
with` group names eight things and not one of them is a link. The page makes a
promise the site cannot keep.

**What could answer it is written, and unpublished.** `docs/kubernetes.md`,
`docs/prometheus.md`, `docs/home-assistant.md` and `docs/telegram.md` are 1628
lines with no `nav.yml` entry, which was always the plan: *"The reference pages
under `docs/` … have no entry until the change that publishes them adds one."*

**They already carry the system names, and only the names.** `413bbbc` renamed
all four away from `<bundle>.md` — so the half of this change that is a rename
has landed, and what it left behind is four pages still titled *"Kubernetes
bundle (subchart)"*, still opening on `chart/charts/kubernetes/`, still carrying
no front matter and therefore no published URL at all.

**But publishing them as they are answers the wrong question.** They are written
for someone who has already decided and is now configuring — flag tables, render
tables, RBAC verbs. A person who clicked a Kubernetes mark is asking whether this
works with their cluster, and lands on `eventsAdapter.source.rules`.

**Each of those pages is two documents glued together.** Roughly 40% is adopter
explanation that has been buried by the other 60%:

> A Kubernetes Event is a point-in-time **fact** about an object whose lifecycle
> is churn by design. … `rules` is how you ask again.

That half is the integration page. The other half is a hand-written inventory of
what each flag renders.

**THE HAND-WRITTEN HALF GOES STALE, AND NOTHING CATCHES IT COMING BACK
EITHER.** Attributing objects to flags against the chart as it stands reproduces
`docs/kubernetes.md`'s component table on all five rows. It has not always, and
the way it stopped and started is the whole argument:

| Commit | Did | The wiring row said |
|---|---|---|
| `af5bb49` | gave each route its own `ServiceAccount`, so `pipelines.enabled` rendered `Pipeline/k8s-observe` **and** `ServiceAccount/agentops-k8s-observe` | one `Pipeline` — **wrong for two commits** |
| `84a2654` | an account exists only where something is bound to it, so the route SA stopped rendering | one `Pipeline` — **right again** |

- **Nobody repaired that row. The chart changed back underneath it.** The page
  was wrong, then correct, and no build, no check and no reviewer registered
  either transition.
- **The row `af5bb49` DID correct is the evidence for the other half.** It edited
  the events-lane row by hand in `84a2654`, in the same commit that silently
  fixed the wiring row by accident — so the table's accuracy on any given day is
  a fact about who was paying attention, not about the chart.

That is the argument, and the repo made it on itself: what is generated cannot
rot, and the component table is exactly the part that does.

## What Changes

- **NEW: `docs/integrations/`, four pages** — Kubernetes, Prometheus, Home
  Assistant, Telegram. Each is written for someone deciding whether to adopt it
  and then adopting it: **what you get**, **turn it on**, **the values you set**,
  **tune what reaches you**, **adopt it** — binding the bundle's parts into
  pipelines of their own — and last, what the bundle renders.
- **NO INDEX PAGE.** The sidebar already lists every integration page, so an
  index would be a second navigation written by hand and edited in step with the
  first forever. The names that are NOT pages get their destinations directly
  from the landing page's chips.
- **The page set is four of the five bundles.**
- **Every `works with` chip gets a destination**, and four of them are not new
  pages:

  | chip | lands on |
  |---|---|
  | Kubernetes · Prometheus · Home Assistant · Telegram | its integration page |
  | The console | `console-guide.md`, which already owns it |
  | any MCP server | `guides/toolsets.md`, which already owns MCPConfig, the allowlist, binding and the privilege split |
  | Cron schedules · your own | `guides/signal-adapter.md` — both mean "you declare the adapter and the source yourself", which is what that guide teaches |

- **NEW: a `renders` marker for the generator.** A page declares
  `<!-- generated: renders bundle=kubernetes -->` and the block is filled by
  rendering the chart and attributing objects to the component that produces
  them. CI's existing `--check` then fails on a stale table, which it cannot do
  today.
- **Component presets are DECLARED, never derived.** Flags are not independent —
  `mcp.enabled=true` refuses to render without `mcp.url` or
  `mcpServers.enabled` — so each component names the set that turns it on, under
  the same placeholder guard `PRESETS` already applies.
- **`prometheus` gains a preset.** It has none — `PRESETS` declares `tier1`
  through `tier4` and none of them enables it — so no generated content on the
  site exercises that bundle at all.
- **BREAKING (site content): `docs/kubernetes.md`, `docs/prometheus.md`,
  `docs/home-assistant.md` and `docs/telegram.md` MOVE under
  `docs/integrations/` and are rewritten there.** Their adopter prose moves with
  them, their component tables become generated blocks, and their exhaustive
  values are left to `helm show values` — which `installation.md` already gives
  as the reason not to hand-copy an inventory.
  - **The move costs no URL.** None of the four carries front matter, so none
    declares a `permalink` and none is a published site page — every inbound
    link today is a raw GitHub URL, and repointing those is already a task here.
- **Pages are named for the SYSTEM, not the subchart, AND THAT HALF HAS LANDED.**
  `413bbbc` renamed all four, so what is left is the two things a rename does not
  do: the routing rule still sends "a subchart's components or values" to
  `docs/<bundle>.md` — a pattern with no files behind it — and the pages still
  read as subchart documentation. The rename that produced the old rule,
  `vm-bundle.md` → `prometheus-bundle.md`, is why the rule binds the system name
  rather than the packaging's.

### Out of scope

- **MCP is a mechanism, not an integration.** "Any MCP server" is how an agent
  gets tools at all, and `guides/toolsets.md` teaches it end to end. A page would
  be a second telling.
- **Cron gets no page here.** `signals/cron` publishes an image and no chart
  renders it, so its page would teach "declare these two CRs yourself" beside
  four that say "set this flag". It is named on the index and left to a change
  that decides whether it should be a bundle at all.
- **The `claude` bundle is a RUNTIME, not an integration.** `chart/charts/claude/`
  is the fifth bundle and `docs/claude.md` already owns it, but the page shape
  here is the `Pipeline`'s three seams — what starts work, what may be reached,
  where it answers — and a runtime is the fourth thing: what EXECUTES the agent.
  Filling that shape for it would mean omitting three of four sections, or
  inventing a fifth question for one page. It keeps its reference page.

## Capabilities

### Modified Capabilities

- `docs-site`: the integration page set and what each page owes, the `works with`
  group gaining destinations, and the generation requirement widening from custom
  resource templates and examples to **what a bundle renders**.
- `documentation-structure`: the routing rule currently sends a subchart's
  components or values to `docs/<bundle>.md` and requires bundle pages to be
  named by their bundle. Both stop being true.

**No new capability.** `docs-site` already owns every other page set on the site
— the Introduction, Getting started, Installation, the Console page, the guides —
so a page set of its own would be the odd one out.

## Impact

### Reference docs

- **`docs/kubernetes.md`, `docs/prometheus.md`, `docs/home-assistant.md`,
  `docs/telegram.md`** — moved under `docs/integrations/` and rewritten there.
  This is the change.
- **`docs/claude.md`** — untouched, and named on the index as the runtime it is.
- **`docs/CLAUDE.md`** — the pages table gains the integrations rows. Its
  components table gains the `renders` marker.
- **`.claude/rules/documentation.md`** — its routing table sends "a subchart's
  components or values" to `docs/<bundle>.md`, which names no file since
  `413bbbc`, and sends a chart value a guide shows to the generator. Both rows
  change. Two more sentences name a bundle page: the docs-task line's *"a bundle
  page"* and the values split's *"a SUBCHART's to that bundle's own page"*.
- **`docs/CHANGELOG.md`** — no entry. This ships no chart version and no image,
  so an adopter has nothing to upgrade.
- **`docs/contracts.md`, `docs/concepts.md`** — checked for sentences pointing at
  a bundle page, corrected where they do.

### The adopter site

- **`docs/index.md`** — the `works with` chips gain their destinations. The
  `Run it` list currently links four bundle pages on GitHub and must point at the
  new pages instead.
- **`docs/installation.md`** — its "Enable a bundle" table links all four bundle
  pages as GitHub URLs. They become internal links, and the page keeps owning the
  parent chart's values while the integration pages own their own.
- **`docs/getting-started.md`, `docs/introduction.md`, `docs/console-guide.md`,
  `docs/guides/*.md`** — read for sentences naming a bundle page or promising
  values it held. `guides/toolsets.md` gains nothing but is the MCP chip's
  destination, so it is verified rather than assumed.
- **`docs/_data/nav.yml`** — one group, four entries. The group IS the index.
- **`README.md`** — one row of the *Where to go next* table carries five links,
  four of which are these pages and the fifth `docs/claude.md`, which stays.
  Inside a **215**-line budget, of which it currently spends 204.

### Code and assets

- `.github/scripts/docs-generate.py` — the `renders` marker, the per-component
  preset declaration, a `prometheus` preset, and letting the block
  dispatcher return a table rather than only a fenced `yaml` block.
- `docs/integrations/` — four pages.
- `.github/scripts/docs_diagrams.py` — one diagram per integration, both themes,
  written by the same generator run.
- Nothing under `chart/`, `platform/` or any component. No image, no chart
  version.
