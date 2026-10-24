## Context

See proposal.md — Why.

Two existing mechanisms carry the adopter narrative this change touches:

- `docs/diagrams/readme-flow.py` writes `assets/img/readme-flow-{light,dark}.svg`
  by hand-drawn SVG, run manually and committed, not CI-generated.
- `index.md`'s `.ao-presentation` list is read by `assets/js/presentation.js`,
  which builds a stepped drawing at runtime from beat data (`NODES`, `WIRES`,
  `SCRIPT`) and scales it to the column width. See
  `docs/.claude/site.md` and `.claude/rules/palette-and-mark.md` for the
  palette/token copy rules both mechanisms must follow.

## Goals / Non-Goals

**Goals:**
- Settle the STORY and COMPOSITION of the Coordinator half of both
  mechanisms before touching the real `.py` script and `.js` component.
- Carry forward the concrete bugs already found and fixed in the
  prototypes, so the real implementation does not re-introduce them.

**Non-Goals:**
- This design does not re-architect `presentation.js`'s beat-script engine
  or `readme-flow.py`'s drawing primitives. Both are proven, working
  mechanisms. This change extends their content, not their machinery.
- The prototypes are not pixel-exact templates to copy-paste. See Decisions.

## Decisions

### The prototypes are committed, not linked

`prototypes/readme-diagram-mockup.html` and
`prototypes/landing-presentation-mockup.html` are standalone, self-contained
HTML files — open either directly in a browser, no build step. They were
each iterated through several real, found-and-fixed defects:

- Connectors defined with an undersized `stroke-dasharray` length cut short
  of their target box. Fixed by reading the length off the rendered path
  (`getTotalLength()`) rather than a hand-typed estimate.
- Shapes stretched with `preserveAspectRatio="none"` lost their
  recognizable silhouette (a hexagon with angles too shallow to read as a
  hexagon). Reverting to full-bleed fill, matched to a shape authored at
  the box's own aspect ratio, was the version that actually shipped clean —
  not the "proportionally correct but doesn't fit the text" alternative,
  which was tried and measured broken (text up to 3x wider than its shape).
- An element defined before the box behind it in SVG document order got
  silently painted over by that box's later, opaque fill.
- A background "patch" rect added to mask one such z-order bug was sized
  wider than the gap it needed to cover, and itself painted a visible
  notch into two neighboring boxes.
- A hand-rolled arrow marker had its triangle manually reversed, not
  accounting for `orient="auto"` already rotating it to match the path's
  travel direction — doubly flipped, pointing backward. Fixed by reusing
  the single shared marker shape every other arrow in the file uses, and
  letting `auto` orientation do the rotation.

None of these are visible in the final prototypes. They are named here so
the person doing the real implementation does not re-derive them by
re-making the same mistakes.

### What the prototype settles, and what it does not

The prototype is the reference for **composition**: which boxes exist,
what shape each kind takes, how the connectors route, where the loop is,
where escalate branches off, and the beat-by-beat order of reveal.

The prototype is explicitly **not** the reference for:

- **Literal pixel coordinates.** Both mockups use their own standalone
  canvas size and coordinate system. `presentation.js`'s actual `DRAW_H`,
  `STAGE_W`, and the real site's existing Pipeline geometry (`NODES`,
  `WIRES` in the live file) are the numbers that matter for the real
  component — the Coordinator geometry needs to be re-derived to share
  that canvas and connector-routing convention, not transplanted wholesale.
- **Copy.** The captions in both prototypes are working placeholders
  ("the agent that decides, and invokes named members by purpose") meant
  to test the story, not final site prose. The real captions should match
  the voice of the beats already shipped for Pipeline.
- **Example names.** `k8s-triage`, `log-reader`, `remediator`,
  `home-triage` are illustrative. Whether the real site reuses these or
  picks its own is an implementation call, not something this design
  fixes.
- **The icon/glyph source.** The prototype hand-copies the relevant
  `SHAPES`/`GLYPHS` entries from `platform/console/ui/src/graph/shapes.tsx`
  as inline constants. The real implementation should import or mirror
  that file directly per whatever convention `palette-and-mark.md`'s
  copy rule already establishes for site assets, not keep a second
  hand-copied set in `presentation.js`.
- **The Coordinator hub's shape choice.** The prototype deliberately draws
  the hub as a diamond, departing from `shapes.tsx`'s own `rect` for
  `coordinators` — the standard flowchart mark for a decision, which is
  the hub's actual job, and what makes it read as distinct from Pipeline's
  rectangle at a glance. This is a one-line deviation from the source of
  truth and should stay deliberate and commented wherever it lands in the
  real file, not silently drift back to `rect` on a future shapes.tsx sync.

### Tab order and naming

Landing presentation tab order: Orchestrator first, Pipeline second.
"Orchestrator" is the human-facing label for the Coordinator's role in
this narrative.

The kind label drawn inside the figure itself stays `Coordinator`, since
that is the name actually declared in the CRD.

Guide prose, CHANGELOG, and security.md continue to say `Coordinator`
throughout. "Orchestrator" is scoped to this one narrative surface. It is
not a renaming of the kind.

### README diagram: redraw, don't duplicate

The existing `readme-flow.svg` Pipeline content is kept close to verbatim.
The Coordinator half is new. Both share the frame/label visual language
already established (plain cards, same arrow style, same palette tokens)
so the two read as one system, not two unrelated pictures bolted together.

## Risks / Trade-offs

- **The landing-page presentation rework touches `presentation.js`'s beat
  engine**, which is shared machinery for both stories. A mistake there
  risks the already-shipped Pipeline story, not just the new Coordinator
  one. Mitigation: extend the existing `NODES`/`WIRES`/`SCRIPT` data shape
  per story rather than restructuring the engine itself, and verify the
  Pipeline tab renders unchanged after the change, not only the new tab.
- **Two static SVG sources now need to agree with the interactive
  presentation** on the same story (README's `readme-flow.svg` and the
  landing page's `.ao-presentation`). They are deliberately not the same
  mechanism (`docs/.claude/site.md` names this as three non-interchangeable
  diagram sources already). Mitigation: review both together before
  calling the change done, so the Coordinator story they each tell does
  not quietly diverge.
- **`introduction.md` and `getting-started.md` are reviewed, not
  pre-diagnosed.** The actual edit scope there is unknown until that
  review happens. Mitigation: the review itself is a tasks.md item with
  its own checkpoint, not folded silently into another task.

## Open Questions

None. Everything that would change the approach or the task breakdown was
resolved during exploration (see proposal.md and the committed
prototypes) rather than deferred.
