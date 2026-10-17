## 1. Split docs/contracts.md by extension point — api-architect

- [x] 1.1 Move the channel adapter contract verbatim to
      `docs/contracts/channel-adapter.md`, leaving a one-line pointer at the
      old heading. Verify the diff is a pure move plus the pointer.
- [x] 1.2 Move the signal adapter contract verbatim to
      `docs/contracts/signal-adapter.md`, same pointer rule, same
      verification.
- [x] 1.3 Retarget every link to a moved `contracts.md` anchor across
      `README.md`, `CLAUDE.md`, `docs/**` and `.claude/rules/**`. Verify by
      resolving every relative link and anchor in the touched files to an
      existing heading.
- [x] 1.4 Add the three contract pages to the README Documentation index.
      Verify each is reachable from the README in one hop.

## 2. Write the runtime specification — api-architect

- [x] 2.1 Write `docs/contracts/runtime.md` at version 1.0.0 per the
      `runtime-contract` delta spec: both parties' obligations, the lifecycle
      state machine, the environment table (always vs conditional, the
      empty-`REPO_URL` meaning), the filesystem contract, every enum with the
      manager's any-other-value-is-failed reading, the bounds and the
      whole-report refusal, compatibility rules with the reserved wire-version
      sentence, and the changelog at the foot. Verify every normative sentence
      traces to source or a test.
- [x] 2.2 Write the numbered conformance checklist, each item individually
      verifiable and citing the runtime test file that exercises it where one
      exists. Verify the three shipped runtimes pass every item by reading
      their tests.
- [x] 2.3 Remove the work-contract narrative from `docs/contracts.md`,
      leaving the pointer at the old anchor. Verify no work-contract
      requirement is stated in two places.

## 3. Generated schema tables — deployment-engineer

- [x] 3.1 Add a `struct-table` marker to `.github/scripts/docs-generate.py`:
      given a Go file and struct name, render a table row per json-tagged
      field from tags and doc comments, failing loudly on a struct it cannot
      parse. Verify with the script suite.
- [x] 3.2 Place markers in `docs/contracts/runtime.md` for the work unit
      struct (`platform/manager/internal/dispatch/dispatch.go`) and the report struct
      (`platform/manager/internal/httpapi/`), run the generator, and verify
      `docs-generate.py --check` is clean and the tables match the structs.

## 4. Rewrite the adopter guide as the station procedure — api-architect

- [x] 4.1 Rewrite `docs/guides/agent-runtime.md` as ordered stations — each
      citing a spec section by anchor, each ending in a verification — fixing
      the repository-checkout claim. Verify the guide states no requirement of
      its own and every cited anchor exists in the spec.
- [x] 4.2 Re-run `docs-generate.py` for the guide's generated resource blocks
      and verify `--check` is clean.

## 5. Governance rule, version claims, routing — api-architect

- [x] 5.1 Add `.claude/rules/runtime-spec.md`, scoped by `paths:` to
      `runtimes/**`, the manager's dispatch, runtimepod and httpapi packages, and
      `docs/contracts/runtime.md`, stating the three obligations
      (verify-against-spec, no unapproved spec edit, a spec change carries
      conformance). Verify with `rules_compliance.py` and the authoring rules,
      and that the rule loads when a file under each named path is read.
- [x] 5.2 Add the claimed specification version (1.0.0) to
      `docs/runtimes/ollama.md`, `docs/runtimes/copilot.md` and
      `docs/claude.md`. Verify each page states it.
- [x] 5.3 Update the routing tables — root `CLAUDE.md`,
      `.claude/rules/documentation.md`, `docs/.claude/site.md` — to name the
      per-extension-point contract pages. Verify the routing reaches each new
      page in one hop.

## 6. Unit tests — testing-specialist

- [ ] 6.1 Script-suite tests for the `struct-table` marker: a rendered table,
      a drift failure, and a loud failure on an unparsable struct. Verify
      `.github/tests/run.sh` passes.
- [ ] 6.2 A link-and-anchor check over `README.md`, `CLAUDE.md` and
      `docs/**`: every relative link resolves to a file and every anchor to a
      heading. Verify it passes on this working copy's tree.
- [ ] 6.3 `rules_compliance.py`, `retired-vocabulary-guard.py` and
      `publication-guard.py` clean over every file the change touched. Verify
      by running all three.
- [ ] 6.4 A scope test for `.claude/rules/runtime-spec.md`: every `paths:` glob
      in its frontmatter matches at least one tracked file, and a file under
      each named path (`runtimes/**`, the manager's dispatch, runtimepod and
      httpapi packages, `docs/contracts/runtime.md`) is matched by the rule.
      Verify it passes and fails when a glob is broken.

## 7. System tests — testing-specialist

- [ ] 7.1 Nothing here is decided by a cluster — the change is documentation,
      context rules and a docs generator. No manager, runtime, chart or RBAC
      behavior changes.

## 8. Documentation

- [ ] 8.1 Reference docs: `python3 .github/scripts/docs-generate.py --check`
      clean over the whole tree, and the slimmed `docs/contracts.md` still
      states where each moved contract lives.
- [ ] 8.2 Adopter site: build the site from this working copy's tree and look
      at the changed pages per `docs/CLAUDE.md` pre-flight — the guide's
      stations render, command fences pair, no table crushes, no horizontal
      overflow in either theme.
- [ ] 8.3 Adopter site: verify `index.md`, the nav and `getting-started.md`
      still reach the contracts and the rewritten guide, and the prose lint in
      `docs/CLAUDE.md` is silent over the changed pages.
