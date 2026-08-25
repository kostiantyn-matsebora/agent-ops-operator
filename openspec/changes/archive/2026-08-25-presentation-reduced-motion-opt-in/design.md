## Context

See `proposal.md` — Why. What shapes the approach is the shape of the component:
`docs/assets/js/presentation.js` builds ONE set of nodes (stage, stanza box,
rail) from the ordered list `index.md` writes, then either plays them or, under
reduced motion, deletes half of them and puts the list back. There is no second
rendering path to extend — the still and the running presentation are the same
DOM in two configurations.

The rail is a three-column grid (`.ao-pres-rail`, `agentops.css`): play button,
caption, dots, with the progress bar spanning the row below.

## Goals / Non-Goals

**Goals:**

- One control visible while still, and it is the one that starts the thing.
- The still stays exactly what it is today until that control is used.
- No second code path: engaging reuses `goTo` / `start`, which every other
  reader already exercises.

**Non-Goals:**

- Persisting the reader's choice across pages or reloads. A preference stored
  against a system preference is a second source of truth for the same
  question, and the reader can press play again.
- Any change to the theme's global reduced-motion rule, which clamps transition
  and animation durations for the whole site.
- Offering the same opt-in to a reader with no scripting. There is nothing to
  press, and the list is already the whole content.

## Decisions

**The still KEEPS the rail and hides its beat-level children, rather than
building a second control.** A standalone "play" button appended somewhere would
be a second element with its own styling, its own label and its own focus
behaviour, and the two would drift. The rail's children are removed and held in
a variable, then re-appended in order when the reader engages — `play` is never
detached, so its label, its glyph and its click handler are the same object in
both configurations.

*Alternative rejected:* a CSS `.ao-pres--still` class hiding the children with
`display: none`. It reads cleaner, but hidden-not-removed leaves the dots in the
tab order and the caption text in the accessibility tree — nine beats announced
twice, once from the rail and once from the list beside it.

**Engaging is ONE-WAY.** There is no path back to the still. A reader who
pauses gets the ordinary paused transport, because they have already said they
want the presentation; taking the beat list back out from under them on pause
would move the page for the reader who asked for less movement.

**It starts at beat one, not at the composed still.** The still is every element
lit at once, which is beat nine's state. Resuming from there would play the last
beat and wrap to the first, so the reader who pressed play would watch the
explanation start at its end.

**The stanza box comes back with it.** It is removed in the still because a
still has no current beat to show lines for; a running presentation without it
loses the manifest half of every beat.

**No prototype.** The change alters WHEN the existing controls are shown, not
what they look like, so there is no composition to settle against a mockup and
nothing to commit as a reference. Verification is a screenshot of the real page
under an emulated reduced-motion preference — see tasks.

## Risks / Trade-offs

- **The still rail is a wide bar holding one small round button** → the CSS
  needs a rule for the still configuration, or it renders as a control stranded
  at the left of an empty box. The grid collapses to one auto column while
  still.
- **A reader engages and the page grows** — the list is removed and the stanza
  box appears, so the content below shifts once → accepted, and it is the
  reader's own action rather than something that happens at them. The stanza
  box carries a reserved `min-height`, so the beats after the first do not
  move it again.
- **The preference is read once, at load** → a reader toggling the OS setting
  with the page open sees no change until reload. Unchanged from today, and a
  `matchMedia` listener would have to decide what to do with a presentation
  already running.
