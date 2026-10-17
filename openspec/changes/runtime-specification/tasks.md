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

## 3. Generated schema tables — deployment-engineer — REVERSED, see 3.3

- [x] 3.1 Add a `struct-table` marker to `.github/scripts/docs-generate.py`:
      given a Go file and struct name, render a table row per json-tagged
      field from tags and doc comments, failing loudly on a struct it cannot
      parse. Verify with the script suite.
- [x] 3.2 Place markers in `docs/contracts/runtime.md` for the work unit
      struct (`platform/manager/internal/dispatch/dispatch.go`) and the report struct
      (`platform/manager/internal/httpapi/`), run the generator, and verify
      `docs-generate.py --check` is clean and the tables match the structs.
- [x] 3.3 Reversed. The Go doc comments written to feed 3.1 carried full
      rationale paragraphs (why a field exists, what it replaced, when to
      trust it), because that is normal Go doc-comment style — and the
      generator copied them verbatim into the "Meaning" column, producing
      unreadable, unmaintainable table cells no reader asked for, plus 300+
      lines of parser and its own test suite to keep working. Removed
      `build_struct_table`/`_parse_go_struct_fields`/`_humanize_go_type` and
      the `struct-table` marker from `docs-generate.py`, and deleted
      `.github/tests/struct-table.test.py`/`.test.sh`. The two field tables
      in `docs/contracts/runtime.md` are now hand-written, in the same
      concise `| Field | Type | Required | Description |` shape
      `cr-reference.md`'s own generator already uses. Verify:
      `docs-generate.py --check` clean with no `struct-table` marker left
      anywhere, and `.github/tests/run.sh` green with the two test files
      gone.

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

- [x] 6.1 Superseded by 3.3 — the `struct-table` marker these tests covered
      no longer exists. Verify `.github/tests/run.sh` passes with the test
      files gone and no `struct-table` reference left in the script suite.
- [x] 6.2 A link-and-anchor check over `README.md`, `CLAUDE.md` and
      `docs/**`: every relative link resolves to a file and every anchor to a
      heading. Verify it passes on this working copy's tree.
- [x] 6.3 `rules_compliance.py`, `retired-vocabulary-guard.py` and
      `publication-guard.py` clean over every file the change touched. Verify
      by running all three.
- [x] 6.4 A scope test for `.claude/rules/runtime-spec.md`: every `paths:` glob
      in its frontmatter matches at least one tracked file, and a file under
      each named path (`runtimes/**`, the manager's dispatch, runtimepod and
      httpapi packages, `docs/contracts/runtime.md`) is matched by the rule.
      Verify it passes and fails when a glob is broken.

## 7. System tests — testing-specialist

- [x] 7.1 Nothing here is decided by a cluster — the change is documentation,
      context rules and a docs generator. No manager, runtime, chart or RBAC
      behavior changes.

## 8. Documentation

- [x] 8.1 Reference docs: `python3 .github/scripts/docs-generate.py --check`
      clean over the whole tree, and the slimmed `docs/contracts.md` still
      states where each moved contract lives.
- [x] 8.2 Adopter site: build the site from this working copy's tree and look
      at the changed pages per `docs/CLAUDE.md` pre-flight — the guide's
      stations render, command fences pair, no table crushes, no horizontal
      overflow in either theme. No `docker` in this remote session, so built
      with the Ruby/Jekyll toolchain already on the machine instead
      (`jekyll build`, served at the `agent-ops-operator/` baseurl) and
      checked with the pre-installed Chromium via `playwright-core`, both
      `colorScheme`s, at 1280×1000: `guides/agent-runtime/`,
      `runtimes/ollama/`, `runtimes/copilot/` and `/` all return `200` with
      `scrollWidth` never exceeding `clientWidth`. Screenshots read by hand —
      the guide's numbered stations, code fences and tables render cleanly,
      and each runtime page's new "conforms to the runtime specification"
      line sits correctly under its opening sentence.
- [x] 8.3 Adopter site: verify `index.md`, the nav and `getting-started.md`
      still reach the contracts and the rewritten guide, and the prose lint in
      `docs/CLAUDE.md` is silent over the changed pages. `docs/index.md`'s
      reference bullet was stale (still pointed at one `contracts.md` for
      "the work and adapter contracts") and is fixed to name the split pages.
      `_data/nav.yml`'s `/guides/agent-runtime/` link is untouched and still
      resolves (the guide kept its permalink). `getting-started.md` names no
      contract page and needed no change. The lint (`rules_compliance.py`
      and the raw awk from `docs/CLAUDE.md`) is silent over every page this
      change wrote or edited. The handful of hits elsewhere all pre-date
      this change, confirmed by `git blame` to commit `dacd810`.
- [x] 8.4 Publish `docs/contracts/runtime.md` as a site page instead of a
      GitHub-only reference: front matter, permalink `/contracts/runtime/`,
      a `next:` card back to the generated CR reference, and a new
      "Specifications" nav group in `_data/nav.yml` (`AgentRuntime` →
      `/contracts/runtime/`) — the same promotion the Integrations group
      already got once. Every site-page link to it (`guides/agent-runtime.md`,
      `runtimes/ollama.md`, `runtimes/copilot.md`) retargeted to
      `{{ '/contracts/runtime/' | relative_url }}`, anchors preserved.
      Reference-only pages (`docs/claude.md`, `docs/contracts.md`,
      `docs/concepts.md`, `docs/CHANGELOG.md`) keep their plain relative
      links since they're read on GitHub, not through the site. Recorded the
      convention in `docs/CLAUDE.md` and `docs/.claude/site.md` so
      `contracts/channel-adapter.md`/`contracts/signal-adapter.md` get the
      same treatment only in the change that rewrites each to this same
      versioned-spec standard. Verified: `anchor-check.py` reports the same
      13 pre-existing breaks and none in a touched file,
      `publication-guard.py` and `retired-vocabulary-guard.py` clean,
      `docs-generate.py --check` clean, and the site rebuilt and screenshotted
      (light/dark, 1400×1200) — `/contracts/runtime/` renders with the H1 from
      front matter, its on-this-page rail lists every section, the sidebar
      highlights "AgentRuntime" under "Specifications", and
      `scrollWidth`/`clientWidth` never diverge on `/contracts/runtime/` or
      `/guides/agent-runtime/`.
- [x] 8.5 Rewrite `docs/contracts/runtime.md` to an actual API specification
      shape instead of narrative reference prose: a `## Conventions` section
      (RFC 2119 keywords, the requirement-ID scheme), the two HTTP
      operations as top-level sections (`GET /work`, `POST /work/done`)
      ahead of everything else, each with its request/response table and a
      short `| ID | Requirement |` table — no prose paragraph states more
      than one rule. `Lifecycle`, `Runtime's obligations`, `Manager's
      provisions`, `Security posture` and `Compatibility` get the same
      `| ID | Requirement |` treatment. The Conformance checklist is now a
      `| # | Verifies | Tests |` traceability table citing the requirement
      IDs, not 14 paragraphs mixing a claim with its test citations. See 3.3
      for the generated-table reversal this depended on. Two headings
      renamed for the new structure (`Work unit` → `GET /work`, `Completion
      report` → `POST /work/done`). Every other anchor is unchanged, so
      `docs/guides/agent-runtime.md`'s ~15 links were retargeted only where
      they pointed at a renamed anchor or a claim that moved sections (the
      long-poll and context-restoration claims, which moved from `Runtime's
      obligations`/`Manager's provisions` into the new `GET /work` section).
      Verify: the repo's own prose lint (`docs/CLAUDE.md`, "Before calling a
      page done") is silent over both files, `anchor-check.py` reports the
      same pre-existing 13 breaks and none in a touched file, and the site
      rebuilt and screenshotted shows the new structure rendering cleanly.
- [x] 8.6 8.5 was still missing what a real API reference states: the actual
      status codes and response bodies (read from `handleWork`/
      `handleWorkDone` in `platform/manager/internal/httpapi/server.go`,
      not assumed) and one worked example per operation. Added to both `GET
      /work` (the `convo`/`pod`/`wait` query params, the server-side 30s
      clamp on `wait`, the `400 {"error": "missing convo"}` case) and `POST
      /work/done` (`200 {"ok": true}`, `400`/`404`/`500` bodies, one example
      request). Verify: the two response bodies and the `wait` clamp match
      `server.go` line by line, the prose lint and `docs-generate.py
      --check` stay clean, and the rebuilt site shows both JSON examples
      rendering as code blocks with no overflow.
- [x] 8.7 8.6's Conformance checklist still cited this repo's own test file
      names (`runtime.mainloop.test.js`, `work_test.go`) against each
      requirement — information only agent-ops' own CI needs, not a third
      party building their own runtime. Replaced with a two-row minimal/
      optional split: the full MUST id range states the minimal conformant
      contract, [Optional capabilities](../contracts/runtime.md#optional-capabilities)
      states what adds nothing to it. Also cut, as the same class of
      problem: the intro's restatement of what `GET /work`/`POST
      /work/done` do (already the next two headings), five `Source:
      platform/...` lines citing internal Go file paths with no normative
      content, and API-3 (one inflight unit per conversation), which
      duplicated LC-1 under Lifecycle word for word — API-4 through API-10
      renumbered down to API-3 through API-9, the one external cross-reference
      in `docs/guides/agent-runtime.md` updated to match. Verify: every
      requirement ID row is contiguous with no gap or collision
      (`grep -n` the four tables), the prose lint and `anchor-check.py`
      stay clean, and the full script suite still passes.
- [x] 8.8 Two readability fixes on the same page: the version tag and the
      purpose sentence were one run-on line (`**Version 1.0.0.** This
      specification...`) — split onto their own lines, version as its own
      paragraph. The Conventions table's "Requirement ID" row packed all
      six prefixes into one comma-separated cell (`API-n`, `LC-n`
      (lifecycle), ...) — replaced with a two-column `Prefix | Section`
      table, one prefix per row, each linking to the section it marks.
      Verify: the prose lint stays clean, `anchor-check.py` resolves all
      six new section links, and the rebuilt site shows the intro and
      Conventions table rendering as distinct, scannable blocks.
- [x] 8.9 Two closing fixes, GitHub REST API docs as the model. `GET
      /work`/`POST /work/done` jumped straight from the heading to the
      request line with no description — the one-sentence summary that
      belongs there (not in the intro, see 8.6/8.7) had been dropped
      entirely rather than moved. Added one line under each heading:
      "Long-polls for this conversation's next work unit." /
      "Reports a run's outcome and closes out the inflight unit." The
      Changelog's `1.0.0` row described what the document now contains, as
      prose — meaningless for a first entry with no prior version to
      change from. Trimmed to "Initial release.", relying on
      `docs/contracts.md`'s own existing pointer to this page for
      provenance instead of restating it. Verify: the prose lint and
      `anchor-check.py` stay clean, and the rebuilt site shows a
      description under both endpoint headings.
