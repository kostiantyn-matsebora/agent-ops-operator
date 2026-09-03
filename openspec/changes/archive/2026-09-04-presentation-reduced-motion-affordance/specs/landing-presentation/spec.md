## MODIFIED Requirements

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
