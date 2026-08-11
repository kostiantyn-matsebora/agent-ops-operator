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

### D1. One page, not three

*Revised: the original decision was a three-page set — hero, C4 components,
domain model. That was scope invented around the request. The ask was a diagram;
a documentation programme is a different change and should be argued on its own.*

What survives from the original reasoning is the rule that stopped the three
being merged into one canvas, and it still applies to the one page that exists:
**one abstraction level per picture.** The hero shows the value path in the
system's own nouns; it does not also try to be a component diagram or an entity
model. When those are wanted they get their own pages and their own change, and
they inherit the vocabulary and the icon set settled here — which is the main
thing this change leaves behind besides the picture itself.

### D2. The `.drawio` file is the deliverable; renders are made on demand

*Revised twice, and the round trip is the point. It began as "`.drawio` is the
source of truth". Building the page argued that out of it — 105 cells, 19 icons
and a palette that must exist twice is not hand-maintainable, so a generator
became the source and the drawing an output. Descoping then argued it back: with
no committed exports and no README embed, a generator exists to keep two
artifacts in step that no longer both exist.*

So: the drawing is the deliverable, editable by anyone with the free draw.io app
and nothing else. Icons are embedded as `data:` URIs, so it has no external
assets. Rendered images are exported by hand when a rendered image is actually
wanted, and are not committed — which removes the whole class of problem where a
checked-in SVG silently disagrees with the drawing beside it.

What that costs, accepted deliberately: the dark variant does not survive. It
only ever existed as generator output, because an embedded `data:` URI cannot be
recoloured by a stylesheet — dark needed its own derived icon set. Anyone wanting
a dark rendering reworks the palette by hand.

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

### D5. No test; the freshness rule is prose

*Revised: the original decision was a Go test asserting that every CRD kind
appears as text in the domain page's SVG. That page is no longer in this change,
and the hero names only three kinds by design.*

A test that asserted "these three strings appear in an SVG" would pin a
composition choice rather than a fact, and would fail every time the picture was
legitimately reworded. So freshness is a routing rule in `CLAUDE.md` and the
requirement that every number on the page be checkable in the repo — the numbers
are where staleness would actually mislead.

If a domain page is drawn later, the coverage test becomes worth it again: there
the mapping from kinds to labels is total, so the assertion is about a fact
rather than a phrasing.

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
construction**: the page is authored as `mxGraphModel` XML and pushed with
`import-diagram`, because the layout is deliberate and dozens of individual
placement calls make it neither reviewable nor reproducible.

The fallback, if the server cannot be held: author the `mxGraphModel` XML
directly and open the file in the draw.io desktop app or the VS Code extension.
Nothing about the deliverable depends on the MCP server — it is an authoring
convenience, and the committed artifact is a plain file either tool can open.

### D7. The README is the only home

*Revised: the original decision added a `docs/architecture.md` to host pages 2
and 3. With one page and no reference prose to index, that file would exist to
hold a single image the README already carries.*

The diagram goes at the top of `README.md`, replacing the ASCII block. Seventeen
lines out, roughly five in, so the README gains headroom against its 150-line
budget rather than spending it. If a components or domain page is drawn later,
`docs/architecture.md` becomes the right home for the set and can be introduced
with them.

## Risks / Trade-offs

- **A stale diagram is believed; stale prose is skimmed.** → Every number on it
  is checkable against the repo and the check is recorded in the spec, so a
  reviewer verifies against a list rather than against memory. There is no test:
  for a single hand-composed picture naming three kinds, an assertion would pin
  a phrasing rather than a fact (D5).
- **No dark variant survives.** → Accepted. It was generator output, and the
  generator is gone with the rest of the descoped machinery. The dark palette is
  recorded in the spec so a later rendering does not start from guesses.
- **No committed render means the diagram is invisible to a README reader.** →
  Also accepted, and deliberate: the README keeps its ASCII block until someone
  exports an image, which is a separate decision. The alternative — committing an
  SVG — reintroduces two artifacts that can disagree.
- **`.drawio` XML merges badly.** → One page in one file, so a conflict is
  resolved by taking one side and re-opening the drawing, never by hand-merging
  mxGraphModel.
- **The palette is invented, not inherited.** → Recorded as hex values in the
  spec, so it is a decision revisitable in one place rather than re-guessed per
  diagram.
- **Deviating from "one diagram combining three views".** → The literal reading
  was rejected for density (D1); the delivered page is the combination, drawn in
  the system's own nouns.

## Migration Plan

Documentation-only: one new file, nothing to deploy, no version to bump.

1. Commit `docs/diagrams/agent-ops.drawio`.
2. Rollback is `git revert`; no other file observes it.

## Open Questions

- **When should a render be committed?** Today none is, so the README still
  shows the ASCII block and the diagram reaches nobody who does not open the
  file. The moment someone wants it in the README, a decision is needed about
  which formats to commit and how to keep them in step — that is the natural
  follow-up change.
- **Do the other two views get drawn?** The component and domain views were cut
  as invented scope. If they are wanted they inherit this vocabulary, and the
  CRD-coverage test becomes worth building at that point (D5).
- **Palette source.** A neutral accessible palette was picked. If a brand
  identity is chosen before going public, the drawing is the one place to change.
