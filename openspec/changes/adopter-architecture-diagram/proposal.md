## Why

The project is preparing to go public, and the only picture it has is a 16-line
ASCII sketch at the top of `README.md`. That sketch answers one question — how a
signal becomes a pod — and it answers it for someone who already believes. A
first-time reader arrives with three questions in a fixed order: **what is this
for**, **what are the pieces**, and **what is the model I have to learn**.
Nothing in the repository answers the first, `docs/contracts.md` answers the
second only in prose, and `docs/concepts.md` answers the third across 519 lines
of reference text. Eleven CRDs and eight processes are a lot to hold from prose
alone; the shape is the product, and the shape is currently invisible.

This is the first step of adopter documentation, and deliberately the first:
every later adopter page (getting started, the wiring walkthrough, the adapter
author guide) will reference these pictures, so drawing them settles the visual
vocabulary — names, colours, boundaries — before the prose that leans on it
gets written.

## What Changes

**A committed diagram set** — one `draw.io` source file, `docs/diagrams/agent-ops.drawio`,
holding three pages that answer the three questions in order, plus committed SVG
exports rendered from it:

- **Page 1 — "Why" (the pitch).** The value story as a picture: an alert or a
  question enters, a Conversation is created with its own chat thread, an
  isolated agent pod answers it, and a human approves from their phone. Carries
  the three claims that distinguish the product — *addressable* (`/<pipeline> <task>`
  reaches a named agent), *isolated* (one pod per conversation, serial, capped),
  *least-privilege by construction* (capabilities come from wiring, the manager
  reads no Secret). This page replaces the ASCII block in `README.md`.
- **Page 2 — C4 component view.** Level 3 over one container boundary per
  process: the manager and its internal components (`httpapi`, the reconcilers,
  `chat` ops queue, `dispatch`, `ingest`), the adapter processes, the runtime
  pods, the Kubernetes API, and the external systems (Alertmanager, Telegram,
  the LLM). Every edge labelled with the actual protocol and direction —
  `GET /work` long-poll, `GET /channel/ops` long-poll, `POST /signal/inbound`,
  `GET /activity/stream` SSE, watch/patch against the API — because who calls
  whom is the single most misread thing about an operator with an HTTP surface.
- **Page 3 — Domain model.** The eleven CRD kinds with their reference and
  ownership edges and cardinalities, laid out so that `Pipeline` is visibly the
  hub: sources × channels + profile + capabilities. Shows what carries wiring
  and what does not, which is the one concept an adopter must get right and the
  one the prose has had to repeat in five places.

**A new documentation page** `docs/architecture.md` that hosts pages 2 and 3
with the paragraph of orientation each needs, and is listed in the README's
Documentation index. Reference prose stays in `docs/concepts.md` and
`docs/contracts.md` — the new page is the visual index into them, not a fourth
copy of their content.

**A rendering and freshness rule**, written into the spec and `CLAUDE.md`: the
`.drawio` file is the source of truth, the SVGs are generated and committed
beside it (a repository that renders on GitHub with no build step is the same
property `go:embed` buys the console), and a change that alters a CRD kind, an
adapter contract endpoint, or a process boundary updates the affected page in
the same commit.

**Dark-mode-safe exports.** GitHub renders README images on a white or dark
canvas depending on the reader's theme; a diagram with a transparent background
and black strokes is unreadable on one of them. Each page exports a light and a
dark SVG, referenced through `<picture>`/`prefers-color-scheme`.

Not in scope: rewriting `docs/concepts.md` or `docs/contracts.md`, a
getting-started guide, a website, a logo, or any change to product behaviour.
This change adds pictures and the one page that frames them.

## Capabilities

### New Capabilities

- `architecture-diagrams`: the committed diagram set — the three views and what
  each must show, `.drawio` as source of truth with generated SVG beside it,
  theme-safe exports, the naming and colour vocabulary shared across pages, and
  the rule that binds a diagram page to the code it depicts.

### Modified Capabilities

- `documentation-structure`: the README's architecture diagram becomes an
  embedded generated image rather than an ASCII block, and the `docs/` page set
  gains `docs/architecture.md` — a page that is neither CRD reference, contract
  reference, nor a bundle page, so the existing routing requirement does not
  cover it. The `CLAUDE.md` routing table gains a row for diagram updates, and
  the README's 150-line budget is restated against a diagram that now costs a
  handful of lines instead of sixteen.

## Impact

- **New files**: `docs/diagrams/agent-ops.drawio` (source), six generated SVGs
  (`why`, `components`, `domain` × light/dark), `docs/architecture.md`.
- **Modified**: `README.md` (ASCII block → embedded hero image; Documentation
  index gains a row), `CLAUDE.md` (map entry for `docs/diagrams/`, routing table
  row, the freshness rule as an invariant), `CHANGELOG.md` (documentation entry;
  no chart version change).
- **Tooling dependency**: authoring uses the draw.io MCP server, which is
  **currently disconnected from this workspace**. The source format is chosen so
  that this is a convenience and not a blocker — `.drawio` is XML and its
  export is reproducible from the desktop app or the VS Code extension. The
  design records the fallback path.
- **No Go code, no CRD, no chart change.** Nothing in `api/`, `internal/`, any
  adapter module, or `chart/` is touched, so the build and test matrix is
  unaffected.
- **Risk to name**: a diagram is a second source of truth about the
  architecture, and a stale one is worse than none — it will be believed. The
  freshness rule and the per-page "what this must show" list in the spec exist
  to make staleness reviewable rather than invisible.
