## 1. Implementation

- [x] 1.1 In `docs/assets/js/presentation.js`, stop reinserting the beat `<ol>`
      after the drawing in the reduced-motion (`still`) branch — leave it
      detached, exactly as the non-still branch already does — and verify by
      reading the diff that `wrap.parentNode.insertBefore(list, ...)` is gone
      from that branch.
- [x] 1.2 Give the still a discoverable affordance: set `wrap`'s `aria-label`
      to the base label plus `" — press to play"` when the still is composed,
      and restore the base label in `engage()`. Verify by reading the diff
      that both the set and the restore are present and that `engage()` no
      longer calls `list.parentNode.removeChild(list)` (dead now that the
      list is never reinserted).
- [x] 1.3 In `docs/assets/css/agentops.css`, add a
      `.ao-pres.is-still .ao-pres-caption::after` rule with a `· press to
      play` cue, declared after the existing `.ao-pres.is-paused` rule so the
      cascade's source-order tie-break gives it priority when both classes
      are present. Verify by reading the diff that the new rule sits after
      the `is-paused` one and mirrors its properties (font, size, colour).
- [x] 1.4 Rewrite the reduced-motion comment block in `presentation.js`
      (currently describing the deleted play-button rail) to describe the
      current behaviour, and add one sentence to the file's header comment
      naming what reduced motion does. Verify by reading the comments back —
      no mention of a "play button" or "rail" remains anywhere in the file.
      (The three surviving mentions are explicitly past-tense — "There was a
      transport", "It went through a bordered rail", "The rail is gone now"
      — explaining why the current design is what it is, the same historical
      framing the file already used before this change.)

## 2. Unit tests

- [x] 2.1 There is no automated test harness for `docs/assets/js/` (no
      `package.json`, no JS test runner under `docs/`) — the site is static
      Jekyll output with hand-written JS, per `docs/CLAUDE.md`. Verification
      is the visual check below, against THIS WORKTREE's own copies of
      `docs/assets/js/presentation.js` and `docs/assets/css/agentops.css` —
      never a previously built or deployed copy.

      **Deviation from the task as written, recorded rather than silently
      substituted:** `docs/CLAUDE.md`'s prescribed method is the
      `jekyll/jekyll:4` container, but the Rancher Desktop docker daemon was
      not running in this session (`docker info` — "Cannot connect to the
      Docker daemon") and starting it was out of scope for a CSS/JS fix.
      Verified instead with a minimal static harness page serving THIS
      WORKTREE's actual `presentation.js` and `agentops.css` unmodified,
      with an `<ol class="ao-presentation">` matching `index.md`'s ten real
      beats byte-for-byte, under `python3 -m http.server`, driven by the
      Playwright install already present at
      `platform/console/ui/node_modules/playwright-core` — same tool
      `visual-check.md` prescribes for a UI change. This exercises the real
      shipped assets against real markup; only Jekyll's markdown-to-HTML
      templating (unchanged by this proposal) is not exercised.

      With `page.emulateMedia({ reducedMotion: 'reduce' })`: confirmed via
      `page.evaluate` and a screenshot that the drawing is fully composed,
      `wrap` carries `is-still` + `is-paused`, `aria-label` reads "How it
      works, one beat at a time — press to play", the caption's computed
      `::after` content resolves to `" · press to play"` (the `is-paused`
      rule's `" · paused"` does NOT win the cascade tie), and no `ol` remains
      in `#ao-content` — no duplicated beat list. Clicking the figure then
      confirmed `is-still` is removed, `aria-label` reverts to the base
      string, and the caption shows beat one's text ("Something happens."),
      matching `engage()`'s contract.
- [x] 2.2 Repeat the same harness with no reduced-motion emulation and confirm
      the ordinary (non-reduced-motion) presentation is unchanged — `wrap`
      carries neither `is-still` nor `is-paused`, `aria-label` is the base
      string, and no `ol` is in the document (unaffected by this change,
      confirmed rather than assumed).

## 3. E2E tests

- [x] 3.1 Not applicable — nothing here is decided by a cluster. This is a
      static site asset (JavaScript and CSS served by GitHub Pages), with no
      Kubernetes object, controller or kubelet behaviour involved.

## 4. Documentation

### Reference docs

- [x] 4.1 Update the `{: .ao-presentation}` row in `docs/CLAUDE.md`'s
      "Components a page may name" table to add one sentence stating the
      current reduced-motion behaviour (the figure is still the control, the
      still carries a `press to play` cue, no beat list is duplicated below
      it). Verify by re-reading the row: it names reduced motion and makes no
      claim the code no longer does.
- [x] 4.2 Add the same one-sentence addition to the landing-page bullet in
      `docs/.claude/site.md` ("IT IS ONE FIGURE, AND THE FIGURE IS THE
      CONTROL"). Verify by re-reading that section for the same claim,
      worded consistently with `docs/CLAUDE.md`'s row.

### Adopter site

- [x] 4.3 Confirm `docs/index.md` needs no changes — the beats and the tab
      strip's wording are unaffected, since only the theme's script and
      stylesheet change. Verify by re-reading `docs/index.md`'s `{: .ao-tabs
      #tour}` section against the proposal's Impact section: no claim there
      is made untrue by this change. Confirmed: `git diff --stat docs/index.md`
      is empty.
