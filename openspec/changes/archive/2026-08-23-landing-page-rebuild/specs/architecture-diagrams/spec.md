## REMOVED Requirements

### Requirement: The landing page of the drawing is the poster's composition, minus what the prose states

**Reason**: The landing page no longer shows an exported drawing. Its model is
carried by the presentation, which builds the same argument one beat at a time in
real text, so a still restating it would be the same claim twice — once in a form
that cannot be selected, translated or searched.

The requirement existed to keep a still legible at the content column's width,
and every attempt to add detail to it had to remove elements instead. That
ceiling is what the presentation removes.

**Migration**: The `landing` page is deleted from
`docs/diagrams/agent-ops.drawio` and from `docs/diagrams/export.py`'s export
list, and its two exported SVGs are deleted from `docs/assets/img/`. The `site`
page, the full-size link that shows it and the standalone `why` poster are
untouched — the drawio source keeps two pages, of which one is exported.

## MODIFIED Requirements

### Requirement: A page the site leads with is authored for the width it is shown at

A drawing the site displays inline SHALL be authored so that at the content
column's width **no text on it renders below 12px**. Type size on the drawing is
therefore chosen from the export width and the column width together, not from
what reads well in the editor.

A drawing that fails this SHALL be simplified until it passes — reduced to fewer,
larger elements — and SHALL NOT be fixed by widening it past the column, since a
column-width breakout is only ever as wide as the column it breaks out of.

**This governs exported drawings only.** The site's presentation is not one: it
is real text laid out by the theme, and it meets the legibility rule by being
scaled to the width it is given rather than by being authored for it.

Its exports SHALL carry an **opaque ground** in each theme's own canvas colour
rather than a transparent one. The theme swap is a deferred script, so the light
export is on the page before it runs — and a transparent one there is not a
mismatched colour but invisible ink.

#### Scenario: The drawing is displayed inline

- **WHEN** a page renders an exported drawing at the content column's width
- **THEN** every label on it is at least 12px on screen

#### Scenario: More detail is wanted on the drawing

- **WHEN** a contributor wants to add elements to a drawing the site displays
  inline
- **THEN** they either keep it within the legibility rule or put the detail on
  the full-size page instead

#### Scenario: The light export is shown on a dark page

- **WHEN** the light variant is on a dark-theme page, before the swap runs or
  because scripting is unavailable
- **THEN** it carries its own ground and stays legible

#### Scenario: An explanation outgrows a still

- **WHEN** an explanation needs more detail than a still can carry at the
  column's width
- **THEN** it is built as a presentation rather than by shrinking the drawing's
  type
