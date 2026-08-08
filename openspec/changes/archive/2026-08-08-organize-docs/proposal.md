# Organize docs: one-page README, changelog document, reference pages

## Why

README.md has grown to 514 lines and is now three documents in one: a product
overview, a full reference manual (adapter contracts, work contract, VM bundle,
tool-access resolution tables), and 137 lines of chart-version migration guides
that only matter to someone upgrading. A newcomer cannot answer "what is this
and how do I try it?" without scrolling past eight `## Migrating to chart …`
sections, and every feature change adds more, so the file only gets worse.

## What Changes

- **README.md becomes a one-page read** (~120 lines): pitch + diagram, a
  one-line-per-kind CRD table, the behaviors that matter, the five-minute demo,
  install, development, status, and a Documentation index linking everything
  else. No section that a first-time reader can skip stays in it.
- **New `CHANGELOG.md`** holds every `## Migrating to chart …` section, moved
  verbatim and re-ordered newest-first (1.12, 1.8, 1.7, 1.6, 1.4, 1.3, 1.1,
  1.0) — the README order is currently arbitrary. It becomes the single place
  future breaking changes are recorded.
- **New `docs/` reference pages**, content moved verbatim (this change adds no
  new technical claims and edits no behavior):
  - `docs/concepts.md` — the full CRD reference (today's dense table cells) and
    the "tool access is wiring" section with both resolution tables.
  - `docs/contracts.md` — the work contract, the channel adapter contract, the
    signal adapter contract, and the HTTP API table.
  - `docs/vm-bundle.md` — the VictoriaMetrics subchart section.
- **Cross-references repointed**: README in-page anchors (`#the-work-contract`,
  `#victoriametrics-bundle-subchart`, `#tool-access-is-wiring`,
  `#the-signal-adapter-contract`) become `docs/*.md` links; `chart/values.yaml`
  and any other "see README" pointer land on the page that now holds the text.
- **CLAUDE.md documents the convention** so it survives: which file receives
  which kind of update, and that migration notes go to `CHANGELOG.md` — the
  current "Update README.md when concepts/behavior change" instruction is what
  grew the file to 514 lines.

Not in scope: rewriting any technical content, changing chart/CRD/code
behavior, or a docs site. Nothing is deleted — every line lands somewhere.

## Capabilities

### New Capabilities
- `documentation-structure`: which repo document holds which kind of content —
  README as a bounded one-page overview, `CHANGELOG.md` as the migration/version
  record, `docs/` as reference — plus the rule that keeps them from re-merging.

### Modified Capabilities
<!-- none: no existing spec in openspec/specs/ references README, CHANGELOG, or docs/ -->

## Impact

- `README.md` (514 → ~120 lines), new `CHANGELOG.md`, new `docs/concepts.md`,
  `docs/contracts.md`, `docs/vm-bundle.md`.
- `CLAUDE.md` — "After changes" section gains the routing rule.
- `chart/values.yaml:137` and any other in-repo "see README" pointer.
- In-flight changes whose tasks say "update README.md"
  (`visualize-agent-ops`, `k8s-bundle`, `ha-bundle`, `add-web-chat-channel`)
  are not rewritten here; the CLAUDE.md rule tells whoever applies them where
  the text now belongs.
- No code, CRD, chart-template, or API change; no test change. Risk is limited
  to broken links, which the verification step checks mechanically.
