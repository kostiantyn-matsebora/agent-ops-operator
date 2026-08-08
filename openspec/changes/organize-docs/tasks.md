## 1. Extract the changelog

- [ ] 1.1 Create `CHANGELOG.md` with a one-line purpose statement ("chart-version
      migration guides; see README.md for the overview") and an `## Unreleased`
      heading
- [ ] 1.2 Move all eight `## Migrating to chart …` sections (README lines
      367–503) verbatim into `CHANGELOG.md`, re-ordered newest-first: 1.12, 1.8,
      1.7, 1.6, 1.4, 1.3, 1.1, 1.0 — keep the `— BREAKING` markers, retitle as
      `## chart <version> — <title>`
- [ ] 1.3 Delete those sections from `README.md`; fix the cross-references
      *inside* the moved guides (the 1.0 guide's "see below" pointing at 1.1,
      the 1.6 guide's "see the VM bundle section" → `docs/vm-bundle.md`)

## 2. Build the reference pages

- [ ] 2.1 Create `docs/concepts.md`: the full CRD table cells as `### <Kind>`
      sections (all nine kinds), followed by the "Tool access is wiring" section
      (README 61–107) with both resolution tables intact
- [ ] 2.2 Create `docs/contracts.md`: the work contract (297–310), channel
      adapter contract (109–176), signal adapter contract (178–205), and the
      HTTP API table (356–365) — verbatim, re-leveled one heading deeper under a
      page title
- [ ] 2.3 Create `docs/vm-bundle.md`: the VictoriaMetrics bundle section
      (207–295) verbatim, including both YAML examples and the registration
      sharp edges
- [ ] 2.4 Rewrite relative links inside all three pages for the `docs/` depth
      (`channel-telegram/` → `../channel-telegram/`, `runtime-claude/` →
      `../runtime-claude/`, `signal-cron/` → `../signal-cron/`, `CLAUDE.md` →
      `../CLAUDE.md`) and repoint former in-README anchors at their new pages

## 3. Trim the README

- [ ] 3.1 Delete the moved sections (61–107, 109–310, 356–365) from `README.md`
- [ ] 3.2 Collapse the CRD table to one line per kind (kind + a single sentence),
      linking the kind name to its `docs/concepts.md#<kind>` section
- [ ] 3.3 Add a `## Documentation` section indexing `docs/concepts.md`,
      `docs/contracts.md`, `docs/vm-bundle.md`, `CHANGELOG.md`, and `CLAUDE.md`,
      one line each saying what it holds
- [ ] 3.4 Repoint the surviving in-README anchors: `#the-work-contract`,
      `#victoriametrics-bundle-subchart`, `#tool-access-is-wiring`,
      `#the-signal-adapter-contract` (in the CRD table, the Install section, and
      the demo section)
- [ ] 3.5 Confirm `README.md` ≤ 150 lines (`wc -l README.md`) and that the
      remaining sections are exactly: title/pitch/diagram, Concepts, Behaviors
      that matter, Try it in five minutes, Install, Documentation, Development,
      Status

## 4. Record the convention

- [ ] 4.1 Replace the `CLAUDE.md` "After changes" line with the routing rule:
      concepts → `docs/concepts.md`, contracts → `docs/contracts.md`, VM bundle
      → `docs/vm-bundle.md`, upgrade steps → `CHANGELOG.md`, README only for
      pitch / kind list / demo / install — plus the ≤150-line README budget
- [ ] 4.2 Add `docs/` and `CHANGELOG.md` to the `CLAUDE.md` Map section
- [ ] 4.3 Update the `CLAUDE.md:3` "see README.md for the product view" pointer
      to name `docs/concepts.md` for the detail

## 5. Sweep and verify

- [ ] 5.1 Repoint remaining in-repo "see README" pointers at the page now
      holding the text (`chart/values.yaml:137`; re-grep for `README` across
      `*.md`, `*.yaml`, `*.go` excluding `openspec/changes/`)
- [ ] 5.2 Line-account the move: former README was 514 lines — verify new
      `README.md` + `CHANGELOG.md` + `docs/*.md` reconcile with no section
      dropped and none duplicated
- [ ] 5.3 Run a one-off link check (not committed) over `README.md`,
      `CLAUDE.md`, `CHANGELOG.md`, `docs/*.md`: every relative link resolves to
      an existing file and every `#anchor` matches a heading in its target
- [ ] 5.4 Render-check the moved tables and fenced YAML blocks (GitHub preview
      or a markdown renderer) — table pipes and code fences survive the move
- [ ] 5.5 Confirm no non-doc file changed: `git status` shows only `README.md`,
      `CLAUDE.md`, `CHANGELOG.md`, `docs/`, `chart/values.yaml` (comment only),
      and the openspec change dir
