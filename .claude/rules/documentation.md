## After changes

**DOCUMENTATION IS PART OF THE CHANGE, NOT A FOLLOW-UP. THIS IS NOT
NEGOTIABLE.**

### EVERY OPENSPEC CHANGE ENDS WITH A DOCUMENTATION TASK

**A dedicated task, its own section, and the LAST one before the change is
finished.**

- **Not a bullet inside another task.** A docs line appended to "implement X" is
  the line that gets ticked with X and never done.
- **Not first, and not in the middle.** Written last, it documents what the
  change ACTUALLY did — which is routinely not what the proposal said it would.
- **The two sections BEFORE it are tests** — unit, then e2e — and the same gate
  reads them. See `change-tests.md`.
- **BOTH HALVES, listed separately in the task**, because they are skipped
  independently:
  1. the reference docs (`docs/concepts.md`, `docs/contracts.md`)
  2. **the ADOPTER SITE** — the landing page, Introduction, Getting started,
     Installation, the integration pages, the guides
- **An `/opsx:archive` with the task unticked is a change reported as finished
  while half of what a reader meets is stale.** Archiving is what makes it
  permanent.

**The archive is the deadline, and by then it is already too late to notice.**

**IT IS ENFORCED IN TWO PLACES, NEITHER OF WHICH IS THIS FILE.** A rule stated
in prose is followed until the evening somebody is tired.

| Where | Does |
|---|---|
| `openspec/config.yaml` — `rules.tasks` / `rules.proposal` | injected into the instructions each time a tasks or proposal file is GENERATED, so the section is written in the first place |
| `.claude/hooks/require-docs-task.sh` — a `PreToolUse` hook on Bash | **REFUSES `openspec archive`** when the last section is not documentation, when the two before it are not unit tests then e2e tests, or when any task in the three is unticked — through `.github/scripts/docs-task-guard.py`, which CI's `docs-task` job calls too |
| `.claude/hooks/require-release-docs.sh` — a `PreToolUse` hook on Bash | **REFUSES `git push` of a `chart-v<semver>` tag** unless `chart/Chart.yaml`, a `## [<semver>]` changelog entry and every version the docs print agree on the number. A CHANGE's docs and a RELEASE's docs are two events, and only the first had a gate until 13.1.0 shipped saying 13.0.1 |

- **The config half reaches the model at the moment it writes the file**, where
  a context file may already have been compacted away.
- **The hook half does not depend on the model at all.** The harness runs it.
- **ARCHIVING IS THE GATE** because it is the point of no return: the deltas
  fold into the specs and nobody revisits the change.
- **The hook FAILS OPEN** on a store-backed change, an unreadable path or a
  missing tasks file. One that blocks work it does not understand gets disabled,
  and then it enforces nothing.
- **This is not the session-title hook.** That one was deleted for competing
  with Claude Code over the terminal title, a race it could not win. This one
  validates a file and blocks one command, and nothing else writes that
  decision.



Before a change is committed, and certainly before it is ARCHIVED, every
document the change made untrue is already updated — in the same commit, not a
later one.

Archiving a change whose docs still describe the old behaviour records the work
as finished when the half a reader meets is not.

**That explicitly INCLUDES the adopter pages on the site.** It is the half most
often skipped, because a behaviour change feels done once `concepts.md` is right
— and the adopter never reads `concepts.md`.

Ask of every change: does the landing page, the Introduction, Getting started or
Installation now say something that is no longer true, or fail to mention
something an adopter must now decide?

**A page promising a step the chart just automated is as wrong as a stale field
name.**

### Where documentation goes

Keep commits scoped to this directory, and write documentation to the file that
OWNS that kind of content.

"Update README.md" is what grew it to 969 lines — three documents wearing one
filename — so the routing is explicit:

| What changed | Where it goes |
|---|---|
| CRD fields, semantics, how capabilities resolve | `docs/concepts.md` |
| Work contract, adapter contracts, HTTP endpoints | `docs/contracts.md` |
| An integration's ADOPTER-FACING content | `docs/integrations/<system>.md` — named for the SYSTEM, never the subchart |
| WHAT A BUNDLE RENDERS | nothing. The `renders` marker on that page, then `python3 .github/scripts/docs-generate.py` |
| The PARENT chart's values, install, upgrade, uninstall | `docs/installation.md` |
| A RELEASE — the chart version the install command prints, a first-party image tag a worked example shows | `docs/installation.md` and `docs/concepts.md`, and `python3 .github/scripts/docs-generate.py --check` FAILS on a number the chart does not ship. Chart 13.1.0 shipped with the site still saying `--version 13.0.1`, and no hook could see it: the two hooks check a task's shape and a commit's branch, nothing checks a typed number |
| Breaking change + upgrade steps | `docs/CHANGELOG.md`, newest first |
| Terminology | `.claude/rules/terminology.md`, `wiring.md`, `adapters.md` |
| Invariants | `.claude/rules/invariants.md` |
| Hard-won gotchas | `.claude/rules/gotchas.md` |
| A name this project STOPPED using — a removed field, a withdrawn rule, a superseded command | `.claude/rules/retired-vocabulary.md`, and a term in `.github/retired-vocabulary.json` in the same change |
| A THREAT, the posture a default install carries, what a control bounds and what is still open | `docs/security.md` — and if the change moves a trust boundary or a flow across one, re-run `python3 docs/diagrams/threat-model.py`, which CI does NOT do |
| The chart KEY that sets a control, its default and its YAML | `docs/installation.md` — never the security page, which states no value |
| What the console is FOR — its views, what each answers, the authentication decision | `docs/console-guide.md` |
| What the console IS — endpoints, RBAC grant, values reference, internals | `docs/console.md` |
| A change to the console's UI | re-run BOTH `npm run screenshots` and `npm run demo` in `platform/console/ui` — the site's screenshots and its landing recording are build output, and the change is not done until both match |
| A CRD field, an api doc comment, or a chart value a guide shows | re-run `python3 .github/scripts/docs-generate.py` — every CR template, every worked example and `docs/cr-reference.md` are build output, and CI fails on a stale one |
| A guide under `docs/guides/` | the page's prose is hand-written, its `yaml` blocks are NOT — edit the `<!-- generated: … -->` marker and re-run the generator |
| The pitch, the kind list, the demo, the install command | `README.md` |
| How a change is PROPOSED here — the openspec workflow, the commit convention, the build and test commands | `CONTRIBUTING.md` |
| How a vulnerability is REPORTED, what is supported, what is in scope | `SECURITY.md` |
| What is expected of participants, and who enforces it | `CODE_OF_CONDUCT.md` — the Contributor Covenant BY REFERENCE, never vendored |
| What an issue or a pull request must ARRIVE with | `.github/ISSUE_TEMPLATE/`, `.github/PULL_REQUEST_TEMPLATE.md` — and there is deliberately NO security template |
| The site's SHELL — Jekyll source, diagrams, what each page owes beyond its row | `docs/.claude/site.md` |
| A colour token, the theme choice, or the mark | `.claude/rules/palette-and-mark.md` — always a TWO-FILE change |
| How the site LOOKS or navigates | `docs/_layouts/`, `_includes/`, `_data/nav.yml`, `assets/` |
| What the site SAYS to an adopter | a markdown page under `docs/` |
| How a page READS — structure, tabs, components, tables, the lint | `docs/CLAUDE.md` |

**A GENERATED BLOCK IS NEVER EDITED IN PLACE.** The three generated rows above
are the same rule as the screenshots: the source is elsewhere, and a hand-edit
is reverted by the next run — silently, because the run reports success.

- **The marker is the interface.** A page declares `kind=`, `name=` and
  `fields=`, and the generator fills the block from the CRDs or from a chart
  render. Change what a page teaches by changing the marker.
- **A field the CRDs no longer have FAILS the generator**, naming the file, the
  marker and the command. That is the point of generating rather than typing.

**Both value rows are "values", so the split is stated.** The PARENT chart's
belong to `docs/installation.md`, a SUBCHART's to that integration's own page,
and neither restates the other.

**AND NEITHER CARRIES AN INVENTORY OF WHAT A BUNDLE RENDERS.** That row routes
to the GENERATOR rather than to a page, because it is a fact about the chart in
exactly the way a worked example is.

- **A page declares `<!-- generated: renders bundle=<subchart> -->`** and the
  generator fills it by rendering the chart with each component off and on and
  diffing the objects. `--check` then fails on a stale table.
- **IT WAS TYPED BY HAND AND IT WENT WRONG SILENTLY.** `af5bb49` gave each route
  its own `ServiceAccount` and no component table said so. `84a2654` stopped
  rendering them, and all three tables became correct again — repaired by the
  chart changing back rather than by anyone noticing.
- **`docs/claude.md` keeps its own hand-written table**, being a reference page
  rather than an integration page. A runtime is what EXECUTES an agent, not a
  seam a Pipeline wires.

**`installation.md` carries the values an operator must DECIDE**, grouped by the
decision they serve. `helm show values` is the exhaustive list, and a
hand-copied inventory rots.

**The last three rows are one rule read three ways: the theme holds no prose,
the pages hold no theme, and neither holds the rules.**

- A layout or include that starts explaining a CRD is in the wrong file.
- So is a markdown page that opens with a `<div>` or an inline style.
- Adding a page to the site is a page plus one line in `_data/nav.yml`, never
  navigation markup written a second time.

### README.md IS THE LANDING PAGE'S COUNTERPART ON THE FORGE. BUDGET: 215 LINES

`wc -l README.md`.

**IT CARRIES WHAT THE LANDING PAGE CARRIES, MORE CONCISELY, AND SAYS THE SITE IS
THE SOURCE.** The two are one story for two audiences — a reader who arrived at
the site, and a stranger who landed on the forge and may never leave it. The
forge audience is the larger one, so sending it away to learn what the project
IS is a gap, not a boundary.

The sections track the landing page's, and a section added there is considered
here:

| Landing page | README |
|---|---|
| the claims strip | the same three, one line |
| the `.ao-presentation` build-up | the flow diagram, then **How it works** in five numbered steps |
| the "What you write" tab | **What you write** — the same `Pipeline`, comment-annotated |
| the console recording | a link to it, plus what the console is |
| When it runs / Why it is built this way / Pluggable at three seams | the same three lists |
| Why agent-ops? and the chip set | the same table, and "Works with" as one line |
| Where to start / Understand the model / Run it | **Where to go next**, site first |

**RESTATING IS THE FAILURE, COVERING IS NOT**, and the difference is where the
DETAIL lives:

- **The walkthrough, the installation decisions, the console tour and the guides
  are the SITE's**, in full. The README names them and links.
- **A README that reproduces a site PAGE is a second source of truth**, and the
  drift is invisible until a reader follows the wrong one. The old expanded
  start was that, and was cut.
- **Naming the same subject in a line is COVERING.** An early draft of this
  change read the rule as forbidding that, and produced a thin index that told a
  stranger nothing they could evaluate the project on. That draft is what this
  section exists to prevent.
- **The KIND TABLE stays.** Eleven kinds IS the product, and a reader who cannot
  see the shape of the model without following a link has not been told what
  this is.

**THE BUDGET WENT 150 → 215, AND THE OLD NUMBER WAS FOR A DIFFERENT DOCUMENT.**
150 bounded a README that was three documents wearing one filename — reference
tables, contracts, upgrade guides.

- **THE SECTION LIST IS THE BOUND. The number FOLLOWS it**, and is what those
  sections currently cost. A section added to the landing page moves it; a
  paragraph that wandered in does not.
- **The diagram costs FOUR lines** — a `<picture>` naming two files — since it
  stopped being inline mermaid. See below.
- **The hard rule is unchanged: NO REFERENCE MATERIAL.** If it is over budget,
  the first question is which section grew, and the answer is usually that
  something belongs in a `docs/` page.

**THE TWO SURFACES USE DIFFERENT MEDIA, WHICH IS WHY NEITHER IS A COPY:**

| Surface | Shows the story as |
|---|---|
| the site's landing page | the `.ao-presentation` tab set and the console recordings |
| `README.md` | `assets/img/readme-flow-{light,dark}.svg` in a `<picture>` |

**GitHub renders neither tabs nor autoplaying video**, so the README needs its
own picture. **THERE ARE NOW THREE DIAGRAM SOURCES AND THEY ARE NOT
INTERCHANGEABLE:**

| Source | Writes | For |
|---|---|---|
| `diagrams/agent-ops.drawio` + `export.py` | `agent-ops-{light,dark}.svg` | the PAGE-SCALE picture, 1778×1349 |
| `diagrams/readme-flow.py` | `readme-flow-{light,dark}.svg` | the README column, 1000×306 |
| `diagrams/message-flow.mmd` | nothing — read as source | `concepts.md` links it |

- **MERMAID WAS TRIED FOR THE README AND REMOVED.** It ignores `direction` on a
  subgraph whose edges cross the boundary, so four source nodes stacked into a
  full page of scrolling on GitHub, and it clipped an edge label to its first
  letter. It also offers no real icons and no custom shapes. **It rendered
  correctly in a local preview and wrongly on GitHub**, which is the reason to
  check a README diagram on the forge and not only in a harness.
- **THE PAGE-SCALE SVG IS LINKED, NOT EMBEDDED.** A forge column shrinks
  1778×1349 past legibility, so it is the caption's click-through. Do not delete
  it as unused, and do not hand-edit it — `export.py` owns it.
- **`readme-flow.py` IS THE README'S, AND IT WRITES BOTH THEMES FROM ONE RUN.**
  Never edit either SVG by hand; the halves would drift, and only one of them is
  ever on screen for a given reader. Its palette is COPIED from
  `assets/css/agentops.css`, which copies from the console theme — the same
  one-directional copy `palette-and-mark.md` documents, so a token change is a
  THIRD file now.
- **THE DIAGRAM IS THE VISUAL, THE PROSE IS THE CONTENT.** Whatever the picture
  also contains is NOT dropped from the text for that reason. A reader skims
  HEADINGS, and cutting prose because a picture had it was tried here and read
  as an empty page.

- **The install command must resolve a PUBLISHED artifact.** One naming a path
  inside the repository is not a start, it is a step that silently assumes a
  clone the previous line never mentioned.
- **What moves out is LINKED, never dropped.** Every reader who followed
  content there before reaches it from the index in ONE hop.
- **Reference material and migration guides do not belong in it.**
- **If it is over budget, something is in the wrong file.**
