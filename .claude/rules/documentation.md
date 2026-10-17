## After changes

**DOCUMENTATION IS PART OF THE CHANGE, NOT A FOLLOW-UP.**

### EVERY OPENSPEC CHANGE ENDS WITH A DOCUMENTATION TASK

**A dedicated task, its own section, LAST** — not a bullet inside another
task. The two sections BEFORE it are tests (unit, then system —
`change-tests.md`). BOTH halves, listed separately: reference docs AND the
adopter site (landing page, Getting started, Installation, guides).

**ENFORCED BY TWO MECHANISMS, NOT THIS FILE:**

| Where | Does |
|---|---|
| `openspec/config.yaml` | injects the shape into a generated tasks/proposal file |
| `.claude/hooks/require-docs-task.sh` | REFUSES `openspec archive` on a missing/unticked section, fails open on an unreadable path |
| `.claude/hooks/require-release-docs.sh` | REFUSES a chart tag push unless `Chart.yaml`, changelog and docs agree on the version |

Archiving is the gate — the point of no return, before which every document
a change made untrue must already be updated, adopter pages included.

### Where documentation goes

Write to the file that OWNS that content:

| What changed | Where it goes |
|---|---|
| CRD fields, capability resolution | `docs/concepts.md` |
| Work/adapter contracts, HTTP endpoints | `docs/contracts.md` |
| An integration's adopter content | `docs/integrations/<system>.md`, dense prose, no exhaustive values, no renders table |
| What a bundle renders, for a reference page (`cr-reference.md`, `guides/`, `runtimes/<vendor>.md`) | the `renders` marker + `docs-generate.py` |
| Install/upgrade/uninstall, storage access mode | `docs/installation.md` |
| Other parent chart values (reversible) | `docs/configuration.md` |
| A release's chart/image version | `installation.md` + `concepts.md` |
| Breaking change + upgrade steps | `docs/CHANGELOG.md`, newest first |
| Terminology | `terminology.md`, `wiring.md`, `adapters.md` |
| Invariants | `invariants.md` |
| Hard-won gotchas | `gotchas.md` |
| A retired name | `retired-vocabulary.md` + `.github/retired-vocabulary.json` |
| A threat, control, open risk | `docs/security.md` — a moved trust boundary re-runs `threat-model.py` |
| The chart key that sets a control | `docs/configuration.md`, never security.md |
| What the console is FOR / IS | `console-guide.md` / `console.md` |
| A console UI change | re-run `screenshots` AND `demo` npm scripts |
| A CRD field / chart value a guide shows | re-run `docs-generate.py` |
| A guide under `docs/guides/` | hand-written prose, generated `yaml` blocks |
| Pitch, kind list, demo, install command | `README.md` |
| Propose workflow, commit convention, build/test | `CONTRIBUTING.md` |
| Vulnerability reporting | `SECURITY.md` |
| Participant conduct | `CODE_OF_CONDUCT.md`, by reference |
| Issue/PR arrival requirements | `.github/ISSUE_TEMPLATE/`, PR template |
| The site's shell, diagrams | `docs/.claude/site.md` |
| A colour token, theme, the mark | `palette-and-mark.md`, two files |
| Site look/navigation | `_layouts/`, `_includes/`, `nav.yml`, `assets/` |
| What the site says to an adopter | a markdown page under `docs/` |
| How a page reads | `docs/CLAUDE.md` |

**A GENERATED BLOCK IS NEVER HAND-EDITED** — the next run reverts it
silently. A marker declares `kind=`/`name=`/`fields=`, failing loudly on a
field the CRDs no longer have. `installation.md` is what install CANNOT
undo, `configuration.md` is everything that CAN.

### README.md mirrors the landing page, more concisely. Budget: 215 lines

The site is the source — sections track the landing page's. RESTATING is
the failure, COVERING is not: walkthrough/install/guides stay the SITE's in
full, README only names and links them. NO REFERENCE MATERIAL.

Two diagram sources, not interchangeable: `export.py` writes the
page-scale SVG (linked, not embedded), `readme-flow.py` writes the
README's own. Never hand-edit either SVG.
