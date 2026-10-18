## 1. Rework the contract page into the specification — api-architect

- [x] 1.1 Confirm the precondition: `runtime-specification` is archived
      (#373), so `docs/contracts/signal-adapter.md` exists. **Correction:**
      `docs-generate.py` does NOT have a `struct-table` marker — it was added
      and removed within `runtime-specification` itself, in favor of
      hand-written requirement-ID tables (`runtime.md`'s actual shape). This
      change follows that shape. See `design.md`.
- [x] 1.2 Rework `docs/contracts/signal-adapter.md` into the versioned
      (1.0.0) specification per the delta spec, published like `runtime.md`
      (front matter, permalink, nav entry): both parties' obligations, the
      inbound/listing/state endpoints, type-scoped auth, manager-side
      grouping and cooldown, honest fingerprinting, the self-exclusion
      invariant cited rather than paraphrased, compatibility rules and the
      changelog at the foot. Verify every normative sentence traces to
      source or a test.
- [x] 1.3 Hand-write the request/response field tables (requirement-ID
      style, matching `runtime.md`'s shape) for the inbound signal, the
      source listing and the state/status calls. Nothing generates them —
      verify `docs-generate.py --check` stays clean regardless since no
      marker targets this page.
- [x] 1.4 Write the Conformance section as a minimal/optional split
      (matching `runtime.md`'s final shape, not a per-item test registry).
      Verify by reading each of `signals/cron`, `signals/alertmanager`,
      `signals/k8s-events`, `signals/ha` and `signals/telegram`'s own tests
      against every requirement. Fix any gap in this change before the
      version is claimed.

## 2. Rewrite the adopter guide as the station procedure

- [x] 2.1 Rewrite `docs/guides/signal-adapter.md` as ordered stations — each
      citing a spec section by anchor, each ending in a verification. Verify
      the guide states no requirement of its own and every cited anchor
      exists.
- [x] 2.2 Re-run `docs-generate.py` for the guide's generated blocks and
      verify `--check` is clean.

## 3. Governance rule and version claims

- [x] 3.1 Extend `.claude/rules/signal-rules.md` (add the signal endpoints in
      `internal/httpapi/` to its `paths:`) with the two obligations (verify before claiming done, never edit the
      specification without the user's approval). Verify with
      `rules_compliance.py` and the authoring rules.
- [x] 3.2 Add the claimed specification version (1.0.0) to
      `docs/integrations/home-assistant.md`, `docs/integrations/prometheus.md`,
      `docs/integrations/kubernetes.md` and `docs/integrations/telegram.md`,
      and claim cron's on the guide, stated as cron's. Verify each page states
      it.

## 4. Unit tests — testing-specialist

- [x] 4.1 A link-and-anchor check over every file the change touched: every
      relative link resolves to a file and every anchor to a heading. Verify
      it passes on this working copy's tree. **Note:** `anchor-check.py`
      found one broken anchor this change caused
      (`docs/concepts.md`'s link into the old narrative page's single
      heading) and it is fixed. The remaining broken links/anchors it
      reports are pre-existing — `docs/changelog/` archives (append-only)
      and three unrelated `docs/concepts.md` anchors — confirmed against
      `origin/master` before this change touched either file.
- [x] 4.2 `rules_compliance.py`, `retired-vocabulary-guard.py` and
      `publication-guard.py` clean over every file the change touched.
      Verified by running all three. `rules_compliance.py` flags pre-existing
      long-paragraph/semicolon findings in `docs/integrations/kubernetes.md`,
      `docs/concepts.md` and `docs/CLAUDE.md`, confirmed present on
      `origin/master` before this change and untouched by it. Every line this
      change added is clean.

## 5. System tests — testing-specialist

- [x] 5.1 Nothing here is decided by a cluster — the change is documentation
      and context rules. No manager, adapter, chart or RBAC behavior changes.

## 6. Documentation

- [x] 6.1 Reference docs: `python3 .github/scripts/docs-generate.py --check`
      is clean over the whole tree. `docs/contracts.md` already points at
      the signal page (unchanged, same form as the runtime row), and
      `.claude/rules/documentation.md`'s routing table already carries the
      "Runtime/channel/signal contract" row (`docs/contracts/<contract>.md`),
      which names this page by its path pattern — verified, no edit needed.
- [ ] 6.2 Adopter site: build the site from this working copy's tree and look
      at the changed pages per `docs/CLAUDE.md` pre-flight — stations render,
      command fences pair, no table crushes, no horizontal overflow in either
      theme. **NOT PERFORMED** — this is a remote session with no running
      Docker daemon (`.claude/rules/remote-session.md`: the visual check is
      WORKSTATION-ONLY). Recorded as not performed, not done.
- [x] 6.3 Adopter site: the prose lint in `docs/CLAUDE.md` (the awk script,
      run directly, not only via the session hook) is silent over
      `docs/contracts/signal-adapter.md` and `docs/guides/signal-adapter.md`.
      `docs/_data/nav.yml` carries the new Specifications entry, and the
      guide's own permalink (`/guides/signal-adapter/`) is unchanged, so
      every existing link to it still resolves.
