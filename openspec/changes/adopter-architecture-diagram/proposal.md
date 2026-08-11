## Why

The project is preparing to go public and the only picture it has is a 16-line
ASCII sketch at the top of `README.md`. That sketch answers one question — how a
signal becomes a pod — and it answers it for someone who already believes.
Nothing in the repository shows a first-time reader what the product is *for*,
what makes it different from the automation they already run, or why an agent
with cluster access is safe to adopt.

This change draws that picture.

## What Changes

**One diagram**, `docs/diagrams/agent-ops.drawio`, arguing the product in its
own vocabulary:

- any signal wakes it, including a non-infrastructure one, so the domain range
  is shown rather than asserted;
- the three declarations appear as their real CRD kinds — `Pipeline`,
  `AgentProfile`, `MCPToolset` — each subtitled with the question it answers,
  beside a real copy-pasteable `Pipeline` manifest;
- a verb ladder defines what "takes care of it" means, with **Acts** marked
  conditional because the shipped defaults are read-only;
- the three extension points are drawn as sockets wired to what replaces them,
  so pluggability is visible rather than claimed.

The file is self-contained: its 19 icons are embedded as `data:` URIs, so it
opens and renders anywhere with no external assets.

### Explicitly not in this change

Earlier drafts of this proposal grew a documentation programme around the
diagram — a C4 component page, a redrawn domain model, a `docs/architecture.md`,
committed SVG exports, a generator script, a Go test asserting CRD coverage, and
README and CHANGELOG edits. That was scope invented around the request rather
than the request itself.

Exports are produced from the drawing on demand, by hand, when a rendered image
is actually needed. Consequently this change makes **no edit to `README.md`**:
the ASCII block stays until there is an exported image to replace it with, which
is a separate decision.

## Capabilities

### New Capabilities

- `architecture-diagrams`: the diagram — what it must show and argue, the visual
  vocabulary it establishes, and the rule that every claim on it is checkable in
  the repo.

### Modified Capabilities

None. Nothing about how documentation is organised changes: no page moves, no
new page, and the README is untouched.

## Impact

- **New**: `docs/diagrams/agent-ops.drawio`.
- **No other file changes.** No Go, no CRD, no chart, no README — the build and
  test matrix is untouched.
- **Known limitation, accepted**: the dark variant was generator output and is
  not retained. The committed drawing is the light version; a dark rendering
  would mean reworking the palette by hand.
- **Risk to name**: a stale diagram is worse than none, because it will be
  believed. Mitigated by keeping every number on it checkable against the repo,
  and by the diagram naming real kinds so a rename is visible.
