## 1. Rework the contract page into the specification — api-architect

- [x] 1.1 Confirm the precondition: `runtime-specification` is archived, so
      `docs/contracts/channel-adapter.md` exists. `docs-generate.py` has NO
      `struct-table` marker — `runtime-specification` tried one and reversed
      it (its own task 3.3) before archiving. Verified both in the tree.
      Updated the proposal, design and delta spec to hand-write the
      struct-backed tables instead, matching `docs/contracts/runtime.md`.
- [x] 1.2 Reworked `docs/contracts/channel-adapter.md` into the versioned
      (1.0.0) specification per the delta spec: both parties' obligations,
      the ops lifecycle, auth, backpressure, pacing, the no-relay-loop rule,
      compatibility rules and the changelog at the foot. Every normative
      sentence traces to source or a test.

      **Resolved the api-architect dispatch's open question**: the delta
      spec's original "reference page with no front matter" line
      contradicted `docs/_data/nav.yml`'s own comment, which already named
      this change as the one promoting the page (same wording the
      archived `runtime-specification` change left unreconciled for
      `docs/contracts/runtime.md` — a pre-existing drift there, out of
      this change's scope to fix). Corrected the delta spec's Requirement 1
      to require the promotion, then added front matter (title, permalink
      `/contracts/channel-adapter/`, description, `next:` card) to the
      page, added its "Specifications" nav entry, and updated
      `docs/.claude/site.md` and `nav.yml`'s own comment to say channel
      adapter is now promoted and only signal adapter still awaits it.
- [x] 1.3 Hand-write the struct-backed op and payload field tables, in the
      same `| Field | Type | Required | Description |` shape
      `docs/contracts/runtime.md` uses, traced to each struct's json tags.
      Keep the body grammar hand-written and named as such. Verify
      `docs-generate.py --check` is clean (no generator touches these
      tables) and every field in the named structs has a row.
- [x] 1.4 Write the numbered conformance checklist, each item citing the
      `channels/telegram` and `platform/console` test that exercises it where
      one exists. Verify both shipped adapters pass every item by reading
      their tests, recording any gap as a gap.
- [x] 1.5 Strip any channel contract statement left in `docs/contracts.md`, so
      the specification page is the only statement. Verify it keeps only a
      pointer to the page. Already true going in — verified unchanged.

## 2. Rewrite the adopter guide as the station procedure

- [x] 2.1 Rewrote `docs/guides/channel-adapter.md` as 7 numbered stations
      (take operations, complete each op, render the message, push replies
      back, declare the ChannelAdapter, declare a Channel, verify end to
      end), each citing the specification by anchor and ending in its own
      **Verify:** step. Kept the "Before you start" and "What comes next"
      framing from the previous narrative guide. Verified the guide states
      no requirement of its own (every normative claim is a citation) and
      every cited anchor exists with `anchor-check.py`.
- [x] 2.2 Re-ran `docs-generate.py` for the guide's generated
      `ChannelAdapter`/`Channel` template and example blocks (unchanged
      content, carried over verbatim). `--check` is clean.

## 3. Governance rule and version claims

- [x] 3.1 Added `.claude/rules/channel-spec.md`, scoped by `paths:` to
      `channels/**`, `platform/console/*.go`, the manager's `internal/chat/`
      and `internal/httpapi/`, and `docs/contracts/channel-adapter.md` —
      mirroring `runtime-spec.md`'s shape exactly, stating the two
      obligations (verify-against-spec, no unapproved spec edit) plus the
      conformance-carries obligation. Added the routing row to root
      `CLAUDE.md`. Verified clean with `rules_compliance.py`. Verified the
      rule's structure and scoping match `runtime-spec.md`'s working
      pattern. Live path-triggering on a file created this session could
      not be re-confirmed in this same session, since the harness's
      scoped-rule index reads the directory at session start. It loads on
      the next session that reads a matching file, same as any new scoped
      rule.
- [x] 3.2 Added the claimed specification version (1.0.0) to
      `docs/integrations/telegram.md` (conforms-to line, Jekyll `relative_url`
      link since it is a published page) and `docs/console.md` (plain
      relative link, reference page). Verified each page states it.
- [x] 3.3 `.claude/rules/documentation.md`'s existing "Runtime/channel/signal
      contract | `docs/contracts/<contract>.md`" row already resolves
      channel contract changes to `docs/contracts/channel-adapter.md`
      generically — the same row `runtime-specification` generalized for
      itself. No row change needed. Verified `docs/contracts.md`'s "## The
      channel adapter contract" heading carries only the one-line pointer,
      no restated contract.

## 4. Unit tests — testing-specialist

- [x] 4.1 Ran `anchor-check.py` over the 11 touched markdown files (the
      12th, `docs/_data/nav.yml`, is YAML). Result: 11 files, every
      relative link and anchor resolves. Zero failures, nothing to fix.
- [x] 4.2 Ran all three guards. `rules_compliance.py` over the same 11
      markdown files: clean (no output, exit 0). `retired-vocabulary-guard.py`
      tree-wide: clean, 128 files, nothing attributable to this change.
      `publication-guard.py` tree-wide: clean. Nothing to fix.

## 5. System tests — testing-specialist

- [x] 5.1 Nothing here is decided by a cluster — the change is documentation
      and context rules. No manager, adapter, chart or RBAC behavior changes.

## 6. Documentation

- [x] 6.1 `python3 .github/scripts/docs-generate.py --check` → "52 generated
      file(s) up to date." `docs/contracts.md`'s "## The channel adapter
      contract" pointer still resolves to the (now-promoted) page.

      **Pre-existing defect found, not introduced by this change, left
      unfixed as out of scope**: a plain reference page's RELATIVE `.md`
      link to a page that carries front matter (a permalink) 404s on the
      built site, because Jekyll serves the promoted page at its permalink
      instead of its original path. Verified this already held for
      `docs/claude.md`'s and `docs/contracts.md`'s existing links to
      `contracts/runtime.md` before this change touched anything — the
      same pattern `runtime-specification` shipped unreconciled. This
      change's `docs/console.md` and the unchanged `docs/contracts.md` now
      link to `contracts/channel-adapter.md` the identical way, so the
      defect now covers two pages instead of one. Flagging for the PR
      description rather than inventing a new link convention unilaterally
      — `docs/claude.md` is not this change's to edit, and a real fix
      needs a decision about every such link at once, not a one-off here.
- [x] 6.2 Docker's daemon is not reachable in this remote session (client
      present, socket absent), so built the site with a native Jekyll
      install (`gem install jekyll jekyll-default-layout jekyll-seo-tag
      jekyll-sitemap`) instead of the containerized command — same Jekyll
      version class, same `_config.yml`. Served over
      `python3 -m http.server` with the `agent-ops-operator/` baseurl
      symlink. Checked `/contracts/channel-adapter/`, `/guides/channel-adapter/`,
      `/console/`, `/integrations/telegram/` with Playwright
      (`chromium-1194`), light and dark, 1280×900: all `200`, zero
      `scrollWidth` overflow in every case. Full-page and anchor-targeted
      screenshots of both rewritten pages: tables keep their code columns
      on one line, the Description column isn't crushed, the sh/powershell
      command pairs render as tabs, the "Specifications" nav group shows
      "Channel adapter" and highlights it as current.
- [x] 6.3 Ran the exact `docs/CLAUDE.md` prose-lint `awk` one-liner over
      `docs/*.md docs/guides/*.md` (its own stated scope — `contracts/`
      and `integrations/` pages aren't in that glob, but stay covered by
      the stricter `rules_compliance.py`, run clean throughout). Found and
      fixed one hit on this change's own file,
      `docs/guides/channel-adapter.md:148` (a 49-word paragraph split in
      two). Every other hit the lint reports is in a file this change
      never touched (`CHANGELOG.md`, `concepts.md`, `configuration.md`,
      `security.md`, three other guides, root `docs/CLAUDE.md` itself) —
      pre-existing debt, left alone. Re-ran after the fix: zero hits on
      any file this change touched. The built site's sidebar
      reaches `/guides/channel-adapter/` under Guides exactly as before —
      unchanged entry, confirmed in the screenshots above.
