# Design

## Context

See `proposal.md` — Why. The requirements are in
`specs/landing-presentation/spec.md` and the two delta specs beside it.

Three constraints from the existing site shape the whole approach:

- **The theme holds no prose and the pages hold no theme.** A page names a class
  with a kramdown attribute list, and the stylesheet supplies only the look.
  Nothing new may put product words into `_includes/`, `_data/` or a script.
- **The content column is 45rem** and there is no breakout. Anything wider is
  either scaled or wrong.
- **Every asset is build output** from a committed command, never a hand
  capture.

**The agreed composition is committed beside this document**, at
`mockup/landing.html`, working: the presentation runs, the tabs switch, both
themes resolve, and it loads the repository's own marks and recording by
relative path. `mockup/README.md` says how to open it.

**It is the reference for composition, and the specs are the reference for
behaviour.** The specs state that a caption reserves its height. They do not
state that it is 87px, and they should not. Every such number — the stage's
authored width, the beat hold, the ghost opacity, the node coordinates — is
settled in the mockup and nowhere else.

Without it an implementer re-derives the design from prose and arrives somewhere
else, which is what this change already cost several rounds of.

## Goals / Non-Goals

**Goals:**

- One new theme component whose content lives entirely in the page.
- A landing opening that survives with scripting unavailable.
- Delete more than is added — the drawing, its exports, the tiles and the
  layout's content split all go.

**Non-Goals:**

- Redesigning the site's shell, palette, navigation or any other page.
- Changing the recording's production pipeline. It gains one beat and keeps
  everything else, including its budgets.
- A general animation framework. This is one component with one job.

## Decisions

### The presentation is HTML and CSS driven by a beat index, not an exported animation

**Chosen:** the drawing is ordinary elements positioned on a stage, and one
script sets a beat index that toggles classes.

Alternatives considered:

- **An animated SVG (SMIL or CSS inside the asset).** Rejected: the text would
  live in the asset, not the page, which is the rule the site is built on. It
  also puts the beats out of reach of translation and search.
- **A rendered video, like the recording.** Rejected for the same reason plus
  weight: two theme variants of a minute of video against roughly nothing for
  markup, and the captions would be pixels.
- **Canvas or WebGL.** Rejected: no text, no selection, no screen reader.

**Beats are a markdown list the page writes**, mirroring `{: .ao-tabs}` — the
script reads the list, builds the strip, and removes it. That is what makes the
no-script fallback free rather than designed.

### The drawing is authored at a fixed width and SCALED, never reflowed

The stage carries absolute coordinates so that connectors can be drawn between
known points. Reflowing it at narrow widths would mean re-authoring every
connector per breakpoint.

**Chosen:** author at one width, then scale to the container.

- A `transform: scale()` does **not** reduce the element's layout width, so the
  container must also be given the scaled height and must hide overflow.
  Getting only half of that produces the exact defect this change is fixing — a
  scrollbar inside the explanation.
- Alternative: an SVG `viewBox`, which scales for free. Rejected because the
  labels would then be SVG text — real text, but outside the page's own flow and
  awkward to style from the theme's tokens.

### The whole drawing is laid out from the first beat, and beats only emphasise

A beat brings an element forward; it never introduces one. Elements start
dashed and faint, connectors start as dotted tracks.

This is a correctness decision as much as an aesthetic one: with elements
appearing into empty space, the first beats put a caption in open space with
nothing beside it, and the composition visibly jumps as each piece lands.

### The caption reserves its height and the beats are written short

Beat text that reflows between one and two lines moves the page under the
reader on every advance. **Chosen:** short beats — the detail is already in the
drawing and the stanza — plus a reserved caption height so a translation cannot
reintroduce the jump.

### The manifest is a per-beat stanza, not the whole document

Showing all fifteen lines beside the drawing makes the reader find the field the
beat is about. Showing three lines that change with the beat does not.

The same manifest text appears in full in the third tab, where a reader who
wants to copy it can.

### The `works with` row is a chip set with one group

Rather than a new component. The chip-set component already renders a labelled
group of short names with page-named marks, which is exactly what this is. The
landing page keeps one group where it used to have three.

### The exported landing drawing is deleted, not kept as a fallback

Keeping it would mean two statements of the model that must agree, one of which
nobody would remember to re-export. The `site` page and the full-size link stay,
so the full argument is still available as a drawing.

### The demo beat goes after the reply beat

Placing origination first would open the story with a person doing something,
which contradicts the product's thesis that signals start the work. After the
reply it reads as a capability reveal.

## Risks / Trade-offs

- **A third component with moving parts on one page** (tabs, player,
  presentation) → each is independent and each degrades to its own content. The
  presentation registers with the existing theme resolver rather than watching
  the theme itself, so two painters cannot disagree after a toggle.
- **The recording is 59.6s against a hard 75s ceiling and gains a beat** → the
  budget is not raised. Existing holds shorten, and the caption track is
  regenerated from the same beats so the two cannot drift.
- **Deleting the exported drawing loses the one asset that could be posted
  elsewhere** → the standalone `why` poster already serves that, rendered on
  demand and never committed.
- **Scaling makes the drawing small on a phone** → the fallback list is the
  answer at that width, and the beats are the whole explanation in text.
- **The stage's absolute coordinates rot when a node's label changes length** →
  the overlap check is a task, run across every beat, rather than a visual
  inspection.

## Migration Plan

Site-only. No chart version, no image, nothing for an adopter to upgrade, so no
`CHANGELOG.md` entry.

The deletions must land in the same change as the additions — a release in which
`index.md` no longer names the exported SVG while the file is still committed
leaves an orphan the drift check would not catch.

Rollback is `git revert`: the deleted SVGs and the drawio page come back with it.

## Open Questions

None. The register, the panel order, the table-over-tiles choice and the demo
beat's placement were each settled against the sketch before this was written.
