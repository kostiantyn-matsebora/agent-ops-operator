## Why

**A reader who has asked for reduced motion currently cannot play the landing
presentation at all, even deliberately.** `presentation.js` reads the
preference, composes the whole drawing, and then removes the transport —
`wrap.removeChild(rail)` — so the play button, the beat dots and the progress
bar are gone from the document. The preference is honoured by DELETING the
control rather than by not moving.

That is stricter than the preference asks for. `prefers-reduced-motion: reduce`
says *do not move things at me*; it does not say *never let me ask*. The reader
who turned it on to stop carousels autoplaying is the same reader who may want
to watch a nine-beat explanation of the product once, on purpose — and on that
machine the landing page's central explanation is unreachable in the form it
was authored in.

It was reported from a second machine as "the animation is missing", which is
also what a broken script looks like. A visible, paused play button says the
motion is there and is waiting to be asked for.

## What Changes

- **The reduced-motion default is unchanged**: at load nothing moves, the
  drawing is fully composed, the beats are the list the page wrote.
- **The play button SURVIVES the reduced-motion path.** It is the one control
  kept; the caption, the dots and the progress bar stay out until the reader
  engages, so nothing duplicates the list beside it.
- **Pressing it opts in**: the beat list is taken back out, the stanza box
  returns, and the presentation plays from the first beat exactly as it does
  for every other reader. Pausing after that leaves the ordinary transport in
  place — the reader has already said what they want.
- **No change for a reader with no scripting**, and no change for a reader who
  has not asked for reduced motion.
- Not breaking: no page markup changes, and the component is still driven by the
  ordered list `index.md` writes.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `landing-presentation`: the reduced-motion requirement becomes *nothing moves
  UNTIL THE READER ASKS* — the still composition plus a play control — and gains
  the scenario for engaging it.

## Impact

**Code**

- `docs/assets/js/presentation.js` — the reduced-motion branch keeps the rail,
  hides the beat-level controls, and gains the one-way `engage()` that converts
  the still into the running presentation.
- `docs/assets/css/agentops.css` — the rail is a three-column grid authored for
  a full transport; a still rail carrying only the play button needs its own
  rule or it renders as a button stranded in a wide bar.

**Reference docs**

- `docs/CLAUDE.md` — the `{: .ao-presentation}` component row states what the
  page gets without scripting and says nothing about reduced motion. It gains
  that sentence, because an author reading the row is the person who would
  otherwise re-delete the control.

**The adopter site**

- `docs/index.md` — no copy changes. The page writes the list and the theme
  supplies the controls, which is what keeps this change out of the page.
- No landing-page claim, Introduction, Getting started, Installation or guide
  says anything about how the presentation behaves under reduced motion, so
  none of them is made untrue. Checked rather than assumed.

**Not affected**

- `docs/CHANGELOG.md` — no chart value, CRD field or upgrade step moves.
- Every other `.ao-*` component. The theme's global reduced-motion rule
  (`agentops.css`, the `*` transition-duration clamp) is untouched.
