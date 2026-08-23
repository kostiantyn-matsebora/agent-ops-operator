# Adopter guides: seven how-to pages across four tiers

## Why

The site already declares this and admits it is missing:

```
  docs/introduction.md:79   ## Follow the guides
  docs/introduction.md:81   Guides are being written. **There are none yet.**
```

An adopter who finishes Getting started has a working demo install and nowhere
to go but reference. `concepts.md` is 1,424 lines and `contracts.md` is 990 —
both correct, neither task-shaped. Nothing in the project says *"here is how you
add your own agent"*, which is the first thing anyone will want.

## What Changes

**Seven how-to pages, four tiers, in learning order:**

| Tier | Page | Teaches |
|---|---|---|
| 1 | Add your own pipeline | `Pipeline` — the wiring, built from objects the install ALREADY has |
| 1 | ↳ Add your own agent | `AgentProfile` — identity only, and the route that reaches it |
| 1 | ↳ Run your agent from a repository | `repository`, `.claude/agents/<name>.md`, the deploy key |
| 2 | Give your agent tools | `MCPToolset`, `MCPConfig`, how `toolsMode` composes against the definition |
| 3 | Wake agents from your own system | `/signal/inbound`, the `SignalAdapter` CR, self-exclusion |
| 3 | Answer on your own chat surface | `/channel/*`, the ops queue, typed messages |
| 4 | Run agents on your own backend | the work contract, and what a runtime is trusted to enforce |
| — | `docs/cr-reference.md` | every field of every kind, generated |

**A TITLE NAMES WHAT THE READER GETS, NEVER THE KIND IT IS BUILT FROM.** "Create
a Pipeline" is the implementation talking. The kind still appears in the page's
first sentence and in its `description`, so nothing is lost to search, and the
URL keeps the technical slug for the same reason.

**THE PIPELINE COMES FIRST, and it creates NOTHING.** It is the only object
carrying any wiring, and a demo install already ships the profile, sources,
channels and toolsets it names — so the fundamental lesson costs a reader no new
resources at all. Teaching the profile first inverts that: it makes an inert
object whose whole point is a Pipeline the reader has not met.

Reachable from the Introduction's existing guides section — one line each, no
index page.

**Every page has the same five parts**, in this order: what the thing IS,
"Before you start" (when it applies and when it does not), "The overall shape",
sections NAMED FOR THE TASK with their code beneath, and "What comes next".

**A guide that opens with instructions has no subject.** A first draft cut the
concept entirely and read as steps for a thing the reader had not been told
about — see `design.md` D8.

**Templates are generated from `chart/files/crds/`. Examples are rendered from
the chart.** Neither is hand-written, and CI fails when either drifts from its
source.

## Impact

- **No behaviour changes.** Documentation, plus a generator and a CI check.
- **Every tier already has a reference implementation to point at** —
  `signals/cron`, `channels/telegram`, `runtimes/claude` — so the pages point
  rather than invent.
- **Examples inherit the chart's own values.** After `scrub-identity` places the
  documented placeholder in the chart, the rendered examples carry it and the
  publication guard keeps it that way. Generating before that would bake a real
  identifier into the site.
- **Affected specs**: `docs-site` gains the guides and the generated-artifact
  rules.

## Out of scope

- **Restating reference.** A page links `concepts.md` and `contracts.md`; it
  does not reproduce them.
- **The CR reference as a site page.** It is pure reference, so it is a `docs/`
  file like `concepts.md` — no front matter, no nav entry.
- **Publishing the reference pages onto the site.** That is its own change and
  this one does not begin it.
