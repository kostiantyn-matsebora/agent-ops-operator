## Context

`README.md` is at its 150-line budget and its only picture is a 17-line ASCII
block (lines 10–26) showing signal → Conversation → runtime pod. It is accurate
and it is not enough for a public audience: it never says what the product is
*for*, it shows three of eight processes, and it shows none of the eleven CRD
kinds that a person has to learn before they can wire anything.

The material to draw already exists and is unusually well specified — `CLAUDE.md`
holds the terminology and invariants, `docs/concepts.md` the CRD semantics,
`docs/contracts.md` the endpoints, and `openspec/specs/` forty-odd capability
specs. Nothing needs to be discovered. What is missing is a visual vocabulary
and a place to put it, and settling those now is why this is the first adopter
change rather than a later polish pass: every subsequent page will reuse the
names, colours and boundaries chosen here.

Constraints that shape the solution:

- **No build step in the reader's path.** The repository renders on GitHub
  as-is. Whatever the source format, what a reader loads must be a committed
  file — the same property `go:embed` buys the console's SPA.
- **No Go, no chart, no CRD change.** This is documentation; the build and test
  matrix must be untouched apart from one optional new test.
- **The draw.io MCP server is disconnected from the current workspace.** The
  format choice has to survive that.
- **Eight processes, eleven kinds.** Density is the real design problem, not
  aesthetics.

## Goals / Non-Goals

**Goals:**

- One entry-point picture that makes a stranger want to keep reading, drawn from
  the real nouns rather than marketing abstractions.
- A C4-style component view that gets **call direction** right, because
  "adapters and runtimes call the manager; the manager never calls them" is the
  single most misread property of this architecture.
- A domain view that makes `Pipeline`-is-the-wiring obvious at a glance, so the
  prose in five files stops carrying that load alone.
- A shared visual vocabulary — shapes, colours, line styles, legend — reusable
  by every later adopter page.
- A staleness story that is checkable, not merely promised.

**Non-Goals:**

- Rewriting `docs/concepts.md` or `docs/contracts.md`. The new page is an index
  into them.
- A getting-started guide, a website, a logo, or social preview cards.
- Diagramming internals below component level (no sequence diagrams, no state
  machines, no per-function structure).
- Any change to product behaviour, CRDs, chart, or contracts.

## Decisions

### D1. Three pages with a combining hero — not one merged canvas

The request is a diagram combining C4 components, the domain model, and the
buyer's case. Drawn literally as one canvas, that fails: C4's one useful rule is
one abstraction level per diagram, and runtime processes and CRD types are
different levels. Eleven kinds plus eight processes plus a value narrative on
one sheet is roughly seventy nodes, which is a poster nobody reads.

The combination happens on **page 1**, and it is a real combination rather than
a compromise: the hero is laid out as the buyer's story — something happens, an
agent takes care of it, and you can see exactly what "takes care of it" means —
but every process on it is real and every object it names is a real CRD kind. It
is the buyer's view spoken in the system's own vocabulary, and pages 2 and 3 are
zoom-ins of its middle and its nouns.

- *Alternative — one literal poster*: rejected for the density above. If wanted
  later it is cheap to add as a fourth page composited from the three, because
  they will already share a vocabulary.
- *Alternative — three separate files*: rejected. One `.drawio` file with three
  `<diagram>` pages keeps the vocabulary physically together and makes "did you
  update the others?" a question the reviewer can answer without hunting.

### D2. The GENERATOR is the source of truth; `.drawio` and SVG are both outputs

*Revised during implementation — the original decision named the `.drawio` file
as the source, and building the hero disproved it.*

The hero is 105 cells, 19 hand-authored line-art icons embedded as data URIs,
and a palette that has to exist twice (light and dark). Hand-maintaining that in
the draw.io app is not realistic: a type-scale change touches every cell, and
the dark variant is not an edit but a *derivation* — the icons bake their stroke
colour in, so dark needs its own generated set.

So the tree is:

```
docs/diagrams/
  build-why.py        <- SOURCE: layout, copy, palette, both themes
  icons/*.svg         <- SOURCE: 19 line-art icons, light strokes
  agent-ops.drawio    <- OUTPUT: light rendering, editable in the app
  *.svg               <- OUTPUT: committed exports, light + dark
```

`python build-why.py light|dark` emits the page; draw.io is used only to render
and export. The dark icons are generated from the light ones by substitution
(`#1A1A1A`→`#E6EDF3` strokes, `#FFFFFF`→`#0D1117` knockouts, amber untouched),
so there is exactly one drawn version of each icon.

- *Alternative — keep `.drawio` authoritative*: rejected. It would mean either
  hand-editing two 105-cell pages in lockstep, or accepting that light and dark
  drift. The whole reason dark needs regenerating is that an embedded `<image>`
  cannot be recoloured by a stylesheet.
- *Consequence for contributors*: editing `agent-ops.drawio` in the app is fine
  for a one-off nudge, but the change must be folded back into `build-why.py`
  or the next generation overwrites it. That rule belongs in `CLAUDE.md`.

### D3. Two exports per page, selected by `prefers-color-scheme`

GitHub renders README images against a light or dark canvas depending on the
reader's theme, and a transparent-background diagram with dark strokes is
unreadable on one of them. Each page exports `<page>-light.svg` and
`<page>-dark.svg` with an **opaque** background, referenced through the
`<picture>` element GitHub supports:

```html
<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/diagrams/why-dark.svg">
  <img alt="…" src="docs/diagrams/why-light.svg">
</picture>
```

- *Alternative — one SVG with an internal `@media (prefers-color-scheme)` style
  block*: elegant, and unreliable through GitHub's image proxying and `<img>`
  embedding. Rejected for a reader-facing surface.
- *Alternative — one theme-neutral diagram* (mid-grey on transparent): readable
  nowhere in particular. Rejected.

Export settings are pinned in the spec: **text exported as text, never as
curves** — so the diagram is searchable, selectable, screen-reader-reachable,
and greppable by the freshness test (D5).

### D4. Line art on two colours — not filled boxes

*Revised during implementation. The original table specified pale-blue filled
rectangles, a cylinder for the API server, a hexagon-tab for CRD kinds and a
legend on every page. The first domain page built to it was rejected on sight:
eleven near-identical rectangles read as an ER diagram, which is exactly what a
page meant to interest a stranger must not be.*

The vocabulary is now **line-art icons in one ink colour plus a single accent**,
grouped by dotted zones, with the only filled surfaces being the cluster
boundary, the code card and the extension tiles. The full palette (both themes)
lives in the spec rather than here, so there is one authoritative copy.

Three consequences worth stating as decisions:

- **One accent, not a per-role palette.** A four-hue scheme was tried and drops
  out: with icons doing the identifying, colour is free to carry a single
  meaning — *this is the thing to notice*. It marks the accent detail in every
  icon, the conditional rung, the sockets and the extension tiles, and nothing
  else.
- **No icon library exists for this.** draw.io ships AWS, Azure, GCP and Cisco
  sets; using any of them on a vendor-neutral Kubernetes operator would be
  actively misleading. The 19 icons are hand-drawn SVG embedded as `data:` URIs
  — which is also why dark needs a derived set (D2).
- **The legend rule is relaxed.** Required on `components` and `domain`, where
  shapes encode meaning; forbidden on the hero, where every element is labelled
  and a legend would be furniture on the one page that must read in seconds.

### D5. Freshness is enforced by a test for the half that can be tested

A diagram nobody re-draws becomes a confident lie. Two mechanisms, and the
design is honest that they cover different amounts:

1. **Testable — CRD coverage.** A Go test reads `docs/diagrams/domain-light.svg`
   and asserts that every kind registered in `api/v1alpha1`'s scheme appears as
   a text node. One direction only: every kind must be drawn; the diagram may
   contain words that are not kinds. This catches the exact failure that matters
   — a twelfth kind ships and the picture silently claims there are eleven — and
   it is cheap because D3 guarantees SVG text is text. It lives beside the
   existing precedent for pinning documentation facts in tests
   (`internal/integration/charttemplate_test.go`).
2. **Prose only — everything else.** Component boundaries, endpoints and call
   directions get a routing-table row in `CLAUDE.md` and a review expectation,
   not a test. Asserting the route set against page 2 would mean scraping mux
   patterns and matching them to arrow labels; the brittleness would exceed the
   value, and a test that gets disabled is worse than a rule that gets read.

- *Alternative — generate the diagrams from code*: the domain page is close to
  derivable from the CRD types, and a generator would never go stale. Rejected
  for now: generated layout is unreadable at this node count, and the hero and
  component pages are not derivable at all, so it would buy one of three pages
  at the cost of a build step this repository does not otherwise have.

### D6. Authoring is MCP-driven, and the one real tooling constraint is session ownership

Authoring uses the draw.io MCP server. The constraint worth recording is not
"the server may be missing" — it is that **only one session can own it at a
time**. The configured launch command is

```
pwsh -Command "docker rm -f drawio-mcp-server; docker run -i --rm --init --name drawio-mcp-server ..."
```

so whichever session starts the server second destroys the first session's
container and, with it, the `docker run -i` process holding that session's stdio
pipe. The symptom is misleading: `docker ps` shows a healthy container on ports
3000/3333 while the session that was using it has no `mcp__drawio__*` tools at
all. The recovery is `/mcp` → reconnect, which re-runs the command and rebinds
the container to the reconnecting session. This happened once during planning.

Within a session, the authoring method is **XML import, not shape-by-shape
construction**: pages are authored as `mxGraphModel` XML and pushed with
`import-diagram`, because layout on pages 2 and 3 is deliberate and dozens of
individual placement calls make it neither reviewable nor reproducible. Export
runs through `export-diagram` with an explicit `background` and
`embed_xml: false`.

Fallbacks, in order, if the server cannot be held:

1. Author the `mxGraphModel` XML directly and export with the draw.io desktop
   CLI (`drawio -x -f svg -p <page>`) or the VS Code extension — the same XML,
   a different exporter.
2. Ship pages 2 and 3 as Mermaid fenced blocks inside `docs/architecture.md`
   (GitHub renders them natively, no export step, no staleness gap) and only the
   hero as draw.io.

Fallback 2 is a real degradation — it loses layout control and the shared
vocabulary on two of three pages — so it is a last resort, recorded so the
change cannot become blocked on a tool.

### D7. `docs/architecture.md` is a visual index, not a fourth reference page

The existing `documentation-structure` spec routes CRD semantics to
`docs/concepts.md`, contracts to `docs/contracts.md`, and one page per bundle.
The new page fits none of those, so the spec is amended rather than stretched.
Its content rule is explicit: each embedded diagram gets one orienting paragraph
and links into the page that owns the detail. Any sentence on it that explains a
CRD field or an endpoint belongs somewhere else — that rule is what keeps a
fourth copy of the reference from growing here, which is the failure mode
`documentation-structure` was written to stop in the first place.

The hero lives at the top of `README.md`, replacing the ASCII block. Seventeen
lines out, roughly five in — the README gains headroom against its budget rather
than spending it.

### D8. The hero's positioning, and the words that carry it

Settled in review, after four rejected framings. Recorded because each rejection
removed a specific wrong claim, and re-deriving that is expensive.

**Headline:** *Something happens. An agent takes care of it.*

**Subhead:** *A signal wakes it, your prompt tells it what to do, your YAML
decides what it may touch — whether that's a crashlooping pod or the hallway
lights.*

What each rejection established, in order:

1. **"Agents you can address"** describes a mechanism, not a benefit.
2. **Alert must not lead.** `alert` is one of four kinds (`alert | job | task |
   chat`). Leading with it silently narrows the product to alerting.
3. **"Autonomous operations" is not a differentiator.** Ansible, StackStorm and
   self-healing controllers are all autonomous operations with no AI in them.
4. **Nor is "judgment about the unforeseen."** That over-narrows to diagnosis.
   Turning on a light is not judgment about an unpredicted failure, and it is
   the product working exactly as designed.

What survived as the actual AI-supplied element: **the behaviour is authored in
prose and becomes executable.** Automation makes you write the remedy in code;
here it is written in English and fenced by grants. That, not reasoning, is why
one product serves a crashlooping pod and a hallway light.

Which reduces the whole thing to three inputs, and they map one-to-one onto the
CRDs the hero's middle zone already draws:

| Input | Means | CRD |
|---|---|---|
| Signal | what wakes it | the `Pipeline` claim over a `SignalSource` |
| Prompt | what it should do | `AgentProfile` |
| Capabilities | what it may touch | `MCPToolset` / `MCPConfig`, bound by `Pipeline` |

**The honesty constraint on the headline.** "Takes care of it" implies
resolution, and the defaults do not resolve: `k8s-observability` is on,
`k8s-admin` is off, `rbacMode` defaults to `readonly`. Out of the box the agent
investigates and explains but cannot act. The subhead therefore has to carry
"what it may touch", and the hero's right-hand column is the verb ladder that
defines the claim — **investigates → explains → acts → asks** — with *acts*
marked as granted, not assumed. A hero that implied autonomous remediation by
default would be the one overclaim this change must not ship.


## Risks / Trade-offs

- **A stale diagram is believed; stale prose is skimmed.** → D5's coverage test
  for kinds, the `CLAUDE.md` routing row for the rest, and a per-page "what this
  must show" list in the spec so a reviewer can check the picture against a list
  instead of against their memory.
- **Density kills the component page.** Eight processes with every edge labelled
  is already busy. → A stated node budget per page in the spec, external systems
  collapsed to one box per system, and the manager's internals limited to the
  five packages a reader must know (`httpapi`, controllers, `chat`, `dispatch`,
  `ingest`) rather than all of `internal/`.
- **`.drawio` XML merges badly.** → Pages are separate `<diagram>` elements so
  conflicts localise to the page edited; in practice conflicts are resolved by
  re-exporting rather than by merging XML, and the SVGs are regenerated
  artefacts so their conflicts are never resolved by hand.
- **Committed binaries-that-aren't-binaries bloat the repo.** → A per-file SVG
  size budget in the spec; text-as-text export and no embedded raster keep
  these in the tens of kilobytes.
- **The palette is being invented, not inherited.** → Recorded as hex values in
  the spec so it is a decision that can be revisited in one place, rather than
  re-guessed per diagram.
- **Choosing three pages over one poster is a deviation from the literal
  request.** → Mitigated by making the hero itself the combination, and by
  noting that a composited poster page is cheap to add once the vocabulary
  exists (D1).

## Migration Plan

Documentation-only; there is nothing to deploy and no version to bump.

1. Reconnect the draw.io MCP server, or fall down D6's ladder.
2. Land the `.drawio` source and all six SVGs in one commit with
   `docs/architecture.md`, so the page is never live against missing images.
3. Swap the README block and add the Documentation index row in the same commit
   — the README must not reference an image that is not yet there.
4. `CLAUDE.md` map entry, routing row, and freshness rule; `CHANGELOG.md`
   documentation entry under `Unreleased` (no chart version).
5. Rollback is `git revert` of the commit; nothing else observes these files.

## Open Questions

- **Does the hero deserve a PNG twin?** A 1280×640 raster is what GitHub social
  previews, conference slides and blog posts need, and SVG serves none of them.
  Deferred rather than decided: it costs one export and a size budget, but it
  adds a second generated artefact per theme with no test behind it.
- **Should `docs/architecture.md` carry a fourth "how a request flows" sequence
  diagram?** It is the natural next question after page 2, and it is also the
  thing most likely to go stale. Left out of this change deliberately; worth
  revisiting once the vocabulary has survived one real update.
- **Palette source.** The change picks a neutral accessible palette. If a brand
  identity is chosen before going public, these six SVGs are the first things to
  re-export.
