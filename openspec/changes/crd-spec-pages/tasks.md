# Tasks — crd-spec-pages

The approved mockup is `mockups/index.html` in this change. Implementation
tasks PORT from it — tab 1 the landing opening, tab 2 the diagram, tab 3 the
spec-page shape, tab 4 the nav — rather than re-deriving the design.

## 1. Generator: per-CRD blocks replace cr-reference — deployment-engineer

- [x] 1.1 Add `crd-example` and `crd-fields` block kinds to
      `.github/scripts/docs-generate.py` (example instance with one comment
      per top-level field, field table with name/type/required/meaning),
      verified by running the script against one hand-marked page
- [x] 1.2 Add the page-set check — `docs/crds/` page set must equal the
      `chart/crds/` kind set, both directions — and verify `--check` fails on
      a missing or extra page
- [x] 1.3 Add the diagram edge-label check — every `data-field` in the
      diagram include names a field present in `chart/crds/` — and verify
      `--check` fails on a renamed field
- [x] 1.4 Delete the `cr-reference.md` generation and `docs/cr-reference.md`
      itself, and verify `--check` passes with both gone

## 2. The thirteen spec pages — frontend-developer

- [x] 2.1 Create `docs/crds/<kind>.md` for all thirteen kinds, each with
      front matter, permalink `/crds/<kind>/`, hand prose and the two
      generated blocks, PORTING the section order and YAML comment style from
      mockup tab 3 — verify by running `docs-generate.py` and building the
      site
- [x] 2.2 Write each kind's opening prose and field meanings in the model's
      vocabulary (wiring/behaviour/starts), verified against
      `.claude/rules/terminology.md` and the retired-vocabulary scan
- [x] 2.3 Add the CRDs nav group to `docs/_data/nav.yml` and verify each page
      renders with its entry marked current
- [x] 2.4 Update every in-repo pointer at `cr-reference.md` (README's "The
      reference", `docs/index.md`'s paths onward, guides) to the new pages,
      verified by a tree-wide grep returning nothing

## 3. The Introduction's domain-model diagram — frontend-developer

- [x] 3.1 PORT the mockup tab-2 SVG into a `docs/_includes/` fragment —
      thirteen kinds, four clusters, console glyphs copied verbatim from
      `platform/console/ui/src/graph/shapes.tsx` (source named in a
      comment), `currentColor` + `--ao-*` theming, `data-field` on labelled
      edges — and verify it renders in both themes
- [x] 3.2 Replace `docs/introduction.md`'s concept cards with the inlined
      diagram plus one or two sentences per kind, each linking
      `/crds/<kind>/`, and verify every shape and sentence link lands on its
      page
- [x] 3.3 Verify connector semantics against the mockup: arrowheads only on
      directional relations, plain lines on references, one connector per
      cluster pair

## 4. Landing opening and presentation removal — frontend-developer

- [x] 4.1 Replace `docs/index.md`'s two beat lists with the `readme-flow`
      picture (named once, `themed.js`-resolved, alt text stating the
      model), PORTING the panel composition from mockup tab 1, and verify
      the tab set renders picture / recording / manifest in that order
- [x] 4.2 Delete the `.ao-presentation` component — CSS, script and any
      layout hook — and verify a tree-wide grep for `ao-presentation` finds
      only records of its removal
- [x] 4.3 Add `.ao-presentation` to `.github/retired-vocabulary.json` and
      verify `retired-vocabulary-guard.py` passes

## 5. Collapsible navigation — frontend-developer

- [x] 5.1 Wrap each sidebar group in `<details open>`/`<summary>` in the
      sidebar include, PORTING the look from mockup tab 4, and verify
      keyboard toggling and the current page's group opening
- [x] 5.2 Add the state script — persist a reader's closed groups, always
      open the current page's group — and verify the choice survives
      navigation and that a no-script render shows every group open

## 6. Unit tests

- [x] 6.1 Extend the docs-generate self-tests (or add them where the script
      has none) to cover the two new block kinds, the page-set check and the
      edge-label check, and verify the suite passes
- [x] 6.2 Run the site build and the docs lint from `docs/CLAUDE.md`
      (semicolon/paragraph awk, `docs-generate.py --check`) over the new and
      changed pages, all green

## 7. System tests

- [x] 7.1 Nothing here is decided by a cluster — the change is site pages,
      theme assets and a generator script. No kubelet, RBAC, informer or pod
      lifecycle is touched.

## 8. Documentation

Reference docs:

- [x] 8.1 `docs/CHANGELOG.md` — entry recording `cr-reference.md`'s removal
      and the per-kind pages that replace it
- [x] 8.2 Re-run `python3 .github/scripts/docs-generate.py` and commit the
      output, `--check` green
- [x] 8.3 `docs/CLAUDE.md` — page table rows for `crds/*.md`, the
      `.ao-presentation` component row removed, the cr-reference bullets
      replaced — and `docs/.claude/site.md` — presentation sections replaced
      by the flow-picture facts

Adopter site:

- [x] 8.4 Build the site, serve it, and screenshot the landing opening, the
      Introduction diagram (both themes) and one CRD page — the Playwright
      assertions from `docs/CLAUDE.md` (no horizontal overflow, both color
      schemes) green on every changed page
- [x] 8.5 `README.md` — "The reference" pointer updated to the site's CRD
      pages, line budget still ≤215
