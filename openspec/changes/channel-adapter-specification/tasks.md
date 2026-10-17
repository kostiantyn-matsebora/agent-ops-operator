## 1. Rework the contract page into the specification — api-architect

- [ ] 1.1 Confirm the precondition: `runtime-specification` is archived, so
      `docs/contracts/channel-adapter.md` exists and `docs-generate.py` has
      the `struct-table` marker. Verify both in the tree.
- [ ] 1.2 Rework `docs/contracts/channel-adapter.md` into the versioned
      (1.0.0) specification per the delta spec: both parties' obligations,
      the ops lifecycle, auth, backpressure, pacing, the no-relay-loop rule,
      compatibility rules and the changelog at the foot. Verify every
      normative sentence traces to source or a test.
- [ ] 1.3 Place `struct-table` markers for the struct-backed op and payload
      tables, keep the body grammar hand-written and named as such, run the
      generator. Verify `docs-generate.py --check` is clean.
- [ ] 1.4 Write the numbered conformance checklist, each item citing the
      `channels/telegram` and `platform/console` test that exercises it where
      one exists. Verify both shipped adapters pass every item by reading
      their tests, recording any gap as a gap.
- [ ] 1.5 Strip any channel contract statement left in `docs/contracts.md`, so
      the specification page is the only statement. Verify it keeps only a
      pointer to the page.

## 2. Rewrite the adopter guide as the station procedure

- [ ] 2.1 Rewrite `docs/guides/channel-adapter.md` as ordered stations — each
      citing a spec section by anchor, each ending in a verification. Verify
      the guide states no requirement of its own and every cited anchor
      exists.
- [ ] 2.2 Re-run `docs-generate.py` for the guide's generated blocks and
      verify `--check` is clean.

## 3. Governance rule and version claims

- [ ] 3.1 Add the scoped rule (loads under `channels/**`, the console's
      adapter surface and the manager's channel surface) stating the three
      obligations. Verify with `rules_compliance.py` and the authoring rules.
- [ ] 3.2 Add the claimed specification version (1.0.0) to
      `docs/integrations/telegram.md` and `docs/console.md`. Verify each page
      states it.

## 4. Unit tests — testing-specialist

- [ ] 4.1 A link-and-anchor check over every file the change touched: every
      relative link resolves to a file and every anchor to a heading. Verify
      it passes on this working copy's tree.
- [ ] 4.2 `rules_compliance.py`, `retired-vocabulary-guard.py` and
      `publication-guard.py` clean over every file the change touched. Verify
      by running all three.

## 5. System tests — testing-specialist

- [x] 5.1 Nothing here is decided by a cluster — the change is documentation
      and context rules. No manager, adapter, chart or RBAC behavior changes.

## 6. Documentation

- [ ] 6.1 Reference docs: `python3 .github/scripts/docs-generate.py --check`
      clean over the whole tree, and `docs/contracts.md`'s pointer to the
      channel page still resolves.
- [ ] 6.2 Adopter site: build the site from this working copy's tree and look
      at the changed pages per `docs/CLAUDE.md` pre-flight — stations render,
      command fences pair, no table crushes, no horizontal overflow in either
      theme.
- [ ] 6.3 Adopter site: the prose lint in `docs/CLAUDE.md` is silent over the
      changed pages, and the nav still reaches the rewritten guide.
