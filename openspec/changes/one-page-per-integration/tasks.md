# Tasks

**This change is mostly documentation, and section 7 is still not optional.**
Sections 1–6 are the deliverable — the generator, the pages, the deletions.
Section 7 is the half about the change itself: the routing rule that tells the
NEXT contributor where an integration's content goes, and the adopter pages this
change made untrue. They are skipped independently, which is why they are listed
separately.

**The Kubernetes page is written first and completely, and is the reference for
the other three.** It is the only integration with all of signals, tools,
suppression rules and self-exclusion, so a shape that survives it survives the
rest. Filling the other three against it is the point — re-deriving a page shape
per vendor is what produces four pages that read like four products.

**The deep prose is MOVED, not rewritten.** The event-suppression explanation,
the workload-grouping rule, the self-exclusion mechanisms, the Home Assistant
privilege split and the credential path are the best writing in the four pages.
Rewriting them from memory loses the reasoning that makes them worth publishing.

## 1. The generator learns what a bundle renders

- [ ] 1.1 Let the block dispatcher return arbitrary markdown rather than only a fenced `yaml` block. Verify the existing `template` and `example` markers still produce byte-identical output by running `python3 .github/scripts/docs-generate.py --check` against the unchanged tree.
- [ ] 1.2 Declare each bundle's components — the bundle's enable key, and per component the `sets` that turn it on and the `requires` it cannot render without. Verify every declared value is a declared placeholder or a boolean, under the same guard `PRESETS` already applies.
- [ ] 1.3 Implement attribution as a set difference against a prerequisite-aware baseline, per `design.md`. Verify `k8s-bundle` reproduces the five rows the design records, `mcpServers` carrying its own five objects and not `mcp`'s two.
- [ ] 1.4 Add the `renders` marker, one per bundle, emitting a row per component in declaration order. Verify the emitted table names the component, the value that enables it, and the objects it renders.
- [ ] 1.5 Add a `prometheus-bundle` preset — it has none, so the generator has never rendered it. Verify it renders without an invented identifier reaching the output.
- [ ] 1.6 Verify the guard actually guards: hand-edit a generated row, run `--check`, and confirm it fails naming the file. A generated block nothing checks is a hand-written block with extra steps.
- [ ] 1.7 Verify a missing `requires` fails loudly — remove one, run the generator, and confirm it stops with the chart's own error rather than emitting an empty row.

## 2. The Kubernetes page, which is the shape

- [ ] 2.1 Create `docs/integrations/kubernetes.md` with its own `permalink`, front matter and nav-ready title. Verify it builds and is reachable at `/integrations/kubernetes/`.
- [ ] 2.2 Write the four sections in the Pipeline's order — what starts work, what it may reach, turn it on, what it renders — omitting "where it answers", which Kubernetes does not carry. Verify no section is present-but-empty.
- [ ] 2.3 Move the event-suppression prose, the workload-grouping rule and the self-exclusion mechanisms from `k8s-bundle.md`, edited only where they address a reader who has already installed. Verify the `for`-versus-`group_wait` correction and the verification ladder survive the move.
- [ ] 2.4 Place the `renders` marker and regenerate. Verify the block lists the five components and that `--check` is clean.
- [ ] 2.5 Verify the page names no exhaustive values, sending a reader wanting them to the chart's own values instead.

## 3. The remaining three pages, filled against it

- [ ] 3.1 `docs/integrations/prometheus.md` — the Alertmanager lane and metrics tooling, moved from `prometheus-bundle.md`. Verify it uses the same four sections in the same order as the Kubernetes page.
- [ ] 3.2 `docs/integrations/home-assistant.md` — the log lane, the house's tools, and the two profiles split by privilege. Verify the privilege split and the credential-path warning both survive, since they are what the page is for.
- [ ] 3.3 `docs/integrations/telegram.md` — the ingest trio and the chat surface. Verify it is the ONLY page carrying a "where it answers" section, and that it says the bundle ships no wiring and why.
- [ ] 3.4 Place a `renders` marker on each and regenerate. Verify `--check` is clean across all four pages.

## 4. The index, and every way in

- [ ] 4.1 Write `docs/integrations/index.md` with the seam table — every integration against what starts work, what may be reached, where answers go. Verify it names the console, cron and bring-your-own, none of which is a page.
- [ ] 4.2 Give the `works with` chips on `docs/index.md` their destinations. Verify every chip resolves: the four systems to their pages, the console to `/console/`, any MCP server to `/guides/toolsets/`, cron and your own to the index.
- [ ] 4.3 Add the integrations group to `docs/_data/nav.yml`. Verify the sidebar marks the current entry on each of the five pages.
- [ ] 4.4 Repoint `docs/installation.md`'s "Enable a bundle" table and `docs/index.md`'s `Run it` list from GitHub URLs to the new internal pages. Verify each link resolves in the built site.

## 5. Retire the bundle pages

- [ ] 5.1 Delete `docs/k8s-bundle.md`, `docs/prometheus-bundle.md`, `docs/ha-bundle.md` and `docs/telegram-bundle.md`. Verify a repository-wide grep for those filenames returns nothing outside archived change history.
- [ ] 5.2 Update `README.md`'s six links. Verify it stays inside its 150-line budget with `wc -l README.md`.

## 6. Verification across the site

- [ ] 6.1 Build the site and assert per page, in both colour schemes, that `document.documentElement.scrollWidth` does not exceed `clientWidth`. Verify the five new pages pass in both.
- [ ] 6.2 Assert no generated table forces the page body to scroll sideways at 620px — an object list is the widest thing these pages carry.
- [ ] 6.3 Run the prose lint from `docs/CLAUDE.md` over every new and changed page. Verify it is silent.
- [ ] 6.4 Verify every internal link in the built site resolves, since this change deletes four pages and repoints roughly a dozen references.
- [ ] 6.5 Screenshot one integration page in both themes and read the captures. Verify the generated table renders as a table and not as an escaped block, and write the captures to the scratchpad rather than the repository.

## 7. Documentation

### Reference docs

- [ ] 7.1 Update `docs/CLAUDE.md`: the pages table gains the integrations rows, and the components table gains the `renders` marker. Verify no row still routes anything to a `<bundle>.md` page.
- [ ] 7.2 Update `.claude/rules/documentation.md`'s routing table — "a subchart's components or values" no longer goes to `docs/<bundle>.md`, and a component inventory goes to the generator rather than to a page. Verify both rows match what the code now does.
- [ ] 7.3 Re-run `python3 .github/scripts/docs-generate.py` and commit the result. Verify `--check` passes, which is what CI will run.
- [ ] 7.4 Confirm no `CHANGELOG.md` entry is owed — this ships no chart version and no image. Verify by checking the change touches nothing under `chart/` or `platform/`.

### The adopter site

- [ ] 7.5 Read `docs/introduction.md`, `docs/getting-started.md` and `docs/console-guide.md` for sentences naming a bundle page or promising values it held. Verify each is either untouched or corrected, and say which.
- [ ] 7.6 Read `docs/guides/*.md` on the same terms, and verify `guides/toolsets.md` earns being the MCP chip's destination by rendering it and reading what a reader arriving there meets.
- [ ] 7.7 Read `docs/concepts.md` and `docs/contracts.md` for links into the deleted pages. Verify each points somewhere that exists.
