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

**Seven pages, four tiers, in learning order:**

| Tier | Page | Teaches |
|---|---|---|
| 1 | Add your own agent | `AgentProfile` with an inline prompt, plus the `Pipeline` that reaches it |
| 1 | ↳ …from your own repository | `repository`, `.claude/agents/<name>.md`, the deploy key |
| 2 | Give it capabilities | `MCPToolset`, `MCPConfig`, how `toolsMode` composes against the definition |
| 3 | Write a signal adapter | `/signal/inbound`, the `SignalAdapter` CR, self-exclusion |
| 3 | Write a channel adapter | `/channel/*`, the ops queue, typed messages |
| 4 | Write an agent runtime | the work contract, and what a runtime is trusted to enforce |
| — | `docs/cr-reference.md` | every field of every kind, generated |

Reachable from the Introduction's existing guides section — one line each, no
index page.

**Every page has the same four parts:** what you are doing, a minimal CR to fill
in, a link to the full generated reference, and the in-repo implementation to
copy from.

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
