# One page per integration, and the render table stops being typed

## Why

**A reader who clicks a logo on the landing page lands nowhere.** The `works
with` group names eight things and not one of them is a link. The page makes a
promise the site cannot keep.

**What could answer it is written, and unpublished.** `k8s-bundle.md`,
`prometheus-bundle.md`, `ha-bundle.md` and `telegram-bundle.md` are 1570 lines
with no `nav.yml` entry, which was always the plan: *"The reference pages under
`docs/` … have no entry until the change that publishes them adds one."*

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

**THE HAND-WRITTEN HALF IS ALREADY GOING STALE, AND NOTHING CATCHES IT.**
Attributing objects to flags against the current chart reproduces
`k8s-bundle.md`'s component table on four of five rows. The fifth is wrong:
`pipelines.enabled` now also renders `ServiceAccount/agentops-k8s-observe`, added
by `runtime-and-identity-are-wiring`, whose documentation section is ten tasks
and names **no bundle page** — while its task 10.4 re-runs the generator, so
every GENERATED block on the site is corrected for free.

That is the argument, and the repo made it on itself: what is generated cannot
rot, and the component table is exactly the part that does.

## What Changes

- **NEW: `docs/integrations/`, an index plus four pages** — Kubernetes,
  Prometheus, Home Assistant, Telegram. Each answers the same four questions in
  the same order: what starts work here, what the agent may reach, where it
  answers, and what turning it on costs.
- **The page set is exactly the four bundles, and the index covers the rest.**
  It carries the seam table — every integration lit against `signalSourceRefs` /
  `toolsets` / `channelRefs`, which is the Pipeline's own shape and the one the
  site's presentation already narrates — including the rows that are not pages:
  the console, cron and bring-your-own.
- **Every `works with` chip gets a destination**, and four of them are not new
  pages:

  | chip | lands on |
  |---|---|
  | Kubernetes · Prometheus · Home Assistant · Telegram | its integration page |
  | The console | `console-guide.md`, which already owns it |
  | any MCP server | `guides/toolsets.md`, which already owns MCPConfig, the allowlist, binding and the privilege split |
  | Cron schedules · your own | the integrations index |

- **NEW: a `renders` marker for the generator.** A page declares
  `<!-- generated: renders bundle=k8s-bundle -->` and the block is filled by
  rendering the chart and attributing objects to the component that produces
  them. CI's existing `--check` then fails on a stale table, which it cannot do
  today.
- **Component presets are DECLARED, never derived.** Flags are not independent —
  `mcp.enabled=true` refuses to render without `mcp.url` or
  `mcpServers.enabled` — so each component names the set that turns it on, under
  the same placeholder guard `PRESETS` already applies.
- **`prometheus-bundle` gains a preset.** It has none, so no generated content on
  the site exercises it at all.
- **BREAKING (site content): `docs/k8s-bundle.md`, `docs/prometheus-bundle.md`,
  `docs/ha-bundle.md` and `docs/telegram-bundle.md` are DELETED.** Their adopter
  prose moves to the integration pages, their component tables become generated
  blocks, and their exhaustive values are left to `helm show values` — which
  `installation.md` already gives as the reason not to hand-copy an inventory.
- **Pages are named for the SYSTEM, not the subchart.** `prometheus.md`, not
  `prometheus-bundle.md`. The rename that produced today's rule —
  `vm-bundle.md` → `prometheus-bundle.md` — happened because the page was named
  after the packaging. A page named for the system would have survived it
  untouched, and an adopter's bookmark with it.

### Out of scope

- **MCP is a mechanism, not an integration.** "Any MCP server" is how an agent
  gets tools at all, and `guides/toolsets.md` teaches it end to end. A page would
  be a second telling.
- **Cron gets no page here.** `signals/cron` publishes an image and no chart
  renders it, so its page would teach "declare these two CRs yourself" beside
  four that say "set this flag". It is named on the index and left to a change
  that decides whether it should be a bundle at all.

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

- **`docs/k8s-bundle.md`, `docs/prometheus-bundle.md`, `docs/ha-bundle.md`,
  `docs/telegram-bundle.md`** — deleted. This is the change.
- **`docs/CLAUDE.md`** — the pages table gains the integrations rows. Its
  components table gains the `renders` marker.
- **`.claude/rules/documentation.md`** — its routing table sends "a subchart's
  components or values" to `docs/<bundle>.md`, and sends a chart value a guide
  shows to the generator. Both rows change.
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
- **`docs/_data/nav.yml`** — one group, five entries.
- **`README.md`** — six links to bundle pages, inside a 150-line budget.

### Code and assets

- `.github/scripts/docs-generate.py` — the `renders` marker, the per-component
  preset declaration, a `prometheus-bundle` preset, and letting the block
  dispatcher return a table rather than only a fenced `yaml` block.
- `docs/integrations/` — the index and four pages.
- Nothing under `chart/`, `platform/` or any component. No image, no chart
  version.
