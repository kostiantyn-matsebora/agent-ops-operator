## Context

See `proposal.md` — Why. `docs/assets/js/presentation.js` builds one set of DOM
nodes (stage, caption, wrap) from the ordered list `index.md` writes, then
either plays them (`goTo`/`start`) or, under reduced motion, composes the still
and — today — reinserts the original `<ol>` after it. There is one rendering
path, not two: the still and the running presentation are the same DOM in
different states, and `wrap.classList` plus a handful of booleans
(`stillNow`, `timer`) are the whole of that state.

The `is-still` class the JS already sets has no matching CSS rule. It was
written for the `2026-08-25-presentation-reduced-motion-opt-in` design, whose
rail is gone (`docs/CLAUDE.md`, `docs/.claude/site.md`: "the figure is the
control … there is no transport"). Nothing since has reconciled the two.

## Goals / Non-Goals

**Goals:**

- The reduced-motion still shows the composed drawing and nothing else below
  it — no second copy of the beats.
- A reader — sighted or using assistive technology — can tell the drawing is a
  control before touching it.
- No new control is built. `toggle()` already calls `engage()` when
  `stillNow` is true; the fix is what surrounds that, not the mechanism.

**Non-Goals:**

- Reintroducing any separate play element. The proposal is explicit that this
  reconciles with "the figure is the control," not with the deleted rail.
- Persisting the reader's choice across pages or reloads — unchanged from the
  2026-08-25 design's own non-goal, and nothing here touches it.
- Fixing "select any beat directly" (`landing-presentation`'s "The reader
  controls the presentation" requirement) for the non-reduced-motion case.
  That capability was already dropped when the transport was deleted, and
  restoring it is a separate change with its own proposal — this one only
  brings the reduced-motion branch in line with the design that replaced it.
- Listening for a live change to `prefers-reduced-motion` — unchanged from the
  predecessor design, which reads the preference once at load.

## Decisions

**Drop the list reinsertion instead of restyling `.is-still`.** The
alternative — write a CSS rule for `.ao-pres.is-still` that visually
distinguishes the reinserted list from a normal one — keeps the duplicated
content and only makes it look intentional. The proposal's complaint is the
duplication itself, not its styling, so the list stays exactly where it
already is at that point in the script: detached (`list.parentNode.removeChild
(list)` a few lines above the branch, which runs for every reader regardless
of the branch taken).

**The affordance is a caption suffix, not an icon or a badge.** The theme
already has this exact pattern for a running presentation's `is-paused` state
— a small, mono, uppercase `· paused` appended to the caption via `::after`
(`agentops.css`). Reusing it for `· press to play` costs one rule and no new
visual language. An icon overlay on the drawing was considered and rejected:
the drawing is dense (six boxes, a chip row, seven connectors) and has no
clear space to put one without colliding with something, which is the same
constraint that put the caption in a reserved lane in the first place.

**`::after` content is not reliably exposed to assistive technology, so the
`aria-label` carries the same information as a second, redundant source.**
`wrap` already has one (`"How it works, one beat at a time"`); the still
branch appends `" — press to play"` to it, and `engage()` restores the base
string. This is the one place the change adds behaviour beyond "stop
duplicating the list" — it was free to add and the base spec already commits
to "a discoverable, non-visual-only cue," which a CSS-only cue does not
reliably satisfy on its own.

**Both classes stay on `wrap` while still (`is-paused` and `is-still`).**
`pause()` already adds `is-paused` as part of stopping the timer, and the
still composition is a paused state in every sense the rest of the code
cares about (no timer running, `wrap.classList` checked by `toggle()`). Rather
than special-case `pause()` to skip its class for this one caller, the new
`.ao-pres.is-still .ao-pres-caption::after` rule is declared after the
existing `.ao-pres.is-paused` one in `agentops.css`, so the cascade's
tie-break by source order gives the still's more specific message priority
over the generic "paused" one. Both selectors have identical specificity, so
order is what decides it, and stating that in a comment is cheaper than adding
a class-removal special case to `pause()` that every other caller doesn't
need.

*Alternative rejected:* have `engage()`/the still branch call a version of
`pause()` that skips `classList.add('is-paused')`. That splits one function's
contract in two for one caller and invites the next reader to wonder which
callers get which behaviour. A CSS ordering comment is one line to maintain
instead.

## Risks / Trade-offs

- **A reader relying on `::after` content being announced gets nothing from
  the visual cue** → mitigated by the `aria-label` change, which is read
  regardless of how a screen reader treats generated content.
- **Removing the list means a reduced-motion reader who never presses play
  gets less raw text on the page than today** (today's list is indexable,
  searchable page content; the still's caption is not the full set of beats).
  Accepted: the drawing's node labels, the reach chips and the closing beat's
  caption are all still real DOM text, and a reader with no scripting at all —
  the case this loss would actually matter for — keeps the untouched
  CSS-only list fallback. A reduced-motion reader has scripting; they can
  press the one control to get the same full account any other reader gets.
- **This does not fix the base spec's "select any beat directly" claim**,
  which the earlier transport deletion already broke for every reader, not
  only the reduced-motion one → out of scope, named as a Non-Goal, left for
  its own change rather than folded in here.
