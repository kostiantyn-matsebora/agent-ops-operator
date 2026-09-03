# landing-presentation Specification

## Purpose

The site's presentation: a drawing that builds one piece at a time under the
reader's own control, explaining the whole model in about a minute without an
exported image, a video file or a word of burned-in text.

## Requirements

### Requirement: A presentation is a list of beats the page writes

The site SHALL provide a **presentation** — a drawing that accretes across an
ordered set of beats — and a page SHALL declare one by naming it on an ordinary
markdown list, exactly as it names any other component.

Each item SHALL be one beat, and its text SHALL be that beat's caption. **Every
word SHALL live in the page.** The theme SHALL supply the drawing, the timing
and the controls, and no beat text SHALL come from the stylesheet, a data file or
a script.

A beat SHALL say what is TRUE from that point on, not what the drawing should do.
The presentation is a statement of the model in order, not an animation script.

#### Scenario: A page declares a presentation

- **WHEN** a page names the presentation on a list of beats
- **THEN** the beats become the presentation's captions, in the order written

#### Scenario: A beat's wording is changed

- **WHEN** a contributor edits a beat's sentence
- **THEN** the change is made in the page and nowhere else

### Requirement: Without scripting it is the story in order

**With no scripting available the presentation SHALL remain its own content** —
the beats as an ordinary numbered list, in order, nothing hidden and nothing
lost.

A reader who never runs the script SHALL still receive the whole explanation.
The presentation is therefore never the only carrier of a claim the page needs
to make.

#### Scenario: Scripting is unavailable

- **WHEN** the presentation is rendered without scripting
- **THEN** every beat is readable as an ordinary list, in order

#### Scenario: A reader has asked for reduced motion

- **WHEN** the reader's system requests reduced motion
- **THEN** nothing moves and the drawing is shown fully composed
- **AND** nothing moves at any later time unless the reader activates it
- **AND** what the reader is shown beside the drawing, and how activating it
  works, is stated by `Reduced motion is a still the reader may start` rather
  than repeated here

### Requirement: The drawing is present from the first beat

The presentation's drawing SHALL be **fully laid out before the first beat
plays**, with every element it will ever show already in place and unemphasised.
A beat SHALL bring an element forward rather than introduce it into empty space.

An early beat that lands on an empty canvas puts its caption in open space with
nothing to read it against, and the reader is given a sentence where they were
promised a picture.

The drawing SHALL accrete on **one canvas** and SHALL NOT cut away between
beats: what a beat established stays established.

#### Scenario: The first beat is shown

- **WHEN** the presentation is at its first beat
- **THEN** the whole drawing is already laid out, with only that beat's subject
  emphasised

#### Scenario: The presentation is scrubbed backwards

- **WHEN** a reader selects an earlier beat
- **THEN** the drawing shows exactly what that beat established, no more and no
  less

### Requirement: The presentation never scrolls inside itself

The drawing SHALL be authored at one width and **scaled to whatever width it is
given**. It SHALL NOT introduce a scrolling region of its own at any viewport
width, and the page body SHALL NEVER scroll sideways because of it.

A scrollbar inside a presentation asks the reader to operate the explanation
before they have understood it.

Scaling SHALL be used rather than reflow so that the drawing's composition — what
sits beside what, and what connects to what — is the same at every width.

#### Scenario: The presentation is shown in a narrow column

- **WHEN** the presentation is rendered narrower than its authored width
- **THEN** it is scaled to fit, and neither it nor the page body scrolls
  sideways

### Requirement: A beat carries the resource text it is about

Where a beat concerns a field of a declared resource, the presentation SHALL show
**that stanza and no more** — the lines that beat is about, changing as the beat
changes.

The whole document SHALL NOT be shown at once beside the drawing. A reader
looking for the field a beat names should not have to find it in a manifest
they were not asked to read.

**Every field name shown SHALL exist on the CRD it claims to be**, on the same
terms as any other published manifest.

#### Scenario: A beat names a field

- **WHEN** a beat concerns one stanza of a resource
- **THEN** only that stanza is shown, and it changes when the beat does

### Requirement: The reader controls the presentation

The presentation SHALL play on its own, and the reader SHALL be able to **stop
it, start it again, and select any beat directly**. Selecting a beat SHALL stop
the automatic advance, because a reader who has taken control has stopped
watching and started reading.

A caption SHALL occupy the **same height on every beat**, so that advancing a
beat never reflows the page under the reader's eye. Beat text is written short
enough to hold that height.

The controls SHALL be reachable and operable by keyboard, and the current beat
SHALL be identifiable without relying on colour alone.

#### Scenario: A reader selects a beat

- **WHEN** a reader selects a beat directly
- **THEN** that beat is shown and the automatic advance stops

#### Scenario: Beats advance

- **WHEN** the presentation moves from one beat to the next
- **THEN** the caption's height is unchanged and nothing below it moves

### Requirement: The presentation carries no burned-in text

Every word the presentation shows SHALL be real text — selectable, translatable,
searchable and available to a screen reader.

**No exported image, video or canvas rendering SHALL carry a word of it.** This
is the same rule the site's recording follows, and it is the reason a
presentation is preferred to an exported drawing for an explanation that has to
grow.

#### Scenario: A reader translates the page

- **WHEN** the page is machine-translated
- **THEN** every beat caption and every label on the drawing is translated

### Requirement: Reduced motion is a still the reader may start

A reduced-motion reader SHALL be offered the same control every other reader
has — the drawing itself — and SHALL NOT be shown a second, separate control
for it. Nothing SHALL move until that control is used.

The preference says *do not move things at me*. It does not say *never let me
ask*, and a page that answers it by deleting the control has decided on the
reader's behalf that the explanation is not for them. The beats are the
landing page's central account of the product, and on that machine they
would otherwise be reachable only in the form a reader with no scripting
gets.

**The still SHALL NOT duplicate the beat list beside the drawing.** The
composed drawing already shows the whole model; a second, unlabelled copy of
the same beats as plain text underneath it is not an accommodation, it is the
same content shown twice with no indication either copy can be acted on. The
list-as-content fallback SHALL be reserved for a reader with no scripting at
all, who has no drawing to look at in the first place.

**The still SHALL carry a discoverable, non-visual-only cue that the drawing
is a control**, readable on the page and to assistive technology, so a reader
is not left to guess that the frozen picture can be pressed.

Engaging it is the reader's own action, so what follows is the ordinary
presentation, and it SHALL start at the first beat rather than resuming from
whatever the still was composed as.

#### Scenario: A reduced-motion reader is offered the presentation

- **WHEN** the presentation is rendered for a reader whose system requests
  reduced motion
- **THEN** the drawing is shown fully composed, and nothing beside it repeats
  the beats
- **AND** a cue on the page, and in the drawing's accessible name, states that
  it can be pressed to play
- **AND** nothing has moved

#### Scenario: A reduced-motion reader starts it

- **WHEN** that reader activates the drawing
- **THEN** the presentation plays from its first beat, exactly as it does for
  a reader who asked for no such preference
- **AND** the cue that it can be pressed to play is gone, since it no longer
  applies

#### Scenario: A reduced-motion reader pauses it again

- **WHEN** that reader stops the presentation after starting it
- **THEN** the current beat stays on the stage, and the reader may start it
  again by activating the drawing, exactly as any reader can
