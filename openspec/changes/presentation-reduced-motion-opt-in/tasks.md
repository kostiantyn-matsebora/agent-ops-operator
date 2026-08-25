## 1. The still keeps its control

- [x] 1.1 In `docs/assets/js/presentation.js`, stop removing the rail on the
  reduced-motion path: keep `rail` in the wrap and detach only the caption, the
  dots and the progress bar, holding references so they can be re-appended in
  their authored order. Verify by loading the landing page with reduced motion
  emulated — the play button is present, the beat text and dots are not.
- [x] 1.2 Add the one-way `engage()` that the play button runs while still: take
  the beat list back out, re-append the caption, dots and progress bar to the
  rail, re-insert the stanza box above it, then `goTo(0)` and `start()`.
  Verify the presentation runs from beat one with its full transport, and that
  pausing afterwards leaves the transport in place rather than returning to the
  still.
- [x] 1.3 Keep every non-reduced-motion path byte-for-byte in behaviour: at
  load `goTo(0)` then `start()`, no list, no still rail. Verify by loading the
  page with no preference set — the presentation autoplays exactly as before.

## 2. The still rail looks like a control, not a stranded button

- [x] 2.1 In `docs/assets/css/agentops.css`, give the still configuration its
  own rail rule so a rail holding only the play button collapses to that
  button's width instead of rendering as a wide empty bar. Verify against both
  themes at a wide and a narrow viewport.

## 3. Verify it on the real page

- [x] 3.1 Screenshot the landing page from the WORKTREE's tree (not master's)
  with `prefers-reduced-motion: reduce` emulated — a Playwright context takes
  `reducedMotion: 'reduce'`, and `platform/console/ui` already carries the
  dependency. Write the PNG to the scratchpad, never the repo, and READ it:
  the drawing is composed, the beats are a list, one play button is visible.
- [x] 3.2 Shoot the same page again after clicking that button, and read it:
  the list is gone, the stanza box is showing beat one's lines, the caption
  reads `1 / 10`, and the dots are present.
- [x] 3.3 Shoot it once more with no preference emulated, and read it: nothing
  about the ordinary reader's page changed.

## 4. Documentation

### 4.1 Reference docs

- [x] 4.1.1 `docs/CLAUDE.md` — the `{: .ao-presentation}` component row states
  what a reader gets with no scripting and says nothing about reduced motion.
  Add the sentence: the still is the default and the play button is kept, so
  the next author does not re-delete the control. Verify the row still reads as
  one row and the page's lint passes.

### 4.2 The adopter site

- [x] 4.2.1 Re-read `docs/index.md`, `docs/introduction.md`,
  `docs/getting-started.md`, `docs/installation.md` and `docs/guides/` for any
  sentence this change made untrue, and fix what it finds. Verify by recording
  the verdict — which pages were read and that none described the
  presentation's motion behaviour — rather than by asserting it.
  **Verdict:** `docs/index.md`, `docs/introduction.md`,
  `docs/getting-started.md`, `docs/installation.md`, `docs/guides/*.md` and
  `README.md` searched for motion, animation and autoplay claims — 0 matches, no
  page edited. The prose lint reports nothing in the file this change touched
  (`docs/CLAUDE.md`); its findings elsewhere are master's and untouched here.
  `publication-guard.py` and `retired-vocabulary-guard.py`: both clean.
- [x] 4.2.2 Confirm `docs/index.md` needs no edit: the page writes the beat
  list and the theme supplies the controls. Verify the rendered landing page is
  unchanged in copy against master's.
