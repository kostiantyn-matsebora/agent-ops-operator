## Why

**A reader whose system requests reduced motion sees the landing page's "How it
works" presentation as a frozen, fully-lit drawing with every beat's caption and
manifest stanza dumped in a plain list underneath it — with nothing telling
them the drawing is clickable at all.** `docs/assets/js/presentation.js`'s
reduced-motion branch still implements the design from
`2026-08-25-presentation-reduced-motion-opt-in`: a visible play-button rail the
reader could press to opt into the animation, with the beat list left in place
beside it as the still's only readable account. A later, undocumented change
deleted that whole transport — the play button, the beat counter, the scrub
dots, the progress bar — in favour of "the figure itself is the control," on
which clicking the drawing is the one control every reader gets. The
reduced-motion branch was never updated to match: it still adds a CSS class
(`is-still`) for which no rule exists any more, so there is no visual
difference marking the drawing as a control, and it still reinserts the entire
beat list — now a second, unlabelled copy of the whole explanation sitting
under a drawing that already shows the whole model.

The result reads as broken rather than as a considered accommodation: a
complete picture, immediately followed by the same account spelled out again
in prose, with no visible way to do anything about either.

## What Changes

- **The reduced-motion still stops reinserting the beat list.** The composed
  drawing already carries the whole model; showing it twice was the defect.
  The list-as-content fallback stays exactly as it is for a reader with no
  scripting at all — that CSS-only path is unaffected.
- **The still gains a discoverable affordance instead**: the caption already
  showing the closing beat's sentence gains a short cue (mirroring the
  existing "· paused" cue) saying the figure can be pressed to play, and the
  group's `aria-label` states the same thing for a screen-reader user.
- **The control is the same one every other reader already has** — clicking or
  keyboard-activating the figure — so nothing new is built. `engage()` and
  `toggle()` are unchanged; only what surrounds the still changes.
- Not breaking: no page markup changes, the component is still driven by the
  ordered list `index.md` writes, and behaviour for a reader with no
  reduced-motion preference and no scripting is unchanged.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `landing-presentation`: the reduced-motion requirement changes from "a
  play-button rail is the only control shown, beside the untouched beat list"
  to "the still is the same figure and the same click every other reader gets,
  with a caption cue and label as the only discoverable difference, and no
  second copy of the beats shown beside it."

## Impact

**Code**

- `docs/assets/js/presentation.js` — the reduced-motion branch stops
  reinserting the beat list, sets and clears a discoverable `aria-label` on
  engage, and its comments are rewritten for the current "figure is the
  control" design rather than the deleted rail.
- `docs/assets/css/agentops.css` — a `.ao-pres.is-still` caption cue is added
  (styled like the existing `.ao-pres.is-paused` one), replacing the dead
  `is-still` class that carried no rule.

**Reference docs**

- `docs/CLAUDE.md` — the `{: .ao-presentation}` component row states what the
  component does without scripting and under "the figure is the control," but
  says nothing about reduced motion. It gains one sentence, the same gap the
  predecessor change closed and a later change silently reopened.
- `docs/.claude/site.md` — the landing-page bullet describing "the figure is
  the control" gets the same one-sentence addition, so the next reader who
  deletes machinery from this component sees the reduced-motion path named
  beside it.

**The adopter site**

- `docs/index.md` — no copy changes. The page writes the beats and the theme
  supplies the drawing and its controls, which is what keeps this change out
  of the page. Checked rather than assumed.
- No landing-page claim, Introduction, Getting started, Installation or guide
  describes the presentation's reduced-motion behaviour, so none of them is
  made untrue.

**Not affected**

- `docs/CHANGELOG.md` — no chart value, CRD field or upgrade step moves.
- Every other `.ao-*` component, and the theme's global reduced-motion rule
  (`agentops.css`, the `*` transition-duration clamp).
