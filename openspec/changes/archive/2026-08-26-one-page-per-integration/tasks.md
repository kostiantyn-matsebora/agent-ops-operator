# Tasks

**This change is mostly documentation, and section 7 is still not optional.**
Sections 1–6 are the deliverable — the generator, the pages, the move.
Section 7 is the half about the change itself: the routing rule that tells the
NEXT contributor where an integration's content goes, and the adopter pages this
change made untrue. They are skipped independently, which is why they are listed
separately.

**The Kubernetes page is written first and completely, and is the reference for
the other three.** It is the only integration with all of signals, tools,
suppression rules and self-exclusion, so a shape that survives it survives the
rest. Filling the other three against it is the point — re-deriving a page shape
per vendor is what produces four pages that read like four products.

**The four pages already carry their system names.** `413bbbc` renamed them from
`<bundle>.md` at the `docs/` root, so this change MOVES `docs/kubernetes.md`,
`docs/prometheus.md`, `docs/home-assistant.md` and `docs/telegram.md` under
`docs/integrations/` and rewrites them there. None carries front matter today, so
no published URL is at stake and every inbound link is already a raw GitHub URL.

**The deep prose is MOVED, not rewritten.** The event-suppression explanation,
the workload-grouping rule, the self-exclusion mechanisms, the Home Assistant
privilege split and the credential path are the best writing in the four pages.
Rewriting them from memory loses the reasoning that makes them worth publishing.

## 1. The generator learns what a bundle renders

- [x] 1.1 Let the block dispatcher return arbitrary markdown rather than only a fenced `yaml` block. Verify the existing `template` and `example` markers still produce byte-identical output by running `python3 .github/scripts/docs-generate.py --check` against the unchanged tree.
- [x] 1.2 Declare each bundle's components — the bundle's enable key, and per component the `sets` that turn it on and the `requires` it cannot render without. Verify every declared value is a declared placeholder or a boolean, under the same guard `PRESETS` already applies.
- [x] 1.3 Implement attribution as a set difference against a prerequisite-aware baseline, per `design.md`. Verify `kubernetes` reproduces the five rows the design records — `eventsAdapter` carrying its own ServiceAccount, `mcpServers` its own five objects and not `mcp`'s two, and `pipelines` the `Pipeline` alone — and that `home-assistant`'s wiring row is its two Pipelines and nothing else.
- [x] 1.4 Add the `renders` marker, one per bundle, emitting a row per component in declaration order. Verify the emitted table names the component, the value that enables it, and the objects it renders.
- [x] 1.5 Add a `prometheus` preset — `PRESETS` declares `tier1` through `tier4` and none enables that bundle, so the generator has never rendered it. Verify it renders without an invented identifier reaching the output.
- [x] 1.6 Verify the guard actually guards: hand-edit a generated row, run `--check`, and confirm it fails naming the file. A generated block nothing checks is a hand-written block with extra steps.
- [x] 1.7 Verify a missing `requires` fails loudly — remove one, run the generator, and confirm it stops with the chart's own error rather than emitting an empty row.

## 2. The Kubernetes page, which is the shape

- [x] 2.1 Create `docs/integrations/kubernetes.md` with its own `permalink`, front matter and nav-ready title. Verify it builds and is reachable at `/integrations/kubernetes/`.
- [x] 2.2 Write the four sections in the Pipeline's order — what starts work, what it may reach, turn it on, what it renders — omitting "where it answers", which Kubernetes does not carry. Verify no section is present-but-empty.
- [x] 2.3 Move the event-suppression prose, the workload-grouping rule and the self-exclusion mechanisms from `docs/kubernetes.md`, edited only where they address a reader who has already installed. Verify the `for`-versus-`group_wait` correction and the verification ladder survive the move.
- [x] 2.4 Place the `renders` marker and regenerate. Verify the block lists the five components and that `--check` is clean.
- [x] 2.5 Verify the page names no exhaustive values, sending a reader wanting them to the chart's own values instead.

## 3. The remaining three pages, filled against the APPROVED Kubernetes page

**The shape was signed off on Kubernetes first, and these three follow it
exactly**: what you get · turn it on · the values you set · tune what reaches
you · adopt it · what the bundle renders · going deeper. Nothing generic is
restated, and every claim about a route is checked against a real
`helm template` of that bundle rather than against the page it replaces.

- [x] 3.1 `docs/integrations/prometheus.md` — the Alertmanager lane and metrics tooling, moved from `docs/prometheus.md`. Verify it uses the same four sections in the same order as the Kubernetes page.
- [x] 3.2 `docs/integrations/home-assistant.md` — the log lane, the house's tools, and the two profiles split by privilege. Verify the privilege split and the credential-path warning both survive, since they are what the page is for.
- [x] 3.3 `docs/integrations/telegram.md` — the ingest trio and the chat surface. Verify it is the ONLY page carrying a "where it answers" section, and that it says the bundle ships no wiring and why.
- [x] 3.4 Place a `renders` marker on each and regenerate. Verify `--check` is clean across all four pages.

## 4. The index, and every way in

- [x] 4.1 NO index page: the sidebar group IS the index, so a page listing what the sidebar lists is a second navigation written by hand. Verify nothing links to `/integrations/` as a destination.
- [x] 4.2 Leave `docs/claude.md` the reference page it is — a runtime is what EXECUTES an agent, not a seam a Pipeline wires. Verify no page or nav entry presents it as a fifth integration.
- [x] 4.3 Give the `works with` chips on `docs/index.md` their destinations. Verify every chip resolves: the four systems to their pages, the console to `/console/`, any MCP server to `/guides/toolsets/`, cron and your own to `/guides/signal-adapter/`.
- [x] 4.4 Add the integrations group to `docs/_data/nav.yml`. Verify the sidebar marks the current entry on each of the four pages.
- [x] 4.5 Repoint `docs/installation.md`'s "Enable a bundle" table and `docs/index.md`'s `Run it` list from GitHub URLs to the new internal pages. Verify each link resolves in the built site.

## 5. Retire the root pages

- [x] 5.1 `git mv` `docs/kubernetes.md`, `docs/prometheus.md`, `docs/home-assistant.md` and `docs/telegram.md` under `docs/integrations/`, leaving no copy at the `docs/` root. Verify a repository-wide grep for both the current root paths and the pre-`413bbbc` `<bundle>.md` names returns nothing outside archived change history and the site's `_site/` build output.
- [x] 5.2 Repoint the four links in `README.md`'s bundles row, leaving `docs/claude.md` as it is. Verify it stays inside its **215**-line budget with `wc -l README.md`, which it currently spends 204 of.

## 6. Verification across the site

- [x] 6.1 Build the site and assert per page, in both colour schemes, that `document.documentElement.scrollWidth` does not exceed `clientWidth`. Verify the four new pages pass in both.
- [x] 6.2 Assert NO ELEMENT exceeds its own content column, at 1600/1280/900/620px — not just that the page body does not scroll, which is the assertion that passed while the object list ran under the on-this-page rail. An object list is the widest thing these pages carry.
- [x] 6.3 Run the prose lint from `docs/CLAUDE.md` over every new and changed page. Verify it is silent.
- [x] 6.4 Crawl the built site and verify every internal link resolves AND every `#fragment` names a real id on its target page. A page-level crawl passes on a link whose anchor does not exist — which is how two stale anchors into the rewritten Kubernetes page reached review.
- [x] 6.5 Screenshot an integration page in both themes and READ the captures. Verify the generated block renders as a code listing with every object name fully visible — not as escaped text, not clipped, not behind a horizontal scrollbar. Write the captures to the scratchpad, never the repository.

## 7. Documentation

### Reference docs

- [x] 7.1 Update `docs/CLAUDE.md`: the pages table gains the integrations rows, and the components table gains the `renders` marker. Verify no row still routes anything to a `<bundle>.md` page.
- [x] 7.2 Update `.claude/rules/documentation.md`'s routing table — "a subchart's components or values" no longer goes to `docs/<bundle>.md`, which has named no file since `413bbbc`, and a component inventory goes to the generator rather than to a page. Correct the two other sentences naming a bundle page: the docs-task line's "a bundle page" and the values split's "a SUBCHART's to that bundle's own page". Verify every row matches what the tree now holds.
- [x] 7.3 Re-run `python3 .github/scripts/docs-generate.py` and commit the result. Verify `--check` passes, which is what CI will run.
- [x] 7.4 Confirm no `CHANGELOG.md` entry is owed — this ships no chart version and no image. Verify by checking the change touches nothing under `chart/` or `platform/`.

### The adopter site

- [x] 7.5 Read `docs/introduction.md`, `docs/getting-started.md` and `docs/console-guide.md` for sentences naming a bundle page or promising values it held. Verify each is either untouched or corrected, and say which.
- [x] 7.6 Read `docs/guides/*.md` on the same terms, and verify `guides/toolsets.md` earns being the MCP chip's destination by rendering it and reading what a reader arriving there meets.
- [x] 7.7 Read `docs/concepts.md` and `docs/contracts.md` for links into the deleted pages. Verify each points somewhere that exists.
