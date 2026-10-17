## 1. Rework the contract page into the specification — api-architect

- [ ] 1.1 Confirm the precondition: `runtime-specification` is archived, so
      `docs/contracts/signal-adapter.md` exists and `docs-generate.py` has
      the `struct-table` marker. Verify both in the tree.
- [ ] 1.2 Rework `docs/contracts/signal-adapter.md` into the versioned
      (1.0.0) specification per the delta spec: both parties' obligations,
      the inbound endpoint, type-scoped auth, manager-side grouping and
      cooldown, honest fingerprinting, the self-exclusion invariant cited
      rather than paraphrased, compatibility rules and the changelog at the
      foot. Verify every normative sentence traces to source or a test.
- [ ] 1.3 Place the `struct-table` marker for the inbound payload table and
      run the generator. Verify `docs-generate.py --check` is clean.
- [ ] 1.4 Write the numbered conformance checklist, each item citing the
      shipped adapter test that exercises it where one exists, across
      `signals/cron`, `signals/alertmanager`, `signals/k8s-events`,
      `signals/ha` and `signals/telegram`. Verify all five pass every item by
      reading their tests. Fix any gap in this change before the version is
      claimed.

## 2. Rewrite the adopter guide as the station procedure

- [ ] 2.1 Rewrite `docs/guides/signal-adapter.md` as ordered stations — each
      citing a spec section by anchor, each ending in a verification. Verify
      the guide states no requirement of its own and every cited anchor
      exists.
- [ ] 2.2 Re-run `docs-generate.py` for the guide's generated blocks and
      verify `--check` is clean.

## 3. Governance rule and version claims

- [ ] 3.1 Extend `.claude/rules/signal-rules.md` (add the signal endpoints in
      `internal/httpapi/` to its `paths:`) with the two obligations (verify before claiming done, never edit the
      specification without the user's approval). Verify with
      `rules_compliance.py` and the authoring rules.
- [ ] 3.2 Add the claimed specification version (1.0.0) to
      `docs/integrations/home-assistant.md`, `docs/integrations/prometheus.md`,
      `docs/integrations/kubernetes.md` and `docs/integrations/telegram.md`,
      and claim cron's on the guide, stated as cron's. Verify each page states
      it.

## 4. Unit tests — testing-specialist

- [ ] 4.1 A link-and-anchor check over every file the change touched: every
      relative link resolves to a file and every anchor to a heading. Verify
      it passes on this working copy's tree.
- [ ] 4.2 `rules_compliance.py`, `retired-vocabulary-guard.py` and
      `publication-guard.py` clean over every file the change touched. Verify
      by running all three.

## 5. System tests — testing-specialist

- [ ] 5.1 Nothing here is decided by a cluster — the change is documentation
      and context rules. No manager, adapter, chart or RBAC behavior changes.

## 6. Documentation

- [ ] 6.1 Reference docs: `python3 .github/scripts/docs-generate.py --check`
      clean over the whole tree. Update `docs/contracts.md` to point at the
      signal page and add its row to `.claude/rules/documentation.md`'s
      routing table. Verify the pointer resolves.
- [ ] 6.2 Adopter site: build the site from this working copy's tree and look
      at the changed pages per `docs/CLAUDE.md` pre-flight — stations render,
      command fences pair, no table crushes, no horizontal overflow in either
      theme.
- [ ] 6.3 Adopter site: the prose lint in `docs/CLAUDE.md` is silent over the
      changed pages, and the nav still reaches the rewritten guide.
