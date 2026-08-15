## Why

The site can get an adopter to a running demo and no further. Getting started
installs `global.demo.enabled=true`, says plainly that it is **a demo, not a
deployment**, and promises that "a page for real installs comes later". This is
that page.

Nothing owns the parent chart's values today. `docs/<bundle>.md` owns each
subchart's, `docs/console.md` owns the console's, `docs/concepts.md` owns the
substrate stanza — and the remaining decisions an operator has to make before a
real install (capacity, storage, RBAC posture, adapter auth, retention,
housekeeping, CRD lifecycle) are spread across comments in `values.yaml` and a
README section that is capped at 150 lines and deliberately holds no reference
material.

So an adopter who finishes the demo and asks "now how do I run this properly?"
has no page to read.

## What Changes

- **A new site page, `docs/installation.md`**, published at `/installation/`:
  - **before you install** — the decisions that are painful to change later:
    storage and context continuity, the RBAC posture the agent inherits, and
    whether CRDs are managed by the release;
  - **install** — namespace, credential, `helm install`, and how to verify the
    release is healthy;
  - **enable a bundle** — what a bundle is, the three that ship, the flag each
    takes, and a link to the page that owns its values;
  - **configure** — the parent chart's values, **grouped by the decision they
    serve** rather than dumped as a generated table, each group naming the few
    keys that matter and linking onward for the rest;
  - **wire it** — that a fresh install answers nothing until a Pipeline claims a
    source, with the smallest real route as the example;
  - **upgrade and uninstall** — where migration steps live, and what survives an
    uninstall.
- **The page owns the parent chart's values as reference.** A new row in the
  documentation routing: parent-chart values, install, upgrade and uninstall go
  here. Bundle values stay with their bundle page, console values with
  `console.md`, the substrate stanza with `concepts.md` — this page links to all
  three rather than restating them.
- **One navigation entry**, under *Start here*, after *Getting started*.
- **Getting started's forward promise becomes a link**, and its Next card points
  at this page.
- **README's Documentation index gains the row.**

Explicitly NOT in this change:

- **No bundle values.** Each bundle has a page that owns them. This page names
  the enable flag and links.
- **Not a generated `values.yaml` dump.** An exhaustive table is what
  `helm show values` is for, and a hand-copied one rots. The page carries the
  values an adopter must DECIDE.
- **No chart change.** If a step needs a values change to be true, the page is
  wrong, not the chart.
- **No new theme component.** Numbered steps, tables and callouts already exist.

## Capabilities

### New Capabilities

None. The published site is one capability and this page belongs to it.

### Modified Capabilities

- `docs-site`: the deliverables grow by the Installation page, and the site
  gains a requirement stating what that page must get an operator to and what it
  must not absorb.
- `documentation-structure`: the routing gains an owner for parent-chart values,
  install, upgrade and uninstall — content that today has no home.

## Impact

- `docs/installation.md` (new), `docs/_data/nav.yml` (one entry),
  `docs/getting-started.md` (the promise becomes a link, Next card retargeted),
  `docs/index.md` (paths onward).
- `README.md` — one row in the Documentation index.
- `CLAUDE.md` — the routing table row, and the `docs/` map line.
- No Go code, no chart, no CRDs.
